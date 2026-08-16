package fingerprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"bruno-browser/internal/storage"
)

// RuntimeBaseline is read from the untouched initial about:blank target. It
// keeps privacy protections coherent with the actual Bruno Engine and host
// instead of inventing a locale, timezone, GPU or hardware combination.
type RuntimeBaseline struct {
	Locale              string   `json:"locale"`
	Languages           []string `json:"languages"`
	Timezone            string   `json:"timezone"`
	NavigatorPlatform   string   `json:"navigatorPlatform"`
	Platform            string   `json:"platform"`
	PlatformVersion     string   `json:"platformVersion"`
	Architecture        string   `json:"architecture"`
	Bitness             string   `json:"bitness"`
	HardwareConcurrency int64    `json:"hardwareConcurrency"`
	DeviceMemory        int      `json:"deviceMemory"`
	WebGLVendor         string   `json:"webglVendor"`
	WebGLRenderer       string   `json:"webglRenderer"`
}

const runtimeBaselineExpression = `(async () => {
  const hints = navigator.userAgentData && navigator.userAgentData.getHighEntropyValues
    ? await navigator.userAgentData.getHighEntropyValues(["architecture", "bitness", "platformVersion"])
    : {};
  let webglVendor = "";
  let webglRenderer = "";
  try {
    const canvas = document.createElement("canvas");
    const gl = canvas.getContext("webgl") || canvas.getContext("experimental-webgl");
    if (gl) {
      const debug = gl.getExtension("WEBGL_debug_renderer_info");
      webglVendor = String(gl.getParameter(debug ? debug.UNMASKED_VENDOR_WEBGL : gl.VENDOR) || "");
      webglRenderer = String(gl.getParameter(debug ? debug.UNMASKED_RENDERER_WEBGL : gl.RENDERER) || "");
    }
  } catch (_) {}
  return {
    locale: navigator.language || "",
    languages: Array.from(navigator.languages || []),
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "",
    navigatorPlatform: navigator.platform || "",
    platform: (navigator.userAgentData && navigator.userAgentData.platform) || "",
    platformVersion: hints.platformVersion || "",
    architecture: hints.architecture || "",
    bitness: hints.bitness || "",
    hardwareConcurrency: navigator.hardwareConcurrency || 0,
    deviceMemory: navigator.deviceMemory || 0,
    webglVendor,
    webglRenderer
  };
})()`

func readRuntimeBaseline(ctx context.Context, client *cdpClient, sessionID string) (RuntimeBaseline, error) {
	if err := client.Call(ctx, sessionID, "Runtime.enable", map[string]any{}, nil); err != nil {
		return RuntimeBaseline{}, fmt.Errorf("enable runtime baseline inspection: %w", err)
	}
	var evaluation struct {
		Result struct {
			Value RuntimeBaseline `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := client.Call(ctx, sessionID, "Runtime.evaluate", map[string]any{
		"expression": runtimeBaselineExpression, "awaitPromise": true, "returnByValue": true,
	}, &evaluation); err != nil {
		return RuntimeBaseline{}, fmt.Errorf("read native runtime baseline: %w", err)
	}
	if len(evaluation.ExceptionDetails) > 0 {
		return RuntimeBaseline{}, errors.New("native runtime baseline evaluation failed")
	}
	if strings.TrimSpace(evaluation.Result.Value.Locale) == "" || strings.TrimSpace(evaluation.Result.Value.NavigatorPlatform) == "" {
		return RuntimeBaseline{}, errors.New("Bruno Engine returned an incomplete runtime baseline")
	}
	return evaluation.Result.Value, nil
}

func (store *Store) AlignRuntime(ctx context.Context, profileID string, baseline RuntimeBaseline) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	paths, err := store.paths.Paths(profileID)
	if err != nil {
		return Profile{}, err
	}
	path := filepath.Join(paths.Root, fingerprintFileName)
	stored, err := load(path)
	if err != nil {
		return Profile{}, err
	}
	if stored.ProfileID != profileID {
		return Profile{}, errors.New("fingerprint profile id does not match its directory")
	}
	aligned := stored
	setString(&aligned.Locale, baseline.Locale)
	setString(&aligned.Timezone, baseline.Timezone)
	setString(&aligned.NavigatorPlatform, baseline.NavigatorPlatform)
	setString(&aligned.Platform, baseline.Platform)
	setString(&aligned.PlatformVersion, baseline.PlatformVersion)
	setString(&aligned.Architecture, baseline.Architecture)
	setString(&aligned.Bitness, baseline.Bitness)
	setString(&aligned.WebGLVendor, baseline.WebGLVendor)
	setString(&aligned.WebGLRenderer, baseline.WebGLRenderer)
	if acceptLanguage := acceptLanguageFrom(baseline.Languages, baseline.Locale); acceptLanguage != "" {
		aligned.AcceptLanguage = acceptLanguage
	}
	if baseline.HardwareConcurrency >= 2 && baseline.HardwareConcurrency <= 64 {
		aligned.HardwareConcurrency = baseline.HardwareConcurrency
	}
	if baseline.DeviceMemory >= 2 && baseline.DeviceMemory <= 64 {
		aligned.DeviceMemory = baseline.DeviceMemory
	}
	if err := aligned.Validate(); err != nil {
		return Profile{}, fmt.Errorf("validate native-aligned fingerprint: %w", err)
	}
	if aligned != stored {
		if err := storage.WriteJSONAtomic(path, aligned, 0o600); err != nil {
			return Profile{}, fmt.Errorf("persist native-aligned fingerprint: %w", err)
		}
	}
	return aligned, nil
}

func setString(destination *string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		*destination = value
	}
}

func acceptLanguageFrom(languages []string, locale string) string {
	values := make([]string, 0, len(languages)+1)
	seen := make(map[string]struct{}, len(languages)+1)
	for _, language := range append([]string{locale}, languages...) {
		language = strings.TrimSpace(language)
		key := strings.ToLower(language)
		if language == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, language)
	}
	parts := make([]string, 0, len(values))
	for index, language := range values {
		if index == 0 {
			parts = append(parts, language)
			continue
		}
		quality := 1.0 - float64(index)*0.1
		if quality < 0.5 {
			quality = 0.5
		}
		parts = append(parts, fmt.Sprintf("%s;q=%.1f", language, quality))
	}
	return strings.Join(parts, ",")
}
