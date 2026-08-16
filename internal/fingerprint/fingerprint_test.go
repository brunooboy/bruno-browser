package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"
)

func TestStoreKeepsOneStableFingerprintPerPhysicalProfile(t *testing.T) {
	profileStore, err := profile.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := profileStore.Create(context.Background(), profile.Fields{
		Name: "Instagram fingerprint", Color: "#36f58b",
		Platforms: []domain.Platform{domain.PlatformInstagram}, Status: domain.StatusStarting,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(profileStore)
	if err != nil {
		t.Fatal(err)
	}
	fixedTime := time.Date(2026, time.August, 5, 15, 0, 0, 0, time.UTC)
	store.clock = func() time.Time { return fixedTime }

	first, err := store.LoadOrCreate(context.Background(), metadata.ID)
	if err != nil {
		t.Fatalf("LoadOrCreate first: %v", err)
	}
	second, err := store.LoadOrCreate(context.Background(), metadata.ID)
	if err != nil {
		t.Fatalf("LoadOrCreate second: %v", err)
	}
	if first != second || first.Seed == "" || !first.CreatedAt.Equal(fixedTime) {
		t.Fatalf("fingerprint was not stable: first=%#v second=%#v", first, second)
	}
	if _, verified, err := store.InspectVerification(context.Background(), metadata.ID); err != nil || verified {
		t.Fatalf("unapplied fingerprint was reported as verified: verified=%v err=%v", verified, err)
	}
	if err := store.MarkVerified(context.Background(), metadata.ID, "153.0.7994.0"); err != nil {
		t.Fatalf("MarkVerified: %v", err)
	}
	verification, verified, err := store.InspectVerification(context.Background(), metadata.ID)
	if err != nil || !verified || verification.ProfileID != metadata.ID || verification.BrowserVersion != "153.0.7994.0" || !verification.VerifiedAt.Equal(fixedTime) {
		t.Fatalf("unexpected fingerprint verification: value=%#v verified=%v err=%v", verification, verified, err)
	}
	paths, err := profileStore.Paths(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(paths.Root, fingerprintFileName))
	if err != nil || info.IsDir() {
		t.Fatalf("fingerprint.json was not persisted: %v", err)
	}
}

func TestPhysicalProfilesReceiveIndependentStableFingerprints(t *testing.T) {
	profileStore, err := profile.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	firstMetadata, err := profileStore.Create(context.Background(), profile.Fields{
		Name: "Conta A", Color: "#36f58b", Status: domain.StatusStarting,
		Platforms: []domain.Platform{domain.PlatformInstagram},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondMetadata, err := profileStore.Create(context.Background(), profile.Fields{
		Name: "Conta B", Color: "#ff4d6d", Status: domain.StatusStarting,
		Platforms: []domain.Platform{domain.PlatformGoogle},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(profileStore)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.LoadOrCreate(context.Background(), firstMetadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.LoadOrCreate(context.Background(), secondMetadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProfileID == second.ProfileID || first.Seed == second.Seed {
		t.Fatalf("profiles shared fingerprint identity: first=%#v second=%#v", first, second)
	}
	firstAgain, err := store.LoadOrCreate(context.Background(), firstMetadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAgain != first {
		t.Fatalf("first fingerprint changed after second profile creation: first=%#v again=%#v", first, firstAgain)
	}
}

func TestIdentityAndScriptStayCoherentWithRunningChromium(t *testing.T) {
	generated, err := generate("profile-test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := BuildIdentity(generated, "Chrome/145.0.7632.12", "Mozilla/5.0 HeadlessChrome/145.0.7632.12")
	if err != nil {
		t.Fatalf("BuildIdentity: %v", err)
	}
	if identity.BrowserMajor != "145" || !strings.Contains(identity.UserAgent, "Chrome/145.0.7632.12") || strings.Contains(identity.UserAgent, "Headless") {
		t.Fatalf("runtime identity is inconsistent: %#v", identity)
	}
	script, err := BuildScript(identity)
	if err != nil {
		t.Fatalf("BuildScript: %v", err)
	}
	for _, expected := range []string{
		"CanvasRenderingContext2D", "WebGLRenderingContext", "AudioBuffer", "hardwareConcurrency", generated.Seed,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("fingerprint script is missing %q", expected)
		}
	}
	if strings.Contains(script, "%!") {
		t.Fatal("fingerprint script contains an invalid formatting expansion")
	}
}

func TestNavigationRecorderKeepsOnlyRestorablePages(t *testing.T) {
	var mu sync.Mutex
	var recorded []string
	recorder := newNavigationRecorder("profile", func(_ context.Context, _ string, rawURL string) error {
		mu.Lock()
		recorded = append(recorded, rawURL)
		mu.Unlock()
		return nil
	})
	recorder.submit("about:blank")
	recorder.submit("chrome://settings")
	recorder.submit("https://example.com/first")
	recorder.submit("https://example.com/final")
	recorder.close()

	mu.Lock()
	defer mu.Unlock()
	if len(recorded) == 0 || recorded[len(recorded)-1] != "https://example.com/final" {
		t.Fatalf("unexpected recorded navigations: %v", recorded)
	}
}
