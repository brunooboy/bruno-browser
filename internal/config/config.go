package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"bruno-browser/internal/storage"
)

const (
	dataDirectoryEnvironment = "BRUNO_BROWSER_DATA_DIR"
	browserPathEnvironment   = "BRUNO_BROWSER_EXECUTABLE"
	discordClientIDEnv       = "BRUNO_BROWSER_DISCORD_CLIENT_ID"
	discordClientSecretEnv   = "BRUNO_BROWSER_DISCORD_CLIENT_SECRET"
	adminDiscordIDEnv        = "BRUNO_BROWSER_ADMIN_DISCORD_ID"
	updateURLEnv             = "BRUNO_BROWSER_UPDATE_URL"
	settingsFileName         = "appconfig.json"
	legacySettingsFileName   = "config.json"
	defaultDiscordClientID   = "1534690836594425896"
	defaultUpdateURL         = "https://raw.githubusercontent.com/brunooboy/bruno-browser/main/version.json"
)

type FileSettings struct {
	DiscordClientID     string `json:"discordClientId"`
	DiscordClientSecret string `json:"discordClientSecret"`
	AdminDiscordID      string `json:"adminDiscordId"`
	UpdateURL           string `json:"updateUrl"`
}

// Config contains process-wide paths. Per-profile preferences live in each
// profile's metadata.json instead of in this structure.
type Config struct {
	DataRoot            string
	BrowserExecutable   string
	DiscordClientID     string
	DiscordClientSecret string
	AdminDiscordID      string
	UpdateURL           string
}

// Default returns an OS-appropriate, persistent configuration. No temporary
// or memory-backed directories are used for browser profile data.
func Default() (Config, error) {
	dataRoot := strings.TrimSpace(os.Getenv(dataDirectoryEnvironment))
	if dataRoot == "" {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve user configuration directory: %w", err)
		}
		dataRoot = filepath.Join(userConfigDir, "bruno-browser")
	}

	config, err := New(dataRoot, strings.TrimSpace(os.Getenv(browserPathEnvironment)))
	if err != nil {
		return Config{}, err
	}
	settings, err := loadOrCreateSettings(
		filepath.Join(config.DataRoot, settingsFileName),
		settingsImportCandidates()...,
	)
	if err != nil {
		return Config{}, err
	}
	config.DiscordClientID = firstNonEmpty(os.Getenv(discordClientIDEnv), settings.DiscordClientID, defaultDiscordClientID)
	config.DiscordClientSecret = firstNonEmpty(os.Getenv(discordClientSecretEnv), settings.DiscordClientSecret)
	config.AdminDiscordID = firstNonEmpty(os.Getenv(adminDiscordIDEnv), settings.AdminDiscordID)
	config.UpdateURL = resolveUpdateURL(os.Getenv(updateURLEnv), settings.UpdateURL)
	return config, nil
}

// New validates and normalizes explicit configuration values.
func New(dataRoot, browserExecutable string) (Config, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return Config{}, errors.New("data root is required")
	}

	absoluteDataRoot, err := filepath.Abs(filepath.Clean(dataRoot))
	if err != nil {
		return Config{}, fmt.Errorf("normalize data root: %w", err)
	}

	if browserExecutable != "" {
		browserExecutable, err = filepath.Abs(filepath.Clean(browserExecutable))
		if err != nil {
			return Config{}, fmt.Errorf("normalize browser executable: %w", err)
		}
	}

	return Config{
		DataRoot:          absoluteDataRoot,
		BrowserExecutable: browserExecutable,
	}, nil
}

func (c Config) ProfilesRoot() string {
	return filepath.Join(c.DataRoot, "profiles")
}

func (c Config) SettingsPath() string {
	return filepath.Join(c.DataRoot, settingsFileName)
}

func loadOrCreateSettings(path string, importCandidates ...string) (FileSettings, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		settings, err := importSettings(importCandidates)
		if err != nil {
			return FileSettings{}, err
		}
		if err := storage.WriteJSONAtomic(path, settings, 0o600); err != nil {
			return FileSettings{}, fmt.Errorf("create application settings: %w", err)
		}
		return settings, nil
	}
	if err != nil {
		return FileSettings{}, fmt.Errorf("open application settings: %w", err)
	}
	defer file.Close()
	return decodeSettings(file)
}

func settingsImportCandidates() []string {
	candidates := make([]string, 0, 3)
	if workingDirectory, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(workingDirectory, legacySettingsFileName))
	}
	if executable, err := os.Executable(); err == nil {
		executableDirectory := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDirectory, legacySettingsFileName),
			filepath.Join(executableDirectory, "..", "..", legacySettingsFileName),
		)
	}
	return candidates
}

func importSettings(candidates []string) (FileSettings, error) {
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = filepath.Clean(strings.TrimSpace(candidate))
		if candidate == "." || candidate == "" {
			continue
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}

		file, err := os.Open(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return FileSettings{}, fmt.Errorf("open imported application settings: %w", err)
		}
		settings, decodeErr := decodeSettings(file)
		closeErr := file.Close()
		if decodeErr != nil {
			return FileSettings{}, fmt.Errorf("decode imported application settings: %w", decodeErr)
		}
		if closeErr != nil {
			return FileSettings{}, fmt.Errorf("close imported application settings: %w", closeErr)
		}
		return settings, nil
	}
	return FileSettings{}, nil
}

func decodeSettings(reader io.Reader) (FileSettings, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 64<<10))
	decoder.DisallowUnknownFields()
	var settings FileSettings
	if err := decoder.Decode(&settings); err != nil {
		return FileSettings{}, err
	}
	settings.DiscordClientID = strings.TrimSpace(settings.DiscordClientID)
	settings.DiscordClientSecret = strings.TrimSpace(settings.DiscordClientSecret)
	settings.AdminDiscordID = strings.TrimSpace(settings.AdminDiscordID)
	settings.UpdateURL = strings.TrimSpace(settings.UpdateURL)
	return settings, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func normalizeUpdateURL(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	placeholders := []string{
		"example.com",
		"seu-usuario",
		"seu_usuario",
		"your-repo",
		"placeholder",
	}
	for _, placeholder := range placeholders {
		if strings.Contains(lower, placeholder) {
			return ""
		}
	}
	return value
}

func resolveUpdateURL(values ...string) string {
	for _, value := range values {
		if normalized := normalizeUpdateURL(value); normalized != "" {
			return normalized
		}
	}
	return defaultUpdateURL
}
