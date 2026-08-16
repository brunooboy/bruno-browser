package browser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"
)

func TestManagerLaunchesPersistentProfileAndTracksProcess(t *testing.T) {
	createdAt := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	store, err := profile.NewStore(t.TempDir(), profile.WithClock(func() time.Time { return createdAt }))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Create(context.Background(), profile.Fields{
		Name: "Instagram principal", Color: "#36f58b",
		Platforms: []domain.Platform{domain.PlatformInstagram}, Status: domain.StatusStarting,
		StartURL: "https://www.instagram.com/",
	})
	if err != nil {
		t.Fatal(err)
	}
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	networkSession := &fakeNetworkSession{arguments: []string{
		"--proxy-server=http://127.0.0.1:49152",
		"--dns-prefetch-disable",
	}}
	manager, err := NewManager(store, Config{
		ExecutablePath: testExecutable,
		PrepareNetwork: func(context.Context, string, string) (NetworkSession, error) {
			return networkSession, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.clock = func() time.Time { return createdAt.Add(time.Minute) }
	var capturedArguments []string
	manager.command = func(_ string, arguments ...string) *exec.Cmd {
		capturedArguments = append([]string(nil), arguments...)
		command := exec.Command(testExecutable, "-test.run=^TestBrowserHelperProcess$")
		command.Env = append(os.Environ(), "BRUNO_BROWSER_TEST_HELPER=1")
		return command
	}

	info, err := manager.Launch(context.Background(), metadata.ID, "")
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if info.ProfileID != metadata.ID || info.PID <= 0 || !manager.IsRunning(strings.ToUpper(metadata.ID)) {
		t.Fatalf("unexpected running process: %#v", info)
	}
	if _, err := manager.Launch(context.Background(), strings.ToUpper(metadata.ID), ""); !errors.Is(err, ErrProfileAlreadyRunning) {
		t.Fatalf("expected duplicate launch protection, got %v", err)
	}
	joined := strings.Join(capturedArguments, "\n")
	if !strings.Contains(joined, "--user-data-dir=") || !strings.Contains(joined, "--restore-last-session") {
		t.Fatalf("persistent launch arguments missing: %v", capturedArguments)
	}
	if !strings.Contains(joined, "--proxy-server=http://127.0.0.1:49152") || !strings.Contains(joined, "--dns-prefetch-disable") {
		t.Fatalf("managed network arguments missing: %v", capturedArguments)
	}
	if capturedArguments[len(capturedArguments)-1] != metadata.StartURL {
		t.Fatalf("first launch did not use configured account URL: %v", capturedArguments)
	}

	deadline := time.Now().Add(3 * time.Second)
	for manager.IsRunning(metadata.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if manager.IsRunning(metadata.ID) {
		t.Fatal("helper browser process did not exit")
	}
	for !networkSession.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !networkSession.closed.Load() {
		t.Fatal("network session was not closed with the Chromium process")
	}
	releaseMaintenance, ok := manager.BeginMaintenance(strings.ToUpper(metadata.ID))
	if !ok {
		t.Fatal("expected closed profile to be reservable for maintenance")
	}
	if _, err := manager.Launch(context.Background(), metadata.ID, ""); !errors.Is(err, ErrProfileUnderMaintenance) {
		t.Fatalf("expected launch to be blocked during maintenance, got %v", err)
	}
	releaseMaintenance()
	releaseMaintenance()
	stored, err := store.Get(context.Background(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LaunchCount != 1 || stored.LastLaunchedAt == nil {
		t.Fatalf("launch metadata not persisted: %#v", stored)
	}
}

func TestManagerAppliesCDPBeforeRestoringLastPage(t *testing.T) {
	createdAt := time.Date(2026, time.August, 5, 14, 0, 0, 0, time.UTC)
	store, err := profile.NewStore(t.TempDir(), profile.WithClock(func() time.Time { return createdAt }))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Create(context.Background(), profile.Fields{
		Name: "Conta Google CDP", Color: "#36f58b",
		Platforms: []domain.Platform{domain.PlatformGoogle}, Status: domain.StatusWarming,
		StartURL: "https://accounts.google.com/",
	})
	if err != nil {
		t.Fatal(err)
	}
	lastURL := "https://myaccount.google.com/security"
	if _, err := store.RecordLastURL(context.Background(), metadata.ID, lastURL); err != nil {
		t.Fatal(err)
	}
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cdpSession := &fakeCDPSession{}
	var attachedProfileID, attachedEndpoint, attachedInitialURL string
	manager, err := NewManager(store, Config{
		ExecutablePath: testExecutable,
		AttachCDP: func(_ context.Context, profileID, endpoint, initialURL string) (CDPSession, error) {
			attachedProfileID, attachedEndpoint, attachedInitialURL = profileID, endpoint, initialURL
			return cdpSession, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.waitDevTools = func(context.Context, string) (string, error) {
		return "ws://127.0.0.1:54444/devtools/browser/test", nil
	}
	var capturedArguments []string
	manager.command = func(_ string, arguments ...string) *exec.Cmd {
		capturedArguments = append([]string(nil), arguments...)
		command := exec.Command(testExecutable, "-test.run=^TestBrowserHelperProcess$")
		command.Env = append(os.Environ(), "BRUNO_BROWSER_TEST_HELPER=1")
		return command
	}

	if _, err := manager.Launch(context.Background(), metadata.ID, ""); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	joined := strings.Join(capturedArguments, "\n")
	for _, expected := range []string{
		"--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "about:blank",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("controlled CDP argument %q missing: %v", expected, capturedArguments)
		}
	}
	if strings.Contains(joined, "--restore-last-session") || capturedArguments[len(capturedArguments)-1] != "about:blank" {
		t.Fatalf("Chromium was allowed to navigate before CDP protection: %v", capturedArguments)
	}
	if attachedProfileID != metadata.ID || attachedInitialURL != lastURL || attachedEndpoint == "" {
		t.Fatalf("unexpected CDP attachment: profile=%s endpoint=%s initial=%s", attachedProfileID, attachedEndpoint, attachedInitialURL)
	}

	deadline := time.Now().Add(3 * time.Second)
	for manager.IsRunning(metadata.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	for !cdpSession.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !cdpSession.closed.Load() {
		t.Fatal("CDP session was not closed with Chromium")
	}
}

func TestBrowserHelperProcess(t *testing.T) {
	if os.Getenv("BRUNO_BROWSER_TEST_HELPER") != "1" {
		return
	}
	time.Sleep(350 * time.Millisecond)
}

func TestBrunoEngineUsesProtectedNewTabWhenProfileHasNoPage(t *testing.T) {
	if actual := neutralControlledStartURL(); actual != brunoNewTabURL {
		t.Fatalf("unexpected Bruno Engine neutral page: %q", actual)
	}
}

type fakeNetworkSession struct {
	arguments []string
	closed    atomic.Bool
}

type fakeCDPSession struct {
	closed atomic.Bool
}

func (session *fakeCDPSession) Close() error {
	session.closed.Store(true)
	return nil
}

func (session *fakeNetworkSession) Arguments() []string {
	return append([]string(nil), session.arguments...)
}

func (session *fakeNetworkSession) Close() error {
	session.closed.Store(true)
	return nil
}
