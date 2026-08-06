package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureProfileIdentityCreatesAndPreservesProfileFiles(t *testing.T) {
	userDataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(userDataDir, "Default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDataDir, "Local State"), []byte(`{"preserved":true,"profile":{"info_cache":{"Default":{"avatar_icon":"avatar"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDataDir, "Default", "Preferences"), []byte(`{"preserved":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureProfileIdentity(userDataDir, "Conta Instagram 01"); err != nil {
		t.Fatal(err)
	}

	localState := readIdentityJSON(t, filepath.Join(userDataDir, "Local State"))
	if localState["preserved"] != true {
		t.Fatalf("existing Local State data was not preserved: %#v", localState)
	}
	profile := localState["profile"].(map[string]any)
	infoCache := profile["info_cache"].(map[string]any)
	defaultProfile := infoCache["Default"].(map[string]any)
	if defaultProfile["name"] != "Conta Instagram 01" || defaultProfile["shortcut_name"] != "Conta Instagram 01" {
		t.Fatalf("unexpected Local State profile identity: %#v", defaultProfile)
	}
	if defaultProfile["avatar_icon"] != "avatar" {
		t.Fatalf("existing profile metadata was not preserved: %#v", defaultProfile)
	}

	preferences := readIdentityJSON(t, filepath.Join(userDataDir, "Default", "Preferences"))
	if preferences["preserved"] != true {
		t.Fatalf("existing Preferences data was not preserved: %#v", preferences)
	}
	preferenceProfile := preferences["profile"].(map[string]any)
	if preferenceProfile["name"] != "Conta Instagram 01" || preferenceProfile["using_default_name"] != false {
		t.Fatalf("unexpected Preferences profile identity: %#v", preferenceProfile)
	}
	blacklist := preferences["ntp"].(map[string]any)["most_visited_blacklist"].(map[string]any)
	for _, shortcutHash := range hiddenWayfernShortcuts {
		if _, exists := blacklist[shortcutHash]; !exists {
			t.Fatalf("Donut shortcut hash %s was not suppressed: %#v", shortcutHash, blacklist)
		}
	}
}

func readIdentityJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	return document
}
