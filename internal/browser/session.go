package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bruno-browser/internal/storage"
)

const maxPreferencesSize = 16 << 20

func EnsureRestoreSession(userDataDir string) error {
	return setStartupPreference(userDataDir, 1)
}

// EnsureControlledStartup prevents Chromium from restoring a page before the
// CDP identity has been installed. The browser manager restores the last URL
// itself immediately after protection is active.
func EnsureControlledStartup(userDataDir string) error {
	return setStartupPreference(userDataDir, 5)
}

func setStartupPreference(userDataDir string, restoreOnStartup int) error {
	defaultDirectory := filepath.Join(userDataDir, "Default")
	if err := os.MkdirAll(defaultDirectory, 0o700); err != nil {
		return fmt.Errorf("create Default profile directory: %w", err)
	}
	preferencesPath := filepath.Join(defaultDirectory, "Preferences")
	preferences := make(map[string]any)

	payload, err := os.ReadFile(preferencesPath)
	switch {
	case err == nil:
		if len(payload) > maxPreferencesSize {
			return errors.New("Chromium Preferences file exceeds safe size")
		}
		if err := json.Unmarshal(payload, &preferences); err != nil {
			return fmt.Errorf("decode Chromium Preferences: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		// A minimal Preferences file is valid for a brand-new Chromium profile.
	default:
		return fmt.Errorf("read Chromium Preferences: %w", err)
	}

	session, ok := preferences["session"].(map[string]any)
	if !ok {
		session = make(map[string]any)
		preferences["session"] = session
	}
	session["restore_on_startup"] = restoreOnStartup

	if err := storage.WriteJSONAtomic(preferencesPath, preferences, 0o600); err != nil {
		return fmt.Errorf("write Chromium restore preference: %w", err)
	}
	return nil
}

func HasPreviousSession(userDataDir string) (bool, error) {
	defaultDirectory := filepath.Join(userDataDir, "Default")
	legacyPaths := []string{
		filepath.Join(defaultDirectory, "Last Session"),
		filepath.Join(defaultDirectory, "Current Session"),
		filepath.Join(defaultDirectory, "Last Tabs"),
		filepath.Join(defaultDirectory, "Current Tabs"),
	}
	for _, path := range legacyPaths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return true, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect Chromium session data: %w", err)
		}
	}

	sessionsDirectory := filepath.Join(defaultDirectory, "Sessions")
	entries, err := os.ReadDir(sessionsDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read Chromium Sessions directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "Session_") || strings.HasPrefix(name, "Tabs_") {
			return true, nil
		}
	}
	return false, nil
}
