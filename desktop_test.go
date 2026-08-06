package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
