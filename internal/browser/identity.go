package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bruno-browser/internal/storage"
)

const chromiumIdentityFileLimit = 16 << 20

// Chromium keys NTP blacklist entries by the MD5 of the URL. These values
// cover legacy third-party landing shortcuts left by older Bruno builds.
var hiddenLegacyShortcuts = []string{
	"a9846c4b18ffd084675a983d194b45a6", // https://donutbrowser.com/
	"533d54915b68f87353bf907dfc4327e5", // https://donutbrowser.com
	"a11bd361b9da3734518ab321ce86a44b", // http://donutbrowser.com/
}

func duckDuckGoSearchProvider() map[string]any {
	return map[string]any{
		"short_name":               "DuckDuckGo",
		"keyword":                  "duckduckgo.com",
		"url":                      "https://duckduckgo.com/?q={searchTerms}",
		"suggestions_url":          "https://duckduckgo.com/ac/?q={searchTerms}&type=list",
		"favicon_url":              "https://duckduckgo.com/favicon.ico",
		"input_encodings":          []string{"UTF-8"},
		"safe_for_autoreplace":     false,
		"preconnect_to_search_url": true,
		"is_active":                1,
	}
}

// EnsureProfileIdentity persists the Bruno profile name in Chromium's own
// profile files and removes obsolete third-party marketing shortcuts.
func EnsureProfileIdentity(userDataDir, profileName string) error {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	if err := mergeChromiumJSON(filepath.Join(userDataDir, "Local State"), func(document map[string]any) {
		profile := childObject(document, "profile")
		infoCache := childObject(profile, "info_cache")
		defaultProfile := childObject(infoCache, "Default")
		defaultProfile["name"] = profileName
		defaultProfile["shortcut_name"] = profileName
		defaultProfile["is_using_default_name"] = false
		profile["last_used"] = "Default"
		profile["last_active_profiles"] = []string{"Default"}
	}); err != nil {
		return fmt.Errorf("write Chromium Local State identity: %w", err)
	}

	if err := mergeChromiumJSON(filepath.Join(userDataDir, "Default", "Preferences"), func(document map[string]any) {
		profile := childObject(document, "profile")
		profile["name"] = profileName
		profile["using_default_name"] = false
		ntp := childObject(document, "ntp")
		blacklist := childObject(ntp, "most_visited_blacklist")
		for _, shortcutHash := range hiddenLegacyShortcuts {
			blacklist[shortcutHash] = nil
		}
		bruno := childObject(document, "bruno_browser")
		if initialized, _ := bruno["search_provider_initialized"].(bool); !initialized {
			providerData := childObject(document, "default_search_provider_data")
			providerData["template_url_data"] = duckDuckGoSearchProvider()
			providerData["mirrored_template_url_data"] = duckDuckGoSearchProvider()
			bruno["search_provider_initialized"] = true
		}
	}); err != nil {
		return fmt.Errorf("write Chromium profile identity: %w", err)
	}
	return nil
}

func mergeChromiumJSON(path string, mutate func(map[string]any)) error {
	document := map[string]any{}
	contents, err := os.ReadFile(path)
	if err == nil {
		if len(contents) > chromiumIdentityFileLimit {
			return fmt.Errorf("file exceeds %d bytes", chromiumIdentityFileLimit)
		}
		if len(contents) > 0 {
			if err := json.Unmarshal(contents, &document); err != nil {
				return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	mutate(document)
	return storage.WriteJSONAtomic(path, document, 0o600)
}

func childObject(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key].(map[string]any); ok {
		return value
	}
	value := map[string]any{}
	parent[key] = value
	return value
}
