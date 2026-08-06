package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"bruno-browser/internal/storage"
)

const (
	maxChromiumPreferenceSize = 16 << 20
	webRTCRestrictedPolicy    = "disable_non_proxied_udp"
	dohTemplates              = "https://cloudflare-dns.com/dns-query https://dns.quad9.net/dns-query{?dns}"
)

func ApplyChromiumNetworkPreferences(userDataDir string, proxied bool) error {
	preferencesPath := filepath.Join(userDataDir, "Default", "Preferences")
	if err := mergeJSONFile(preferencesPath, func(preferences map[string]any) {
		webrtc := childMap(preferences, "webrtc")
		webrtc["ip_handling_policy"] = webRTCRestrictedPolicy
	}); err != nil {
		return fmt.Errorf("apply WebRTC privacy preference: %w", err)
	}

	localStatePath := filepath.Join(userDataDir, "Local State")
	if err := mergeJSONFile(localStatePath, func(localState map[string]any) {
		dns := childMap(localState, "dns_over_https")
		if proxied {
			dns["mode"] = "off"
			dns["templates"] = ""
		} else {
			dns["mode"] = "automatic"
			dns["templates"] = dohTemplates
		}
		asyncDNS := childMap(localState, "async_dns")
		asyncDNS["enabled"] = !proxied
	}); err != nil {
		return fmt.Errorf("apply DNS preference: %w", err)
	}
	return nil
}

func mergeJSONFile(path string, mutate func(map[string]any)) error {
	value := make(map[string]any)
	payload, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(payload) > maxChromiumPreferenceSize {
			return errors.New("Chromium preference file exceeds safe size")
		}
		if err := json.Unmarshal(payload, &value); err != nil {
			return fmt.Errorf("decode Chromium preference file: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		// A new profile has no preference files yet.
	default:
		return err
	}
	mutate(value)
	return storage.WriteJSONAtomic(path, value, 0o600)
}

func childMap(parent map[string]any, key string) map[string]any {
	child, ok := parent[key].(map[string]any)
	if !ok {
		child = make(map[string]any)
		parent[key] = child
	}
	return child
}
