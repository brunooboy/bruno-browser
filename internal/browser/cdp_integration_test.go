package browser

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/fingerprint"
	"bruno-browser/internal/profile"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// This test is opt-in because it starts an installed Chromium executable. It
// always uses t.TempDir as --user-data-dir and never touches a personal browser
// profile. Run with BRUNO_BROWSER_INTEGRATION=1.
func TestLiveChromiumFingerprintIsAppliedBeforeNavigation(t *testing.T) {
	if os.Getenv("BRUNO_BROWSER_INTEGRATION") != "1" {
		t.Skip("set BRUNO_BROWSER_INTEGRATION=1 to run the live Chromium test")
	}
	executable, err := findStockChromiumForIntegration()
	if err != nil {
		t.Skipf("compatible Chromium is not installed: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte("<!doctype html><title>CDP integration</title><canvas id='canvas' width='16' height='16'></canvas>"))
	}))
	defer server.Close()

	profileStore, err := profile.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := profileStore.Create(context.Background(), profile.Fields{
		Name: "Live CDP profile", Color: "#36f58b",
		Platforms: []domain.Platform{domain.PlatformGoogle}, Status: domain.StatusStarting,
		StartURL: server.URL + "/account",
	})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := profileStore.Paths(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureControlledStartup(paths.UserData); err != nil {
		t.Fatal(err)
	}
	if err := clearDevToolsEndpoint(paths.UserData); err != nil {
		t.Fatal(err)
	}
	arguments, err := BuildArguments(LaunchOptions{
		UserDataDir: paths.UserData, StartURL: "about:blank", RemoteDebugging: true,
		ExtraArguments: []string{
			"--headless=new", "--disable-gpu", "--disable-software-rasterizer", "--no-sandbox",
			"--disable-search-engine-choice-screen", "--disable-breakpad", "--disable-crash-reporter",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, arguments...)
	var browserOutput bytes.Buffer
	command.Stdout = &browserOutput
	command.Stderr = &browserOutput
	if err := command.Start(); err != nil {
		t.Fatalf("start live Chromium: %v", err)
	}
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	endpoint, err := waitForDevToolsEndpoint(ctx, paths.UserData)
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
	session, err := controller.Attach(ctx, metadata.ID, endpoint, metadata.StartURL)
	if err != nil {
		t.Fatalf("attach live fingerprint controller: %v\nChromium output:\n%s", err, browserOutput.String())
	}
	defer session.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stored, getErr := profileStore.Get(context.Background(), metadata.ID)
		if getErr == nil && stored.LastURL == metadata.StartURL {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	stored, err := profileStore.Get(context.Background(), metadata.ID)
	if err != nil || stored.LastURL != metadata.StartURL {
		t.Fatalf("last page was not recorded after protected navigation: metadata=%#v err=%v", stored, err)
	}

	persistentIdentity, err := fingerprintStore.LoadOrCreate(context.Background(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	allocatorContext, allocatorCancel := chromedp.NewRemoteAllocator(ctx, endpoint, chromedp.NoModifyURL)
	defer allocatorCancel()
	browserContext, browserCancel := chromedp.NewContext(allocatorContext)
	defer browserCancel()
	targets, err := chromedp.Targets(browserContext)
	if err != nil {
		t.Fatalf("list live Chromium targets: %v", err)
	}
	var protectedTargetID string
	for _, candidate := range targets {
		if candidate.Type == "page" && candidate.URL == metadata.StartURL {
			protectedTargetID = string(candidate.TargetID)
			break
		}
	}
	if protectedTargetID == "" {
		t.Fatalf("protected page target was not found: %#v", targets)
	}
	inspectionContext, inspectionCancel := chromedp.NewContext(browserContext, chromedp.WithTargetID(target.ID(protectedTargetID)))
	defer inspectionCancel()
	var observation struct {
		Webdriver           bool   `json:"webdriver"`
		Language            string `json:"language"`
		Timezone            string `json:"timezone"`
		Platform            string `json:"platform"`
		HardwareConcurrency int64  `json:"hardwareConcurrency"`
		DeviceMemory        int    `json:"deviceMemory"`
		UserAgent           string `json:"userAgent"`
		CanvasStable        bool   `json:"canvasStable"`
	}
	expression := `(() => {
      const render = () => {
        const canvas = document.createElement("canvas"); canvas.width = 16; canvas.height = 16;
        const context = canvas.getContext("2d"); context.fillStyle = "#36f58b"; context.fillRect(0, 0, 16, 16);
        return canvas.toDataURL();
      };
      return {
        webdriver: navigator.webdriver === undefined,
        language: navigator.language,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        platform: navigator.platform,
        hardwareConcurrency: navigator.hardwareConcurrency,
        deviceMemory: navigator.deviceMemory || 0,
        userAgent: navigator.userAgent,
        canvasStable: render() === render()
      };
    })()`
	if err := chromedp.Run(inspectionContext, chromedp.Evaluate(expression, &observation)); err != nil {
		t.Fatalf("inspect protected page: %v", err)
	}
	if !observation.Webdriver || observation.Language != persistentIdentity.Locale ||
		observation.Timezone != persistentIdentity.Timezone || observation.Platform != persistentIdentity.NavigatorPlatform ||
		observation.HardwareConcurrency != persistentIdentity.HardwareConcurrency || observation.DeviceMemory != persistentIdentity.DeviceMemory ||
		!observation.CanvasStable || !strings.Contains(observation.UserAgent, "Chrome/") {
		t.Fatalf("live fingerprint differs from its persistent profile: observation=%#v identity=%#v", observation, persistentIdentity)
	}

	popupURL := server.URL + "/popup"
	var popupTargetID target.ID
	err = chromedp.Run(browserContext, chromedp.ActionFunc(func(actionContext context.Context) error {
		var createErr error
		popupTargetID, createErr = target.CreateTarget(popupURL).Do(actionContext)
		return createErr
	}))
	if err != nil {
		t.Fatalf("create protected popup target: %v", err)
	}
	var lastTargets []*target.Info
	popupDeadline := time.Now().Add(5 * time.Second)
	for popupTargetID == "" && time.Now().Before(popupDeadline) {
		candidates, targetsErr := chromedp.Targets(browserContext)
		if targetsErr != nil {
			t.Fatal(targetsErr)
		}
		lastTargets = candidates
		for _, candidate := range candidates {
			if candidate.Type == "page" && candidate.URL == popupURL {
				popupTargetID = candidate.TargetID
				break
			}
		}
		if popupTargetID == "" {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if popupTargetID == "" {
		details := make([]string, 0, len(lastTargets))
		for _, candidate := range lastTargets {
			details = append(details, "type="+candidate.Type+" url="+candidate.URL+" id="+string(candidate.TargetID))
		}
		t.Fatalf("popup did not resume after pre-navigation fingerprint protection: targets=%v protectionError=%v", details, session.LastProtectionError())
	}
	popupContext, popupCancel := chromedp.NewContext(browserContext, chromedp.WithTargetID(popupTargetID))
	defer popupCancel()
	var popupObservation struct {
		Webdriver           bool   `json:"webdriver"`
		Language            string `json:"language"`
		Timezone            string `json:"timezone"`
		HardwareConcurrency int64  `json:"hardwareConcurrency"`
	}
	if err := chromedp.Run(popupContext, chromedp.Evaluate(`({
      webdriver: navigator.webdriver === undefined,
      language: navigator.language,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      hardwareConcurrency: navigator.hardwareConcurrency
    })`, &popupObservation)); err != nil {
		t.Fatalf("inspect protected popup: %v", err)
	}
	if !popupObservation.Webdriver || popupObservation.Language != persistentIdentity.Locale ||
		popupObservation.Timezone != persistentIdentity.Timezone || popupObservation.HardwareConcurrency != persistentIdentity.HardwareConcurrency {
		t.Fatalf("popup escaped fingerprint protection: observation=%#v identity=%#v", popupObservation, persistentIdentity)
	}
}

func findStockChromiumForIntegration() (string, error) {
	if executable, err := FindExecutable(""); err == nil {
		return executable, nil
	}
	for _, candidate := range []string{
		filepath.Join("..", "..", "build", "bin", "engine", "chrome-win", "chrome.exe"),
		filepath.Join("build", "bin", "engine", "chrome-win", "chrome.exe"),
	} {
		if executable, err := validateBrunoEnginePath(candidate); err == nil {
			return executable, nil
		}
	}
	return "", ErrBrowserNotFound
}
