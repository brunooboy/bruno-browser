package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRestoreSessionPreservesPreferences(t *testing.T) {
	userData := t.TempDir()
	defaultDirectory := filepath.Join(userData, "Default")
	if err := os.MkdirAll(defaultDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	preferencesPath := filepath.Join(defaultDirectory, "Preferences")
	if err := os.WriteFile(preferencesPath, []byte(`{"browser":{"theme":2},"session":{"existing":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRestoreSession(userData); err != nil {
		t.Fatalf("EnsureRestoreSession: %v", err)
	}
	payload, err := os.ReadFile(preferencesPath)
	if err != nil {
		t.Fatal(err)
	}
	var preferences map[string]any
	if err := json.Unmarshal(payload, &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences["browser"].(map[string]any)["theme"] != float64(2) {
		t.Fatal("existing preferences were not preserved")
	}
	if preferences["session"].(map[string]any)["restore_on_startup"] != float64(1) {
		t.Fatal("restore_on_startup was not enabled")
	}
}

func TestEnsureControlledStartupDefersRestoreUntilCDPIsReady(t *testing.T) {
	userData := t.TempDir()
	if err := EnsureControlledStartup(userData); err != nil {
		t.Fatalf("EnsureControlledStartup: %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(userData, "Default", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var preferences map[string]any
	if err := json.Unmarshal(payload, &preferences); err != nil {
		t.Fatal(err)
	}
	if preferences["session"].(map[string]any)["restore_on_startup"] != float64(5) {
		t.Fatal("controlled startup did not select the neutral new-tab mode")
	}
}

func TestHasPreviousSession(t *testing.T) {
	userData := t.TempDir()
	sessionsDirectory := filepath.Join(userData, "Default", "Sessions")
	if err := os.MkdirAll(sessionsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDirectory, "Session_123"), []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	hasSession, err := HasPreviousSession(userData)
	if err != nil {
		t.Fatalf("HasPreviousSession: %v", err)
	}
	if !hasSession {
		t.Fatal("expected the Chromium session to be detected")
	}
}
