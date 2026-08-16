package backups

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bruno-browser/internal/browser"
	"bruno-browser/internal/domain"
	"bruno-browser/internal/extensions"
	"bruno-browser/internal/fingerprint"
	"bruno-browser/internal/network"
	"bruno-browser/internal/profile"
)

const backupTestPassword = "senha-segura-123"

type testProtector struct{}

func (testProtector) Protect(value []byte) (string, error) { return "protected:" + string(value), nil }
func (testProtector) Unprotect(value string) ([]byte, error) {
	return []byte(value[len("protected:"):]), nil
}

func TestContainerRejectsWrongPasswordAndTampering(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "payload.zip")
	if err := os.WriteFile(source, bytes.Repeat([]byte("bruno"), 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "profile.bruno-profile")
	if _, err := encryptFile(source, archive, backupTestPassword); err != nil {
		t.Fatal(err)
	}
	if err := decryptFile(archive, filepath.Join(root, "wrong.zip"), "senha-incorreta"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password error = %v", err)
	}
	payload, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0xff
	if err := os.WriteFile(archive, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := decryptFile(archive, filepath.Join(root, "tampered.zip"), backupTestPassword); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("tampered archive error = %v", err)
	}
}

func TestExportDeleteImportRoundTrip(t *testing.T) {
	service, profiles, networkManager, fingerprintStore, dataRoot := newTestService(t)
	ctx := context.Background()
	created, err := profiles.Create(ctx, validFields("Perfil migrado"))
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := profiles.Paths(created.ID)
	cookiePayload := []byte("cookie-and-session-state")
	if err := os.WriteFile(filepath.Join(paths.UserData, "Cookies"), cookiePayload, 0o600); err != nil {
		t.Fatal(err)
	}
	originalFingerprint, err := fingerprintStore.LoadOrCreate(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := networkManager.Save(ctx, created.ID, network.SaveInput{
		Mode: network.ModeSOCKS5, DNSPreset: network.DNSProPlus, Host: "127.0.0.1", Port: 1080,
		Username: "bruno", Password: "proxy-secret", BypassList: []string{"localhost"},
	}); err != nil {
		t.Fatal(err)
	}
	for name, secret := range map[string]string{"license.json": "LICENSE-MUST-NOT-LEAK", "account.json": "DISCORD-MUST-NOT-LEAK", "keys-history.json": "KEYS-MUST-NOT-LEAK"} {
		if err := os.WriteFile(filepath.Join(dataRoot, name), []byte(secret), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(t.TempDir(), "roundtrip.bruno-profile")
	if _, err := service.Export(ctx, []string{created.ID}, archive, backupTestPassword); err != nil {
		t.Fatal(err)
	}
	decrypted := filepath.Join(t.TempDir(), "roundtrip.zip")
	if err := decryptFile(archive, decrypted, backupTestPassword); err != nil {
		t.Fatal(err)
	}
	assertArchiveDoesNotContain(t, decrypted, []string{"LICENSE-MUST-NOT-LEAK", "DISCORD-MUST-NOT-LEAK", "KEYS-MUST-NOT-LEAK", "license.json", "account.json", "keys-history.json"})
	if err := profiles.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.Import(ctx, archive, backupTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Profiles) != 1 || result.Profiles[0].ID != created.ID || result.Profiles[0].Rekeyed {
		t.Fatalf("unexpected import result: %+v", result.Profiles)
	}
	restoredPaths, _ := profiles.Paths(created.ID)
	restoredCookies, err := os.ReadFile(filepath.Join(restoredPaths.UserData, "Cookies"))
	if err != nil || !bytes.Equal(restoredCookies, cookiePayload) {
		t.Fatalf("restored cookies = %q, %v", restoredCookies, err)
	}
	restoredNetwork, err := networkManager.RuntimeSettingsForBackup(ctx, created.ID)
	if err != nil || restoredNetwork.Password != "proxy-secret" || restoredNetwork.DNSPreset != network.DNSProPlus {
		t.Fatalf("restored network = %+v, %v", restoredNetwork, err)
	}
	restoredFingerprint, ok, err := fingerprintStore.Inspect(ctx, created.ID)
	if err != nil || !ok || restoredFingerprint.Seed != originalFingerprint.Seed {
		t.Fatalf("restored fingerprint = %+v, %v, %v", restoredFingerprint, ok, err)
	}
	history, err := service.History(ctx)
	if err != nil || len(history) != 2 || history[0].Operation != "import" || history[1].Operation != "export" {
		t.Fatalf("history = %+v, %v", history, err)
	}
}

func TestImportCollisionUsesNewUUID(t *testing.T) {
	service, profiles, _, _, _ := newTestService(t)
	ctx := context.Background()
	created, err := profiles.Create(ctx, validFields("Perfil original"))
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "collision.bruno-profile")
	if _, err := service.Export(ctx, []string{created.ID}, archive, backupTestPassword); err != nil {
		t.Fatal(err)
	}
	result, err := service.Import(ctx, archive, backupTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Profiles) != 1 || !result.Profiles[0].Rekeyed || result.Profiles[0].ID == created.ID {
		t.Fatalf("collision was not rekeyed: %+v", result.Profiles)
	}
	items, _ := profiles.List(ctx)
	if len(items) != 2 {
		t.Fatalf("profile count = %d", len(items))
	}
}

func TestImportRejectsPathTraversal(t *testing.T) {
	service, _, _, _, _ := newTestService(t)
	archive := encryptedZip(t, map[string][]byte{"../escape.txt": []byte("escape")})
	_, err := service.Import(context.Background(), archive, backupTestPassword)
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("traversal error = %v", err)
	}
}

func newTestService(t *testing.T) (*Service, *profile.Store, *network.Manager, *fingerprint.Store, string) {
	t.Helper()
	dataRoot := t.TempDir()
	profiles, err := profile.NewStore(filepath.Join(dataRoot, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	networkStore, err := network.NewStore(profiles, testProtector{})
	if err != nil {
		t.Fatal(err)
	}
	networkManager, err := network.NewManager(networkStore)
	if err != nil {
		t.Fatal(err)
	}
	browserManager, err := browser.NewManager(profiles, browser.Config{})
	if err != nil {
		t.Fatal(err)
	}
	extensionService, err := extensions.New(dataRoot, profiles)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(dataRoot, profiles, browserManager, networkManager, extensionService)
	if err != nil {
		t.Fatal(err)
	}
	fingerprintStore, err := fingerprint.NewStore(profiles)
	if err != nil {
		t.Fatal(err)
	}
	return service, profiles, networkManager, fingerprintStore, dataRoot
}

func validFields(name string) profile.Fields {
	return profile.Fields{
		Name: name, Color: "#42ff91", Platforms: []domain.Platform{domain.PlatformInstagram},
		Status: domain.StatusWarming, Tags: []domain.Tag{{ID: "warming", Label: "Aquecendo", Color: "#f5b942", Kind: domain.TagKindStatus}},
	}
}

func assertArchiveDoesNotContain(t *testing.T, archive string, forbidden []string) {
	t.Helper()
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		for _, value := range forbidden {
			if bytes.Contains([]byte(entry.Name), []byte(value)) {
				t.Fatalf("archive entry leaks %q", value)
			}
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(input)
		_ = input.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range forbidden {
			if bytes.Contains(payload, []byte(value)) {
				t.Fatalf("archive payload leaks %q", value)
			}
		}
	}
}

func encryptedZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	root := t.TempDir()
	zipPath := filepath.Join(root, "malicious.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, payload := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	_ = writer.Close()
	_ = file.Close()
	archive := filepath.Join(root, "malicious.bruno-profile")
	if _, err := encryptFile(zipPath, archive, backupTestPassword); err != nil {
		t.Fatal(err)
	}
	return archive
}

func validArchiveManifest(metadata domain.Metadata) []byte {
	payload, _ := json.Marshal(archiveManifest{
		SchemaVersion: CurrentSchemaVersion, AppVersion: AppVersion, ExportedAt: time.Now().UTC(),
		Profiles: []archiveProfile{{Metadata: metadata, Network: archiveNetwork{Mode: network.ModeDirect, DNSPreset: network.DNSNormal}}},
	})
	return payload
}
