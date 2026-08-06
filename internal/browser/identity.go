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
// cover the Donut landing-page variants that Wayfern may visit on a brand-new
// user-data directory before Bruno takes control of the initial page.
var hiddenWayfernShortcuts = []string{
	"a9846c4b18ffd084675a983d194b45a6", // https://donutbrowser.com/
	"533d54915b68f87353bf907dfc4327e5", // https://donutbrowser.com
	"a11bd361b9da3734518ab321ce86a44b", // http://donutbrowser.com/
}

// EnsureProfileIdentity persists the Bruno profile name in Chromium's own
// profile files. Stock Chromium displays this identity in its profile UI;
// Wayfern additionally renders it directly in the browser toolbar.
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
		for _, shortcutHash := range hiddenWayfernShortcuts {
			blacklist[shortcutHash] = nil
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
