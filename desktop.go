package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"bruno-browser/internal/account"
	appcore "bruno-browser/internal/app"
	"bruno-browser/internal/browser"
	"bruno-browser/internal/domain"
	"bruno-browser/internal/extensions"
	"bruno-browser/internal/license"
	"bruno-browser/internal/maintenance"
	"bruno-browser/internal/network"
	"bruno-browser/internal/preferences"
	"bruno-browser/internal/profile"
	"bruno-browser/internal/telemetry"
	"bruno-browser/internal/updates"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Desktop struct {
	core            *appcore.Core
	settingsPath    string
	oauthConfigured bool
	ctx             context.Context
}

type BootstrapState struct {
	Profiles        []ProfileView           `json:"profiles"`
	Account         *account.User           `json:"account,omitempty"`
	Plan            license.Activation      `json:"plan"`
	Preferences     preferences.Preferences `json:"preferences"`
	Updates         updates.Status          `json:"updates"`
	Extensions      []extensions.Extension  `json:"extensions"`
	OAuthConfigured bool                    `json:"oauthConfigured"`
	SettingsPath    string                  `json:"settingsPath"`
	Telemetry       telemetry.Snapshot      `json:"telemetry"`
}

type ProfileView struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Color            string               `json:"color"`
	CreatedAt        time.Time            `json:"createdAt"`
	UpdatedAt        time.Time            `json:"updatedAt"`
	LastLaunchedAt   *time.Time           `json:"lastLaunchedAt,omitempty"`
	LaunchCount      uint64               `json:"launchCount"`
	Platforms        []domain.Platform    `json:"platforms"`
	Status           domain.ProfileStatus `json:"status"`
	Tags             []domain.Tag         `json:"tags"`
	Notes            string               `json:"notes"`
	StartURL         string               `json:"startUrl"`
	LastURL          string               `json:"lastUrl"`
	ExtensionPaths   []string             `json:"extensionPaths"`
	DNSPreset        network.DNSPreset    `json:"dnsPreset"`
	Proxy            *ProxyView           `json:"proxy"`
	Running          bool                 `json:"running"`
	Engine           string               `json:"engine"`
	FingerprintScore int                  `json:"fingerprintScore"`
	FingerprintLabel string               `json:"fingerprintLabel"`
	Risk             string               `json:"risk"`
	RiskReasons      []string             `json:"riskReasons"`
}

type ProxyView struct {
	Mode        network.Mode `json:"mode"`
	Host        string       `json:"host"`
	Port        uint16       `json:"port"`
	Username    string       `json:"username"`
	HasPassword bool         `json:"hasPassword"`
	BypassList  []string     `json:"bypassList"`
	Endpoint    string       `json:"endpoint"`
	LatencyMs   int64        `json:"latencyMs"`
}

type ProfileInput struct {
	Name      string               `json:"name"`
	Color     string               `json:"color"`
	Platforms []domain.Platform    `json:"platforms"`
	Status    domain.ProfileStatus `json:"status"`
	Tags      []domain.Tag         `json:"tags"`
	Notes     string               `json:"notes"`
	StartURL  string               `json:"startUrl"`
}

func NewDesktop(core *appcore.Core, settingsPath string, oauthConfigured bool) *Desktop {
	return &Desktop{core: core, settingsPath: settingsPath, oauthConfigured: oauthConfigured}
}

func (d *Desktop) startup(ctx context.Context) { d.ctx = ctx }

func (d *Desktop) domReady(ctx context.Context) {
	d.ctx = ctx
	go func() {
		status, err := d.core.Updates.Check(ctx)
		if err != nil {
			return
		}
		runtime.EventsEmit(ctx, "update:status", status)
	}()
}

