//go:build windows

package updates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkHealthyOnlyWritesInsideUpdateHealthDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	marker := filepath.Join(dataRoot, "updates", "health", "startup.ok")
	if err := MarkHealthy(marker, dataRoot); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "healthy\n" {
		t.Fatalf("unexpected health marker payload %q, err=%v", payload, err)
	}
	if err := MarkHealthy(filepath.Join(dataRoot, "outside.ok"), dataRoot); err == nil {
		t.Fatal("expected an out-of-root marker to be rejected")
	}
}

func TestUpdaterArgumentHelpers(t *testing.T) {
	args := []string{"Bruno Browser.exe", helperSwitch, "--version=1.4.0", healthFilePrefix + `C:\safe\health.ok`}
	if !containsArgument(args, helperSwitch) || argumentValue(args, "--version=") != "1.4.0" {
		t.Fatalf("updater arguments were not parsed: %#v", args)
	}
	if UpdateHealthMarker(args) != `C:\safe\health.ok` {
		t.Fatalf("unexpected health marker %q", UpdateHealthMarker(args))
	}
	root := filepath.Join(t.TempDir(), "updates")
	if !pathWithin(root, filepath.Join(root, "downloads", "setup.exe")) || pathWithin(root, filepath.Dir(root)) {
		t.Fatal("update path containment failed")
	}
}
