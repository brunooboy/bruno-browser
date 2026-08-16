package license

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateActivateAndRevalidate(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.clock = func() time.Time { return time.Unix(1_722_878_400, 0) }
	entry, err := service.Generate(context.Background(), "123456789012345678", Plan30Days)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Claims.ExpiresAt <= entry.Claims.CreatedAt || entry.Key == "" {
		t.Fatal("generated key is incomplete")
	}
	activation, err := service.Activate(context.Background(), entry.Key, "123456789012345678")
	if err != nil {
		t.Fatal(err)
	}
	if !activation.Activated || activation.Status != "active" {
		t.Fatalf("unexpected activation: %+v", activation)
	}
	if err := service.RequireActive(context.Background(), "123456789012345678"); err != nil {
		t.Fatal(err)
	}
	if err := service.RequireActive(context.Background(), "999999999999999999"); err == nil {
		t.Fatal("another account must not use the activation")
	}
}

func TestExpiredActivationIsRemoved(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_722_878_400, 0)
	service.clock = func() time.Time { return now }
	entry, err := service.Generate(context.Background(), "", Plan7Days)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(context.Background(), entry.Key, "123456789012345678"); err != nil {
		t.Fatal(err)
	}
	service.clock = func() time.Time { return now.AddDate(0, 0, 8) }
	status, err := service.Status(context.Background(), "123456789012345678")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "expired" || status.Activated {
		t.Fatalf("expected expired status, got %+v", status)
	}
}

func TestOneDayPlanExpiresExactlyAfterOneDay(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_722_878_400, 0).UTC()
	service.clock = func() time.Time { return now }
	entry, err := service.Generate(context.Background(), "123456789012345678", Plan1Day)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Claims.Plan != Plan1Day || entry.Claims.ExpiresAt != now.AddDate(0, 0, 1).Unix() {
		t.Fatalf("unexpected one-day claims: %+v", entry.Claims)
	}
	if _, err := service.Activate(context.Background(), entry.Key, "123456789012345678"); err != nil {
		t.Fatal(err)
	}
	service.clock = func() time.Time { return now.Add(24 * time.Hour) }
	status, err := service.Status(context.Background(), "123456789012345678")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "expired" || status.Activated {
		t.Fatalf("one-day activation did not expire: %+v", status)
	}
}

func TestCorruptActivationIsQuarantinedWithoutBlockingStartup(t *testing.T) {
	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, activationFileName)
	if err := os.WriteFile(path, []byte(`{"activation":{"status":"active"},"key":"broken"} trailing`), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(context.Background(), "123456789012345678")
	if err != nil {
		t.Fatalf("corrupt local activation must not block application startup: %v", err)
	}
	if status.Status != "none" || status.Activated {
		t.Fatalf("unexpected status for corrupt activation: %+v", status)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt activation file was not removed: %v", err)
	}
}
