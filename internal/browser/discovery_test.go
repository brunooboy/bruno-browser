package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExecutableUsesExplicitFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chromium-test")
	if err := os.WriteFile(path, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	found, err := FindExecutable(path)
	if err != nil {
		t.Fatalf("FindExecutable: %v", err)
	}
	absolutePath, _ := filepath.Abs(path)
	if found != absolutePath {
		t.Fatalf("got %q, want %q", found, absolutePath)
	}
}

func TestFindBundledBrunoEngine(t *testing.T) {
	root := t.TempDir()
	enginePath := filepath.Join(root, "engine", "chrome-win", "chrome.exe")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(enginePath), "locales"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string][]byte{
		enginePath: []byte("MZ-test"),
		filepath.Join(filepath.Dir(enginePath), "chrome.dll"):           []byte("dll"),
		filepath.Join(filepath.Dir(enginePath), "resources.pak"):        []byte("pak"),
		filepath.Join(filepath.Dir(enginePath), "locales", "en-US.pak"): []byte("locale"),
	} {
		if err := os.WriteFile(path, contents, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	found, ok := findBundledBrunoEngineInRoots([]string{root})
	if !ok {
		t.Fatal("bundled Bruno Engine was not found")
	}
	if found != enginePath {
		t.Fatalf("got %q, want bundled engine %q", found, enginePath)
	}
}

func TestIncompleteBundledBrunoEngineIsRejected(t *testing.T) {
	root := t.TempDir()
	enginePath := filepath.Join(root, "engine", "chrome-win", "chrome.exe")
	if err := os.MkdirAll(filepath.Dir(enginePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enginePath, []byte("MZ-test"), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, ok := findBundledBrunoEngineInRoots([]string{root}); ok {
		t.Fatal("incomplete Bruno Engine must not be accepted")
	}
}
