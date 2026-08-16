package app

import (
	"context"
	"fmt"

	"bruno-browser/internal/account"
	"bruno-browser/internal/browser"
	"bruno-browser/internal/config"
	"bruno-browser/internal/extensions"
	"bruno-browser/internal/fingerprint"
	"bruno-browser/internal/license"
	"bruno-browser/internal/maintenance"
	"bruno-browser/internal/network"
	"bruno-browser/internal/preferences"
	"bruno-browser/internal/profile"
	"bruno-browser/internal/telemetry"
	"bruno-browser/internal/updates"
)

// Core wires the durable profile store to the Chromium process manager.
// The Wails bridge will expose this core to the frontend in stage 7.
type Core struct {
	Profiles    *profile.Store
	Browser     *browser.Manager
	Maintenance *maintenance.Service
	Network     *network.Manager
	Fingerprint *fingerprint.Controller
	Account     *account.Service
	License     *license.Service
	Preferences *preferences.Service
	Extensions  *extensions.Service
	Updates     *updates.Service
	Telemetry   *telemetry.Service
}

func New(config config.Config) (*Core, error) {
	profiles, err := profile.NewStore(config.ProfilesRoot())
	if err != nil {
		return nil, fmt.Errorf("initialize profile store: %w", err)
	}
	protector, err := network.NewDefaultProtector(config.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("initialize network credential protection: %w", err)
	}
	networkStore, err := network.NewStore(profiles, protector)
	if err != nil {
		return nil, fmt.Errorf("initialize network store: %w", err)
	}
	networkManager, err := network.NewManager(networkStore)
	if err != nil {
		return nil, fmt.Errorf("initialize network manager: %w", err)
	}
	fingerprintStore, err := fingerprint.NewStore(profiles)
	if err != nil {
		return nil, fmt.Errorf("initialize fingerprint store: %w", err)
	}
	fingerprintController, err := fingerprint.NewController(fingerprintStore, func(ctx context.Context, profileID, rawURL string) error {
		_, recordErr := profiles.RecordLastURL(ctx, profileID, rawURL)
		return recordErr
	})
	if err != nil {
		return nil, fmt.Errorf("initialize fingerprint controller: %w", err)
	}
	browserManager, err := browser.NewManager(profiles, browser.Config{
		ExecutablePath: config.BrowserExecutable,
		PrepareNetwork: func(ctx context.Context, profileID, userDataDir string) (browser.NetworkSession, error) {
			return networkManager.Prepare(ctx, profileID, userDataDir)
		},
		AttachCDP: func(ctx context.Context, profileID, websocketURL, initialURL string) (browser.CDPSession, error) {
			return fingerprintController.Attach(ctx, profileID, websocketURL, initialURL)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize browser manager: %w", err)
	}
	maintenanceService, err := maintenance.NewService(profiles, browserManager)
	if err != nil {
		return nil, fmt.Errorf("initialize maintenance service: %w", err)
	}
	accountService, err := account.New(config.DataRoot, account.Config{
		ClientID: config.DiscordClientID, ClientSecret: config.DiscordClientSecret, AdminID: config.AdminDiscordID,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize account service: %w", err)
	}
	licenseService, err := license.New(config.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("initialize license service: %w", err)
	}
	preferencesService, err := preferences.New(config.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("initialize preferences: %w", err)
	}
	extensionService, err := extensions.New(config.DataRoot, profiles)
	if err != nil {
		return nil, fmt.Errorf("initialize extension library: %w", err)
	}
	if bundledPath, exists := extensions.FindBrunoINSSIST(); exists {
		if _, _, err := extensionService.EnsureBundled(context.Background(), bundledPath, extensions.BrunoINSSISTSHA256); err != nil {
			return nil, fmt.Errorf("initialize bundled Bruno INSSIST extension: %w", err)
		}
	}
	updateService, err := updates.New(config.UpdateURL, updates.WithDataRoot(config.DataRoot))
	if err != nil {
		return nil, fmt.Errorf("initialize update service: %w", err)
	}
	telemetryService, err := telemetry.New(config.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("initialize telemetry: %w", err)
	}
	return &Core{
		Profiles:    profiles,
		Browser:     browserManager,
		Maintenance: maintenanceService,
		Network:     networkManager,
		Fingerprint: fingerprintController,
		Account:     accountService,
		License:     licenseService,
		Preferences: preferencesService,
		Extensions:  extensionService,
		Updates:     updateService,
		Telemetry:   telemetryService,
	}, nil
}
