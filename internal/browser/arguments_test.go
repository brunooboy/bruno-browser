package browser

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildArgumentsKeepsProfileOnDiskAndRestoresSession(t *testing.T) {
	userData := filepath.Join(t.TempDir(), "chromium")
	arguments, err := BuildArguments(LaunchOptions{
		UserDataDir: userData,
		StartURL:    "https://www.instagram.com/",
		Restore:     true,
	})
	if err != nil {
		t.Fatalf("BuildArguments: %v", err)
	}
	joined := strings.Join(arguments, "\n")
	for _, expected := range []string{"--user-data-dir=", "--profile-directory=Default", "--new-window", "--restore-last-session", "--disable-default-apps"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("missing %q in arguments: %v", expected, arguments)
		}
	}
	if arguments[len(arguments)-1] != "https://www.instagram.com/" {
		t.Fatalf("start URL must be the final argument: %v", arguments)
	}
}

func TestBuildArgumentsAddsWayfernProfileIdentity(t *testing.T) {
	arguments, err := BuildArguments(LaunchOptions{
		UserDataDir:  t.TempDir(),
		WayfernLabel: "Conta Instagram 01",
		WayfernColor: "#36f58b",
	})
	if err != nil {
		t.Fatalf("BuildArguments: %v", err)
	}
	joined := strings.Join(arguments, "\n")
	for _, expected := range []string{"--wayfern-profile-label=Conta Instagram 01", "--wayfern-profile-color=36F58B"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("missing %q in arguments: %v", expected, arguments)
		}
	}
}

func TestBuildArgumentsRejectsPersistenceOverrides(t *testing.T) {
	_, err := BuildArguments(LaunchOptions{
		UserDataDir:    t.TempDir(),
		ExtraArguments: []string{"--incognito"},
	})
	if err == nil {
		t.Fatal("expected incognito argument to be rejected")
	}
}
