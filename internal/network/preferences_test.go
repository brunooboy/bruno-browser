package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyChromiumNetworkPreferencesForDirectConnection(t *testing.T) {
	userData := t.TempDir()
	preferencesPath := filepath.Join(userData, "Default", "Preferences")
	writeNetworkTestFile(t, preferencesPath, `{"browser":{"theme":2}}`)
	if err := ApplyChromiumNetworkPreferences(userData, false); err != nil {
		t.Fatal(err)
	}
	preferences := readJSONMap(t, preferencesPath)
	if preferences["browser"].(map[string]any)["theme"] != float64(2) {
		t.Fatal("existing Chromium preferences were not preserved")
	}
	if preferences["webrtc"].(map[string]any)["ip_handling_policy"] != webRTCRestrictedPolicy {
		t.Fatal("WebRTC privacy policy was not applied")
	}
	localState := readJSONMap(t, filepath.Join(userData, "Local State"))
	dns := localState["dns_over_https"].(map[string]any)
	if dns["mode"] != "automatic" || !strings.Contains(dns["templates"].(string), "dns.quad9.net") {
		t.Fatalf("automatic DNS was not configured: %#v", dns)
	}
}

func TestApplyChromiumNetworkPreferencesForProxy(t *testing.T) {
	userData := t.TempDir()
	if err := ApplyChromiumNetworkPreferences(userData, true); err != nil {
		t.Fatal(err)
	}
	localState := readJSONMap(t, filepath.Join(userData, "Local State"))
	dns := localState["dns_over_https"].(map[string]any)
	if dns["mode"] != "off" || dns["templates"] != "" {
		t.Fatalf("local DNS must be disabled for proxy routes: %#v", dns)
	}
	if localState["async_dns"].(map[string]any)["enabled"] != false {
		t.Fatal("async local DNS must be disabled for proxy routes")
	}
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func writeNetworkTestFile(t *testing.T, path, payload string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
}
