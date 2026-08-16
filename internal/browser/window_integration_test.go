package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/fingerprint"
	"bruno-browser/internal/profile"
)

// This opt-in test opens one visible, temporary Chromium window. It validates
// the same manager path used by the Wails button without touching user data.
func TestLiveManagerLaunchesNamedProfileWindow(t *testing.T) {
	if os.Getenv("BRUNO_BROWSER_WINDOW_INTEGRATION") != "1" {
		t.Skip("set BRUNO_BROWSER_WINDOW_INTEGRATION=1 to run the visible window test")
	}
	executable, err := findStockChromiumForIntegration()
	if err != nil {
		t.Skipf("compatible Chromium is not installed: %v", err)
	}

	profileStore, err := profile.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := profileStore.Create(context.Background(), profile.Fields{
		Name: "Bruno Window Test", Color: "#36f58b",
		Platforms: []domain.Platform{domain.PlatformGoogle}, Status: domain.StatusStarting,
	})
	if err != nil {
		t.Fatal(err)
	}
	fingerprintStore, err := fingerprint.NewStore(profileStore)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := fingerprint.NewController(fingerprintStore, func(ctx context.Context, profileID, rawURL string) error {
		_, recordErr := profileStore.RecordLastURL(ctx, profileID, rawURL)
		return recordErr
	})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(profileStore, Config{
		ExecutablePath: executable,
		// Codex runs the child browser inside a restricted test sandbox. This
		// flag is limited to this opt-in test and is never used by the app.
		ExtraArguments: []string{"--no-sandbox", "--disable-search-engine-choice-screen", "--disable-breakpad", "--disable-crash-reporter"},
		AttachCDP: func(ctx context.Context, profileID, websocketURL, initialURL string) (CDPSession, error) {
			return controller.Attach(ctx, profileID, websocketURL, initialURL)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := manager.Launch(ctx, metadata.ID, "")
	if err != nil {
		t.Fatalf("launch visible named profile: %v", err)
	}
	defer func() {
		if manager.IsRunning(metadata.ID) {
			_ = manager.Stop(context.Background(), metadata.ID)
		}
	}()
	if info.ProfileName != metadata.Name || info.PID <= 0 || !manager.IsRunning(metadata.ID) {
		t.Fatalf("unexpected running process: %#v", info)
	}
	health := controller.Health(context.Background(), metadata.ID)
	if !health.StandardReady || health.Error != "" {
		t.Fatalf("Bruno CDP fingerprint was not verified: %#v", health)
	}

	paths, err := profileStore.Paths(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	preferences := readIdentityJSON(t, filepath.Join(paths.UserData, "Default", "Preferences"))
	profilePreferences := preferences["profile"].(map[string]any)
	if profilePreferences["name"] != metadata.Name {
		t.Fatalf("Chromium profile name was not persisted: %#v", profilePreferences)
	}

	if err := manager.Stop(ctx, metadata.ID); err != nil {
		t.Fatalf("close visible profile cleanly: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for manager.IsRunning(metadata.ID) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if manager.IsRunning(metadata.ID) {
		t.Fatal("profile process remained running after Browser.close")
	}
}
