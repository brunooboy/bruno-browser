package app

import (
	"context"
	"testing"

	"bruno-browser/internal/config"
	"bruno-browser/internal/domain"
	"bruno-browser/internal/network"
	"bruno-browser/internal/profile"
)

func TestCoreWiresProfilesNetworkBrowserAndMaintenance(t *testing.T) {
	applicationConfig, err := config.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	core, err := New(applicationConfig)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if core.Profiles == nil || core.Network == nil || core.Browser == nil || core.Maintenance == nil || core.Fingerprint == nil || core.Diagnostics == nil || core.Backups == nil {
		t.Fatalf("core services are incomplete: %#v", core)
	}
	metadata, err := core.Profiles.Create(context.Background(), profile.Fields{
		Name:      "Integrated network profile",
		Color:     "#36f58b",
		Platforms: []domain.Platform{domain.PlatformGoogle},
		Status:    domain.StatusStarting,
	})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := core.Network.Save(context.Background(), metadata.ID, network.SaveInput{
		Mode: network.ModeHTTP, Host: "proxy.example.com", Port: 8080,
	})
	if err != nil {
		t.Fatalf("save network settings through core: %v", err)
	}
	if settings.ProfileID != metadata.ID || settings.Mode != network.ModeHTTP {
		t.Fatalf("unexpected network settings: %#v", settings)
	}
}
