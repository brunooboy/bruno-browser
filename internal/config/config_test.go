package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSettingsImportsLegacyConfig(t *testing.T) {
	root := t.TempDir()
	legacyPath := filepath.Join(root, legacySettingsFileName)
	targetPath := filepath.Join(root, "private", settingsFileName)
	payload := []byte(`{
  "discordClientId": "123456789012345678",
  "discordClientSecret": "private-value",
  "adminDiscordId": "987654321098765432",
  "updateUrl": ""
}`)
	if err := os.WriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	settings, err := loadOrCreateSettings(targetPath, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if settings.DiscordClientID != "123456789012345678" {
		t.Fatalf("unexpected imported client id %q", settings.DiscordClientID)
	}
	if settings.DiscordClientSecret != "private-value" {
		t.Fatal("client secret was not imported")
	}
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("private settings copy was not created: %v", err)
	}
}

func TestLoadOrCreateSettingsPrefersExistingPrivateConfig(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, settingsFileName)
	legacyPath := filepath.Join(root, legacySettingsFileName)
	if err := os.WriteFile(targetPath, []byte(`{"discordClientId":"private"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"discordClientId":"legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	settings, err := loadOrCreateSettings(targetPath, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if settings.DiscordClientID != "private" {
		t.Fatalf("existing private config was not preferred: %q", settings.DiscordClientID)
	}
}

func TestNormalizeUpdateURLIgnoresRepositoryPlaceholder(t *testing.T) {
	placeholder := "https://raw.githubusercontent.com/seu-usuario/bruno-browser/main/version.json"
	if normalized := normalizeUpdateURL(placeholder); normalized != "" {
		t.Fatalf("placeholder should be disabled, got %q", normalized)
	}

	realEndpoint := "https://updates.bruno.example.net/version.json"
	if normalized := normalizeUpdateURL(realEndpoint); normalized != realEndpoint {
		t.Fatalf("real endpoint changed to %q", normalized)
	}
}
