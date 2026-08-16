package extensions

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"
)

func TestInstallAndAssignCRX3(t *testing.T) {
	root := t.TempDir()
	profiles, err := profile.NewStore(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := profiles.Create(context.Background(), profile.Fields{
		Name: "Perfil teste", Color: "#42ff91", Platforms: []domain.Platform{domain.PlatformGoogle}, Status: domain.StatusStarting,
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(root, profiles)
	if err != nil {
		t.Fatal(err)
	}
	crxPath := filepath.Join(root, "sample.crx")
	if err := os.WriteFile(crxPath, testCRX3(t), 0o600); err != nil {
		t.Fatal(err)
	}
	extension, err := service.InstallCRX(context.Background(), crxPath)
	if err != nil {
		t.Fatal(err)
	}
	if extension.Name != "Sample Extension" || extension.ManifestVersion != 3 {
		t.Fatalf("unexpected extension: %+v", extension)
	}
	extension, err = service.SetAssignments(context.Background(), extension.ID, []string{metadata.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(extension.AssignedProfileIDs) != 1 || extension.AssignedProfileIDs[0] != metadata.ID {
		t.Fatalf("unexpected assignments: %+v", extension.AssignedProfileIDs)
	}
	if err := service.Remove(context.Background(), extension.ID); err != nil {
		t.Fatalf("Remove assigned extension: %v", err)
	}
	updatedProfile, err := profiles.Get(context.Background(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedProfile.ExtensionPaths) != 0 {
		t.Fatalf("extension assignment was not removed: %+v", updatedProfile.ExtensionPaths)
	}
	if _, err := os.Stat(filepath.Join(root, "extensions", extension.ID)); !os.IsNotExist(err) {
		t.Fatalf("extension directory still exists: %v", err)
	}
	installed, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 0 {
		t.Fatalf("extension remained in library: %+v", installed)
	}
}

func TestAssignsExtensionToProfileCreatedAfterServiceInitialization(t *testing.T) {
	root := t.TempDir()
	profiles, err := profile.NewStore(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(root, profiles)
	if err != nil {
		t.Fatal(err)
	}
	crxPath := filepath.Join(root, "sample.crx")
	if err := os.WriteFile(crxPath, testCRX3(t), 0o600); err != nil {
		t.Fatal(err)
	}
	extension, err := service.InstallCRX(context.Background(), crxPath)
	if err != nil {
		t.Fatal(err)
	}

	metadata, err := profiles.Create(context.Background(), profile.Fields{
		Name: "Perfil criado depois", Color: "#42ff91",
		Platforms: []domain.Platform{domain.PlatformInstagram}, Status: domain.StatusStarting,
	})
	if err != nil {
		t.Fatal(err)
	}
	extension, err = service.SetAssignments(context.Background(), extension.ID, []string{metadata.ID})
	if err != nil {
		t.Fatalf("SetAssignments for newly created profile: %v", err)
	}
	if !slices.Equal(extension.AssignedProfileIDs, []string{metadata.ID}) {
		t.Fatalf("assigned profiles = %#v", extension.AssignedProfileIDs)
	}
	stored, err := profiles.Get(context.Background(), metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.ExtensionPaths) != 1 || stored.ExtensionPaths[0] != extension.Path {
		t.Fatalf("new profile extension paths = %#v", stored.ExtensionPaths)
	}
}

func TestBundledCRXIsImportedOnceAndCanStayUninstalled(t *testing.T) {
	root := t.TempDir()
	profiles, err := profile.NewStore(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(root, profiles)
	if err != nil {
		t.Fatal(err)
	}
	payload := testCRX3(t)
	hash := sha256.Sum256(payload)
	path := filepath.Join(root, "bundled.crx")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	extension, installed, err := service.EnsureBundled(context.Background(), path, hex.EncodeToString(hash[:]))
	if err != nil || !installed || !extension.Bundled {
		t.Fatalf("EnsureBundled first import: extension=%+v installed=%v err=%v", extension, installed, err)
	}
	if err := service.Remove(context.Background(), extension.ID); err != nil {
		t.Fatal(err)
	}
	if _, installed, err := service.EnsureBundled(context.Background(), path, hex.EncodeToString(hash[:])); err != nil || installed {
		t.Fatalf("removed bundled extension was silently restored: installed=%v err=%v", installed, err)
	}
	listed, err := service.List(context.Background())
	if err != nil || len(listed) != 0 {
		t.Fatalf("removed extension remains in the library: listed=%+v err=%v", listed, err)
	}
}

func testCRX3(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	manifest, err := writer.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Write([]byte(`{"manifest_version":3,"name":"Sample Extension","version":"1.0.0"}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	result := []byte{'C', 'r', '2', '4', 3, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(result[8:12], 0)
	return append(result, archive.Bytes()...)
}
