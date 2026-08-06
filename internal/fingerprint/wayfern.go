package fingerprint

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// AttachWayfern uses Wayfern's native fingerprint CDP domain. It deliberately
// avoids Page.addScriptToEvaluateOnNewDocument and Runtime.evaluate because the
// free engine classifies those commands as paid browser automation.
func (controller *Controller) AttachWayfern(ctx context.Context, profileID, websocketURL, initialURL string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(websocketURL) == "" {
		return nil, errors.New("DevTools websocket URL is required")
	}
	if !wayfernInitialURLAllowed(initialURL) {
		return nil, errors.New("initial Wayfern page must use http, https or the clean new-tab page")
	}
	baseIdentity, err := controller.store.LoadOrCreate(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("load persistent fingerprint: %w", err)
	}
	client, err := newCDPClient(ctx, websocketURL)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = client.Close()
		}
	}()

	operationContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	sessionID, err := attachFirstPage(operationContext, client)
	if err != nil {
		return nil, err
	}

	nativeFingerprint, exists, err := controller.store.LoadWayfern(operationContext, profileID)
	if err != nil {
		return nil, err
	}
	if !exists {
		refreshParameters := map[string]any{
			"operatingSystem": hostOperatingSystem(),
			"timezone":        baseIdentity.Timezone,
			"language":        baseIdentity.Locale,
		}
		if err := client.Call(operationContext, sessionID, "Wayfern.refreshFingerprint", refreshParameters, nil); err != nil {
			return nil, fmt.Errorf("generate native Wayfern fingerprint: %w", err)
		}
		var generated map[string]any
		if err := client.Call(operationContext, sessionID, "Wayfern.getFingerprint", map[string]any{}, &generated); err != nil {
			return nil, fmt.Errorf("read native Wayfern fingerprint: %w", err)
		}
		nativeFingerprint = fingerprintObject(generated)
		if len(nativeFingerprint) == 0 {
			return nil, errors.New("Wayfern returned an empty native fingerprint")
		}
		applyStableLocation(nativeFingerprint, baseIdentity)
	}

	var applied map[string]any
	if err := client.Call(operationContext, sessionID, "Wayfern.setFingerprint", nativeFingerprint, &applied); err != nil {
		return nil, fmt.Errorf("apply native Wayfern fingerprint: %w", err)
	}
	if actual := fingerprintObject(applied); len(actual) > 0 {
		nativeFingerprint = actual
	}
	if err := controller.store.SaveWayfern(operationContext, profileID, nativeFingerprint); err != nil {
		return nil, err
	}

	session := &Session{
		profileID:  profileID,
		client:     client,
		recorder:   newNavigationRecorder(profileID, controller.record),
		protecting: make(map[string]struct{}),
	}
	if initialURL != "" && initialURL != "about:blank" {
		if err := client.Call(operationContext, sessionID, "Page.navigate", map[string]any{"url": initialURL}, nil); err != nil {
			session.Close()
			return nil, fmt.Errorf("restore native Wayfern page: %w", err)
		}
		if restorableURL(initialURL) {
			session.recorder.submit(initialURL)
		}
	}
	failed = false
	return session, nil
}

func wayfernInitialURLAllowed(rawURL string) bool {
	return rawURL == "" || rawURL == "about:blank" || rawURL == "chrome://newtab" || rawURL == "chrome://newtab/" || restorableURL(rawURL)
}

func attachFirstPage(ctx context.Context, client *cdpClient) (string, error) {
	var targets struct {
		TargetInfos []targetInfo `json:"targetInfos"`
	}
	if err := client.Call(ctx, "", "Target.getTargets", map[string]any{}, &targets); err != nil {
		return "", fmt.Errorf("list Wayfern targets: %w", err)
	}
	targetID := ""
	for _, candidate := range targets.TargetInfos {
		if candidate.Type == "page" {
			targetID = candidate.TargetID
			if candidate.URL == "" || candidate.URL == "about:blank" {
				break
			}
		}
	}
	if targetID == "" {
		return "", errors.New("Wayfern did not expose an initial page target")
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := client.Call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, &attached); err != nil {
		return "", fmt.Errorf("attach initial Wayfern target: %w", err)
	}
	if attached.SessionID == "" {
		return "", errors.New("Wayfern returned an empty CDP session id")
	}
	return attached.SessionID, nil
}

func fingerprintObject(value map[string]any) map[string]any {
	if nested, ok := value["fingerprint"].(map[string]any); ok {
		return nested
	}
	return value
}

func applyStableLocation(value map[string]any, identity Profile) {
	value["timezone"] = identity.Timezone
	value["language"] = identity.Locale
	baseLanguage := strings.Split(identity.Locale, "-")[0]
	value["languages"] = []string{identity.Locale, baseLanguage}
	if location, err := time.LoadLocation(identity.Timezone); err == nil {
		_, offsetSeconds := time.Now().In(location).Zone()
		value["timezoneOffset"] = -(offsetSeconds / 60)
	}
}

func hostOperatingSystem() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "linux":
		return "linux"
	default:
		return "windows"
	}
}