func (d *Desktop) Bootstrap() (BootstrapState, error) {
	ctx := d.context()
	profiles, telemetrySnapshot, err := d.listProfilesAndTelemetry(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	user, err := d.core.Account.Get(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	discordID := ""
	if user != nil {
		discordID = user.ID
	}
	plan, err := d.core.License.Status(ctx, discordID)
	if err != nil {
		return BootstrapState{}, err
	}
	prefs, err := d.core.Preferences.Get(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	updateStatus, err := d.core.Updates.Current(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	extensionList, err := d.core.Extensions.List(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	return BootstrapState{
		Profiles: profiles, Account: user, Plan: plan, Preferences: prefs,
		Updates: updateStatus, Extensions: extensionList,
		OAuthConfigured: d.oauthConfigured, SettingsPath: d.settingsPath,
		Telemetry: telemetrySnapshot,
	}, nil
}

func (d *Desktop) ListProfiles() ([]ProfileView, error) { return d.listProfiles(d.context()) }

func (d *Desktop) CreateProfile(input ProfileInput) (ProfileView, error) {
	metadata, err := d.core.Profiles.Create(d.context(), input.fields(nil))
	if err != nil {
		return ProfileView{}, err
	}
	return d.singleProfileView(d.context(), metadata)
}

func (d *Desktop) UpdateProfile(profileID string, input ProfileInput) (ProfileView, error) {
	current, err := d.core.Profiles.Get(d.context(), profileID)
	if err != nil {
		return ProfileView{}, err
	}
	metadata, err := d.core.Profiles.Update(d.context(), profileID, input.fields(current.ExtensionPaths))
	if err != nil {
		return ProfileView{}, err
	}
	return d.singleProfileView(d.context(), metadata)
}

func (d *Desktop) LaunchProfile(profileID, targetURL string) (browser.ProcessInfo, error) {
	if err := d.requirePremium(); err != nil {
		return browser.ProcessInfo{}, err
	}
	info, err := d.core.Browser.Launch(d.context(), profileID, strings.TrimSpace(targetURL))
	_ = d.core.Telemetry.RecordLaunch(d.context(), profileID, err == nil)
	return info, err
}

func (d *Desktop) StopProfile(profileID string) error {
	return d.core.Browser.Stop(d.context(), profileID)
}

func (d *Desktop) DeleteProfile(profileID string) (maintenance.Report, error) {
	return d.core.Maintenance.DeleteProfile(d.context(), profileID)
}

func (d *Desktop) ClearProfileCache(profileID string) (maintenance.Report, error) {
	return d.core.Maintenance.ClearHistoryAndCache(d.context(), profileID)
}

func (d *Desktop) ClearProfileSession(profileID string) (maintenance.Report, error) {
	return d.core.Maintenance.ClearCookiesAndSession(d.context(), profileID)
}

func (d *Desktop) SaveNetwork(profileID string, input network.SaveInput) (network.Settings, error) {
	if err := d.requirePremium(); err != nil {
		return network.Settings{}, err
	}
	return d.core.Network.Save(d.context(), profileID, input)
}

func (d *Desktop) TestNetwork(profileID string) (network.TestResult, error) {
	if err := d.requirePremium(); err != nil {
		return network.TestResult{}, err
	}
	result, err := d.core.Network.TestProxy(d.context(), profileID)
	_ = d.core.Telemetry.RecordProxyTest(d.context(), profileID, err == nil, result.LatencyMs)
	return result, err
}

func (d *Desktop) GetTelemetry() (telemetry.Snapshot, error) {
	metadataList, err := d.core.Profiles.List(d.context())
	if err != nil {
		return telemetry.Snapshot{}, err
	}
	states, err := d.telemetryStates(d.context(), metadataList)
	if err != nil {
		return telemetry.Snapshot{}, err
	}
	return d.core.Telemetry.Snapshot(d.context(), states)
}

func (d *Desktop) SavePreferences(input preferences.Preferences) (preferences.Preferences, error) {
	return d.core.Preferences.Save(d.context(), input)
}

func (d *Desktop) GetUpdates() (updates.Status, error)      { return d.core.Updates.Current(d.context()) }
func (d *Desktop) CheckForUpdates() (updates.Status, error) { return d.core.Updates.Check(d.context()) }

func (d *Desktop) CopyText(value string) error {
	if len(value) > 64<<10 {
		return errors.New("clipboard text is too large")
	}
	return runtime.ClipboardSetText(d.context(), value)
}

func (d *Desktop) LoginDiscord() (account.User, error) {
	return d.core.Account.Login(d.context(), func(rawURL string) error {
		runtime.BrowserOpenURL(d.context(), rawURL)
		return nil
	})
}

func (d *Desktop) LogoutDiscord() error { return d.core.Account.Logout(d.context()) }

func (d *Desktop) ActivateKey(key string) (license.Activation, error) {
	user, err := d.loggedUser()
	if err != nil {
		return license.Activation{}, err
	}
	return d.core.License.Activate(d.context(), key, user.ID)
}

func (d *Desktop) DeactivateKey() error { return d.core.License.Deactivate(d.context()) }

func (d *Desktop) LicenseStatus() (license.Activation, error) {
	user, _ := d.core.Account.Get(d.context())
	if user == nil {
		return d.core.License.Status(d.context(), "")
	}
	return d.core.License.Status(d.context(), user.ID)
}

func (d *Desktop) GenerateKey(discordID string, plan license.Plan) (license.HistoryEntry, error) {
	if err := d.requireAdmin(); err != nil {
		return license.HistoryEntry{}, err
	}
	return d.core.License.Generate(d.context(), discordID, plan)
}

func (d *Desktop) InspectKey(key string) (license.Claims, error) {
	if err := d.requireAdmin(); err != nil {
		return license.Claims{}, err
	}
	return d.core.License.Inspect(d.context(), key)
}

func (d *Desktop) KeyHistory() ([]license.HistoryEntry, error) {
	if err := d.requireAdmin(); err != nil {
		return nil, err
	}
	return d.core.License.History(d.context())
}

func (d *Desktop) InstallExtension() (extensions.Extension, error) {
	if err := d.requirePremium(); err != nil {
		return extensions.Extension{}, err
	}
	path, err := runtime.OpenFileDialog(d.context(), runtime.OpenDialogOptions{
		Title:   "Instalar extensão CRX",
		Filters: []runtime.FileFilter{{DisplayName: "Extensão do Chromium (*.crx)", Pattern: "*.crx"}},
	})
	if err != nil {
		return extensions.Extension{}, err
	}
	if path == "" {
		return extensions.Extension{}, errors.New("installation cancelled")
	}
	return d.core.Extensions.InstallCRX(d.context(), path)
}

func (d *Desktop) ListExtensions() ([]extensions.Extension, error) {
	return d.core.Extensions.List(d.context())
}

func (d *Desktop) SetExtensionAssignments(extensionID string, profileIDs []string) (extensions.Extension, error) {
	if err := d.requirePremium(); err != nil {
		return extensions.Extension{}, err
	}
	return d.core.Extensions.SetAssignments(d.context(), extensionID, profileIDs)
}

func (d *Desktop) RemoveExtension(extensionID string) error {
	if err := d.requirePremium(); err != nil {
		return err
	}
	installed, err := d.core.Extensions.List(d.context())
	if err != nil {
		return err
	}
	for _, extension := range installed {
		if extension.ID != extensionID {
			continue
		}
		for _, profileID := range extension.AssignedProfileIDs {
			if d.core.Browser.IsRunning(profileID) {
				return errors.New("feche os perfis que usam esta extensão antes de desinstalá-la")
			}
		}
		break
	}
	return d.core.Extensions.Remove(d.context(), extensionID)
}

func (d *Desktop) listProfiles(ctx context.Context) ([]ProfileView, error) {
	profiles, _, err := d.listProfilesAndTelemetry(ctx)
	return profiles, err
}

func (d *Desktop) listProfilesAndTelemetry(ctx context.Context) ([]ProfileView, telemetry.Snapshot, error) {
	metadataList, err := d.core.Profiles.List(ctx)
	if err != nil {
		return nil, telemetry.Snapshot{}, err
	}
	states, err := d.telemetryStates(ctx, metadataList)
	if err != nil {
		return nil, telemetry.Snapshot{}, err
	}
	snapshot, err := d.core.Telemetry.Snapshot(ctx, states)
	if err != nil {
		return nil, telemetry.Snapshot{}, err
	}
	metricByProfile := make(map[string]telemetry.ProfileMetric, len(snapshot.Profiles))
	for _, metric := range snapshot.Profiles {
		metricByProfile[metric.ProfileID] = metric
	}
	result := make([]ProfileView, 0, len(metadataList))
	for _, metadata := range metadataList {
		view, err := d.profileView(ctx, metadata)
		if err != nil {
			return nil, telemetry.Snapshot{}, err
		}
		view.applyMetric(metricByProfile[metadata.ID])
		result = append(result, view)
	}
	return result, snapshot, nil
}

func (d *Desktop) profileView(ctx context.Context, metadata domain.Metadata) (ProfileView, error) {
	settings, err := d.core.Network.Get(ctx, metadata.ID)
	if err != nil {
		return ProfileView{}, err
	}
	var proxyView *ProxyView
	if settings.Mode != network.ModeDirect {
		proxyView = &ProxyView{
			Mode: settings.Mode, Host: settings.Host, Port: settings.Port,
			Username: settings.Username, HasPassword: settings.HasPassword,
			BypassList: settings.BypassList,
			Endpoint:   fmt.Sprintf("%s://%s:%d", settings.Mode, settings.Host, settings.Port),
		}
	}
	return ProfileView{
		ID: metadata.ID, Name: metadata.Name, Color: metadata.Color,
		CreatedAt: metadata.CreatedAt, UpdatedAt: metadata.UpdatedAt,
		LastLaunchedAt: metadata.LastLaunchedAt, LaunchCount: metadata.LaunchCount,
		Platforms: metadata.Platforms, Status: metadata.Status, Tags: metadata.Tags,
		Notes: metadata.Notes, StartURL: metadata.StartURL, LastURL: metadata.LastURL,
		ExtensionPaths: metadata.ExtensionPaths, DNSPreset: settings.DNSPreset, Proxy: proxyView,
		Running: d.core.Browser.IsRunning(metadata.ID),
		Engine:  d.core.Browser.Engine(), Risk: "medium", RiskReasons: []string{},
	}, nil
}

func (d *Desktop) singleProfileView(ctx context.Context, metadata domain.Metadata) (ProfileView, error) {
	view, err := d.profileView(ctx, metadata)
	if err != nil {
		return ProfileView{}, err
	}
	states, err := d.telemetryStates(ctx, []domain.Metadata{metadata})
	if err != nil {
		return ProfileView{}, err
	}
	snapshot, err := d.core.Telemetry.Snapshot(ctx, states)
	if err != nil {
		return ProfileView{}, err
	}
	if len(snapshot.Profiles) == 1 {
		view.applyMetric(snapshot.Profiles[0])
	}
	return view, nil
}

func (d *Desktop) telemetryStates(ctx context.Context, metadataList []domain.Metadata) ([]telemetry.ProfileState, error) {
	engine := d.core.Browser.Engine()
	states := make([]telemetry.ProfileState, 0, len(metadataList))
	for _, metadata := range metadataList {
		settings, err := d.core.Network.Get(ctx, metadata.ID)
		if err != nil {
			return nil, err
		}
		health := d.core.Fingerprint.Health(ctx, metadata.ID)
		fingerprintReady := health.StandardReady
		states = append(states, telemetry.ProfileState{
			ID: metadata.ID, CreatedAt: metadata.CreatedAt, LastLaunchedAt: metadata.LastLaunchedAt,
			LaunchCount: metadata.LaunchCount, Running: d.core.Browser.IsRunning(metadata.ID), Engine: engine,
			FingerprintReady: fingerprintReady, ProxyConfigured: settings.Mode != network.ModeDirect,
		})
	}
	return states, nil
}

func (view *ProfileView) applyMetric(metric telemetry.ProfileMetric) {
	view.Engine = metric.Engine
	view.FingerprintScore = metric.FingerprintScore
	view.FingerprintLabel = metric.FingerprintLabel
	view.Risk = metric.Risk
	view.RiskReasons = metric.RiskReasons
	if view.Proxy != nil {
		view.Proxy.LatencyMs = metric.ProxyLatencyMs
	}
}

func (d *Desktop) requirePremium() error {
	user, err := d.loggedUser()
	if err != nil {
		return err
	}
	return d.core.License.RequireActive(d.context(), user.ID)
}

func (d *Desktop) requireAdmin() error {
	user, err := d.loggedUser()
	if err != nil {
		return err
	}
	if !user.IsAdmin {
		return errors.New("administrator access is required")
	}
	return nil
}

func (d *Desktop) loggedUser() (*account.User, error) {
	user, err := d.core.Account.Get(d.context())
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("Discord login is required")
	}
	return user, nil
}

func (d *Desktop) context() context.Context {
	if d.ctx == nil {
		return context.Background()
	}
	return d.ctx
}

func (input ProfileInput) fields(extensionPaths []string) profile.Fields {
	return profile.Fields{
		Name: input.Name, Color: input.Color, Platforms: input.Platforms,
		Status: input.Status, Tags: input.Tags, Notes: input.Notes,
		StartURL: input.StartURL, ExtensionPaths: extensionPaths,
	}
}
