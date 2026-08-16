package network

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"
)

func TestStoreProtectsAndPreservesProxyPassword(t *testing.T) {
	profiles, metadata := createNetworkTestProfile(t)
	clock := time.Date(2026, time.August, 5, 18, 0, 0, 0, time.UTC)
	store, err := NewStore(profiles, testProtector{}, WithClock(func() time.Time { return clock }))
	if err != nil {
		t.Fatal(err)
	}

	settings, err := store.Save(context.Background(), metadata.ID, SaveInput{
		Mode:       ModeSOCKS5,
		Host:       "Proxy.Example.com",
		Port:       1080,
		Username:   "operator",
		Password:   "secret-value",
		BypassList: []string{"LOCALHOST", "*.internal.example", "localhost"},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !settings.HasPassword || settings.Host != "Proxy.Example.com" {
		t.Fatalf("unexpected public settings: %#v", settings)
	}
	paths, _ := profiles.Paths(metadata.ID)
	payload, err := os.ReadFile(filepath.Join(paths.Root, networkFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "secret-value") {
		t.Fatal("network.json contains the plaintext proxy password")
	}

	runtimeSettings, err := store.Resolve(context.Background(), metadata.ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if runtimeSettings.Password != "secret-value" {
		t.Fatalf("resolved password = %q", runtimeSettings.Password)
	}

	settings, err = store.Save(context.Background(), metadata.ID, SaveInput{
		Mode: ModeSOCKS5, Host: "proxy.example.com", Port: 1081, Username: "operator",
	})
	if err != nil {
		t.Fatalf("Save preserving password: %v", err)
	}
	if !settings.HasPassword {
		t.Fatal("blank password update unexpectedly removed the stored password")
	}
	settings, err = store.Save(context.Background(), metadata.ID, SaveInput{
		Mode: ModeSOCKS5, Host: "proxy.example.com", Port: 1081, Username: "operator", ClearPassword: true,
	})
	if err != nil {
		t.Fatalf("Save clearing password: %v", err)
	}
	if settings.HasPassword {
		t.Fatal("ClearPassword did not remove the stored password")
	}
}

func TestStoreDefaultsToDirectAndDirectClearsProxy(t *testing.T) {
	profiles, metadata := createNetworkTestProfile(t)
	store, err := NewStore(profiles, testProtector{})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Get(context.Background(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Mode != ModeDirect || settings.DNSPreset != DNSNormal || settings.HasPassword {
		t.Fatalf("unexpected defaults: %#v", settings)
	}
	if _, err := store.Save(context.Background(), metadata.ID, SaveInput{
		Mode: ModeHTTP, Host: "127.0.0.1", Port: 8080, Username: "user", Password: "pass",
	}); err != nil {
		t.Fatal(err)
	}
	settings, err = store.Save(context.Background(), metadata.ID, SaveInput{Mode: ModeDirect, DNSPreset: DNSPro})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Mode != ModeDirect || settings.DNSPreset != DNSPro || settings.Host != "" || settings.HasPassword {
		t.Fatalf("direct mode retained proxy data: %#v", settings)
	}
}

func TestDefaultProtectorRoundTrip(t *testing.T) {
	protector, err := NewDefaultProtector(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	protected, err := protector.Protect([]byte("proxy password"))
	if err != nil {
		t.Fatal(err)
	}
	if protected == "" || strings.Contains(protected, "proxy password") {
		t.Fatalf("invalid protected value %q", protected)
	}
	plaintext, err := protector.Unprotect(protected)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "proxy password" {
		t.Fatalf("unprotected value = %q", plaintext)
	}
}

type testProtector struct{}

func (testProtector) Protect(value []byte) (string, error) {
	return base64.StdEncoding.EncodeToString(append([]byte("protected:"), value...)), nil
}

func (testProtector) Unprotect(value string) ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return []byte(strings.TrimPrefix(string(payload), "protected:")), nil
}

func createNetworkTestProfile(t *testing.T) (*profile.Store, domain.Metadata) {
	t.Helper()
	profiles, err := profile.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := profiles.Create(context.Background(), profile.Fields{
		Name:      "Proxy profile 01",
		Color:     "#36f58b",
		Platforms: []domain.Platform{domain.PlatformGoogle},
		Status:    domain.StatusStarting,
	})
	if err != nil {
		t.Fatal(err)
	}
	return profiles, metadata
}
