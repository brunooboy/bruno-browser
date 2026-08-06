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

func TestFindExecutableDiscoversDownloadedDonutWayfern(t *testing.T) {
	localAppData := t.TempDir()
	t.Setenv("LOCALAPPDATA", localAppData)
	wayfernPath := filepath.Join(localAppData, "DonutBrowser", "binaries", "wayfern", "151.0.1", "wayfern-win", "wayfern.exe")
	if err := os.MkdirAll(filepath.Dir(wayfernPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wayfernPath, []byte("MZ-test"), 0o700); err != nil {
		t.Fatal(err)
	}

	found, err := FindExecutable("")
	if err != nil {
		t.Fatalf("FindExecutable: %v", err)
	}
	if found != wayfernPath {
		t.Fatalf("got %q, want downloaded Wayfern %q", found, wayfernPath)
	}
}
