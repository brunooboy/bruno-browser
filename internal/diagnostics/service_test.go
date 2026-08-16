package diagnostics

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"bruno-browser/internal/account"
	"bruno-browser/internal/browser"
	"bruno-browser/internal/domain"
	"bruno-browser/internal/extensions"
	"bruno-browser/internal/license"
	"bruno-browser/internal/network"
	"bruno-browser/internal/profile"
	"bruno-browser/internal/updates"
)

func TestRunValidatesLocalSubsystemsAndDetectsDamagedNetworkState(t *testing.T) {
	service, profiles := createDiagnosticService(t)
	metadata, err := profiles.Create(context.Background(), profile.Fields{
		Name: "Perfil homologação", Color: "#42ff91",
		Platforms: []domain.Platform{domain.PlatformGoogle}, Status: domain.StatusStarting,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "attention" {
		t.Fatalf("report without an account should request attention, got %+v", report)
	}
	for _, check := range report.Checks {
		if check.Status == CheckFail {
			t.Fatalf("healthy local subsystem failed diagnostics: %+v", check)
		}
	}

	paths, err := profiles.Paths(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Root, "network.json"), []byte(`{"schemaVersion":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err = service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "blocked" || checkByID(report.Checks, "network").Status != CheckFail {
		t.Fatalf("damaged network state was not detected: %+v", report)
	}
}

func TestIncidentLogIsBoundedAndRecoversFromInvalidJSON(t *testing.T) {
	service, _ := createDiagnosticService(t)
	if err := os.WriteFile(service.logPath(), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Record(context.Background(), "Proxy Test!", errors.New("route failed via socks5://bruno:segredo@proxy.example:1080\nwithout exposing secrets")); err != nil {
		t.Fatal(err)
	}
	incidents, err := service.Incidents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 || incidents[0].Scope != "proxytest" || incidents[0].Message != "route failed via socks5://***@proxy.example:1080 without exposing secrets" {
		t.Fatalf("unexpected recovered incidents: %+v", incidents)
	}
	if err := service.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	incidents, err = service.Incidents(context.Background())
	if err != nil || len(incidents) != 0 {
		t.Fatalf("diagnostics log was not cleared: %+v, %v", incidents, err)
	}
}

func createDiagnosticService(t *testing.T) (*Service, *profile.Store) {
	t.Helper()
	root := t.TempDir()
	profiles, err := profile.NewStore(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	networkStore, err := network.NewStore(profiles, diagnosticProtector{})
	if err != nil {
		t.Fatal(err)
	}
	networkManager, err := network.NewManager(networkStore)
	if err != nil {
		t.Fatal(err)
	}
	enginePath := filepath.Join(root, "Bruno Engine.exe")
	if err := os.WriteFile(enginePath, []byte("MZ-diagnostic-fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	browserManager, err := browser.NewManager(profiles, browser.Config{ExecutablePath: enginePath})
	if err != nil {
		t.Fatal(err)
	}
	accountService, err := account.New(root, account.Config{})
	if err != nil {
		t.Fatal(err)
	}
	licenseService, err := license.New(root)
	if err != nil {
		t.Fatal(err)
	}
	extensionService, err := extensions.New(root, profiles)
	if err != nil {
		t.Fatal(err)
	}
	updateService, err := updates.New("")
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(root, profiles, browserManager, networkManager, extensionService, accountService, licenseService, updateService)
	if err != nil {
		t.Fatal(err)
	}
	return service, profiles
}

func checkByID(checks []Check, id string) Check {
	for _, check := range checks {
		if check.ID == id {
			return check
		}
	}
	return Check{}
}

type diagnosticProtector struct{}

func (diagnosticProtector) Protect(value []byte) (string, error) {
	return base64.StdEncoding.EncodeToString(value), nil
}

func (diagnosticProtector) Unprotect(value string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(value)
}
