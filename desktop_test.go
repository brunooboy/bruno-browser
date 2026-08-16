package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bruno-browser/internal/account"
	appcore "bruno-browser/internal/app"
	"bruno-browser/internal/license"
)

func TestPremiumAccessRequiresLicenseForAdministrator(t *testing.T) {
	dataRoot := t.TempDir()
	const adminID = "123456789012345678"

	accountService, err := account.New(dataRoot, account.Config{AdminID: adminID})
	if err != nil {
		t.Fatal(err)
	}
	licenseService, err := license.New(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	accountPayload, err := json.Marshal(account.User{ID: adminID, Username: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "account.json"), accountPayload, 0o600); err != nil {
		t.Fatal(err)
	}

	desktop := &Desktop{core: &appcore.Core{Account: accountService, License: licenseService}}
	if err := desktop.requirePremium(); !errors.Is(err, license.ErrNoActivePlan) {
		t.Fatalf("administrator without a key must be blocked, got %v", err)
	}

	entry, err := licenseService.Generate(context.Background(), adminID, license.Plan1Day)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := licenseService.Activate(context.Background(), entry.Key, adminID); err != nil {
		t.Fatal(err)
	}
	if err := desktop.requirePremium(); err != nil {
		t.Fatalf("activated administrator should be allowed: %v", err)
	}
	if err := licenseService.Deactivate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := desktop.requirePremium(); !errors.Is(err, license.ErrNoActivePlan) {
		t.Fatalf("removed key must block the next premium operation, got %v", err)
	}
}

func TestPremiumAccessExpiresWithoutRestartingTheApplication(t *testing.T) {
	dataRoot := t.TempDir()
	const accountID = "123456789012345678"
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	accountService, err := account.New(dataRoot, account.Config{})
	if err != nil {
		t.Fatal(err)
	}
	accountPayload, err := json.Marshal(account.User{ID: accountID, Username: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "account.json"), accountPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	licenseService, err := license.New(dataRoot, license.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := licenseService.Generate(context.Background(), accountID, license.Plan1Day)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := licenseService.Activate(context.Background(), entry.Key, accountID); err != nil {
		t.Fatal(err)
	}
	desktop := &Desktop{core: &appcore.Core{Account: accountService, License: licenseService}}
	if err := desktop.requirePremium(); err != nil {
		t.Fatalf("fresh one-day activation was rejected: %v", err)
	}
	now = now.Add(24 * time.Hour)
	if err := desktop.requirePremium(); !errors.Is(err, license.ErrNoActivePlan) {
		t.Fatalf("expired key must block the next premium operation without a restart, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "license.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired activation was not removed from disk: %v", err)
	}
}
