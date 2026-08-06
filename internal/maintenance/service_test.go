package maintenance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"
)

func TestClearHistoryAndCachePreservesAuthentication(t *testing.T) {
	store, metadata, paths := createTestProfile(t)
	processes := newFakeProcessState()
	service, err := NewService(store, processes)
	if err != nil {
		t.Fatal(err)
	}

	removedPaths := map[string]string{
		"history": filepath.Join(paths.UserData, "Default", "History"),
		"cache":   filepath.Join(paths.UserData, "Default", "Cache", "cache.bin"),
		"gpu":     filepath.Join(paths.UserData, "GPUCache", "gpu.bin"),
	}
	for _, path := range removedPaths {
		writeTestFile(t, path, "temporary")
	}
	preservedPaths := map[string]string{
		"cookies": filepath.Join(paths.UserData, "Default", "Network", "Cookies"),
		"local":   filepath.Join(paths.UserData, "Default", "Local Storage", "leveldb", "auth.ldb"),
		"session": filepath.Join(paths.UserData, "Default", "Sessions", "Session_123"),
	}
	for _, path := range preservedPaths {
		writeTestFile(t, path, "authenticated")
	}

	report, err := service.ClearHistoryAndCache(context.Background(), metadata.ID)
	if err != nil {
		t.Fatalf("ClearHistoryAndCache: %v", err)
	}
	if report.Operation != OperationClearHistoryAndCache || report.BytesFreed == 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for label, path := range removedPaths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s was not removed: %v", label, err)
		}
	}
	for label, path := range preservedPaths {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s authentication data was removed: %v", label, err)
		}
	}
	if _, err := store.Get(context.Background(), metadata.ID); err != nil {
		t.Fatalf("metadata must remain readable: %v", err)
	}
}

func TestClearCookiesAndSessionPreservesProfileStructure(t *testing.T) {
	store, metadata, paths := createTestProfile(t)
	service, err := NewService(store, newFakeProcessState())
	if err != nil {
		t.Fatal(err)
	}

	removedPaths := []string{
		filepath.Join(paths.UserData, "Default", "Network", "Cookies"),
		filepath.Join(paths.UserData, "Default", "Local Storage", "leveldb", "auth.ldb"),
		filepath.Join(paths.UserData, "Default", "IndexedDB", "account.db"),
		filepath.Join(paths.UserData, "Default", "Sessions", "Session_456"),
		filepath.Join(paths.UserData, "Default", "Service Worker", "Database", "worker.db"),
	}
	for _, path := range removedPaths {
		writeTestFile(t, path, "session")
	}
	preservedPaths := []string{
		paths.Metadata,
		filepath.Join(paths.UserData, "Default", "History"),
		filepath.Join(paths.UserData, "Default", "Preferences"),
		filepath.Join(paths.UserData, "Default", "Bookmarks"),
		filepath.Join(paths.UserData, "Default", "Login Data"),
	}
	for _, path := range preservedPaths[1:] {
		writeTestFile(t, path, "preserved")
	}

	report, err := service.ClearCookiesAndSession(context.Background(), metadata.ID)
	if err != nil {
		t.Fatalf("ClearCookiesAndSession: %v", err)
	}
	if report.Operation != OperationClearCookiesSession || report.BytesFreed == 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for _, path := range removedPaths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("session target was not removed: %s (%v)", path, err)
		}
	}
	for _, path := range preservedPaths {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("profile structure was removed: %s (%v)", path, err)
		}
	}
}

func TestDeleteProfileRemovesOnlySelectedDirectory(t *testing.T) {
	store, first, firstPaths := createTestProfile(t)
	second, err := store.Create(context.Background(), testFields("Conta Google 02"))
	if err != nil {
		t.Fatal(err)
	}
	secondPaths, err := store.Paths(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(firstPaths.UserData, "Default", "Cookies"), "account")
	service, err := NewService(store, newFakeProcessState())
	if err != nil {
		t.Fatal(err)
	}

	report, err := service.DeleteProfile(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if report.Operation != OperationDeleteProfile || report.BytesFreed == 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if _, err := os.Lstat(firstPaths.Root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("selected profile still exists: %v", err)
	}
	if _, err := os.Stat(secondPaths.Metadata); err != nil {
		t.Fatalf("other profile was affected: %v", err)
	}
	if _, err := store.Get(context.Background(), first.ID); !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("deleted profile should be missing, got %v", err)
	}
}

func TestMaintenanceRejectsBusyProfileWithoutChangingFiles(t *testing.T) {
	store, metadata, paths := createTestProfile(t)
	target := filepath.Join(paths.UserData, "Default", "History")
	writeTestFile(t, target, "history")
	processes := newFakeProcessState()
	processes.busy[metadata.ID] = true
	service, err := NewService(store, processes)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ClearHistoryAndCache(context.Background(), metadata.ID)
	if !errors.Is(err, ErrProfileRunning) {
		t.Fatalf("expected ErrProfileRunning, got %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("busy profile was modified: %v", err)
	}
}

func TestSafeTargetPathRejectsEscape(t *testing.T) {
	if _, err := safeTargetPath(t.TempDir(), "../outside"); err == nil {
		t.Fatal("expected an escaping maintenance target to be rejected")
	}
}

type fakeProcessState struct {
	mu       sync.Mutex
	busy     map[string]bool
	reserved map[string]bool
}

func newFakeProcessState() *fakeProcessState {
	return &fakeProcessState{busy: make(map[string]bool), reserved: make(map[string]bool)}
}

func (state *fakeProcessState) BeginMaintenance(profileID string) (func(), bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.busy[profileID] || state.reserved[profileID] {
		return nil, false
	}
	state.reserved[profileID] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			state.mu.Lock()
			delete(state.reserved, profileID)
			state.mu.Unlock()
		})
	}, true
}

func createTestProfile(t *testing.T) (*profile.Store, domain.Metadata, profile.Paths) {
	t.Helper()
	store, err := profile.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Create(context.Background(), testFields("Conta Google 01"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := store.Paths(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	return store, metadata, paths
}

func testFields(name string) profile.Fields {
	return profile.Fields{
		Name:      name,
		Color:     "#36f58b",
		Platforms: []domain.Platform{domain.PlatformGoogle},
		Status:    domain.StatusStarting,
		StartURL:  "https://accounts.google.com/",
	}
}

func writeTestFile(t *testing.T, path, payload string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
}
