package fingerprint

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"bruno-browser/internal/profile"
	"bruno-browser/internal/storage"
)

const (
	fingerprintFileName = "fingerprint.json"
	wayfernFileName     = "wayfern-fingerprint.json"
	maxFingerprintSize  = 256 << 10
)

type PathProvider interface {
	Paths(string) (profile.Paths, error)
}

// Inspect validates an existing Bruno CDP fingerprint without creating one.
func (store *Store) Inspect(ctx context.Context, profileID string) (Profile, bool, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	paths, err := store.paths.Paths(profileID)
	if err != nil {
		return Profile{}, false, err
	}
	stored, err := load(filepath.Join(paths.Root, fingerprintFileName))
	if errors.Is(err, os.ErrNotExist) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	return stored, true, nil
}

func (store *Store) LoadWayfern(ctx context.Context, profileID string) (map[string]any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	paths, err := store.paths.Paths(profileID)
	if err != nil {
		return nil, false, err
	}
	contents, err := os.ReadFile(filepath.Join(paths.Root, wayfernFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(contents) == 0 || len(contents) > maxFingerprintSize {
		return nil, false, errors.New("Wayfern fingerprint file has an invalid size")
	}
	var stored map[string]any
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, false, fmt.Errorf("decode Wayfern fingerprint: %w", err)
	}
	if len(stored) == 0 {
		return nil, false, errors.New("Wayfern fingerprint is empty")
	}
	return stored, true, nil
}

func (store *Store) SaveWayfern(ctx context.Context, profileID string, value map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(value) == 0 {
		return errors.New("Wayfern fingerprint is empty")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode Wayfern fingerprint: %w", err)
	}
	if len(encoded) > maxFingerprintSize {
		return errors.New("Wayfern fingerprint exceeds the size limit")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	paths, err := store.paths.Paths(profileID)
	if err != nil {
		return err
	}
	return storage.WriteJSONAtomic(filepath.Join(paths.Root, wayfernFileName), value, 0o600)
}

type Store struct {
	paths PathProvider
	clock func() time.Time
	mu    sync.Mutex
}

func NewStore(paths PathProvider) (*Store, error) {
	if paths == nil {
		return nil, errors.New("profile path provider is required")
	}
	return &Store{paths: paths, clock: time.Now}, nil
}

func (store *Store) LoadOrCreate(ctx context.Context, profileID string) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	paths, err := store.paths.Paths(profileID)
	if err != nil {
		return Profile{}, fmt.Errorf("resolve fingerprint path: %w", err)
	}
	path := filepath.Join(paths.Root, fingerprintFileName)
	stored, err := load(path)
	if err == nil {
		if stored.ProfileID != profileID {
			return Profile{}, errors.New("fingerprint profile id does not match its directory")
		}
		return stored, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Profile{}, err
	}

	created, err := generate(profileID, store.clock().UTC())
	if err != nil {
		return Profile{}, err
	}
	if err := storage.WriteJSONAtomic(path, created, 0o600); err != nil {
		return Profile{}, fmt.Errorf("write fingerprint: %w", err)
	}
	return created, nil
}

func load(path string) (Profile, error) {
	file, err := os.Open(path)
	if err != nil {
		return Profile{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxFingerprintSize+1))
	decoder.DisallowUnknownFields()
	var stored Profile
	if err := decoder.Decode(&stored); err != nil {
		return Profile{}, fmt.Errorf("decode fingerprint: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Profile{}, errors.New("fingerprint contains trailing JSON content")
	}
	if err := stored.Validate(); err != nil {
		return Profile{}, fmt.Errorf("validate fingerprint: %w", err)
	}
	return stored, nil
}

type localePreset struct {
	locale, acceptLanguage, timezone string
}

var localePresets = []localePreset{
	{locale: "pt-BR", acceptLanguage: "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7", timezone: "America/Sao_Paulo"},
	{locale: "en-US", acceptLanguage: "en-US,en;q=0.9", timezone: "America/New_York"},
	{locale: "en-US", acceptLanguage: "en-US,en;q=0.9", timezone: "America/Los_Angeles"},
	{locale: "es-ES", acceptLanguage: "es-ES,es;q=0.9,en;q=0.7", timezone: "Europe/Madrid"},
	{locale: "pt-PT", acceptLanguage: "pt-PT,pt;q=0.9,en;q=0.7", timezone: "Europe/Lisbon"},
}

type platformPreset struct {
	navigator, name, version, architecture, bitness string
	webgl                                           []string
}

func currentPlatformPreset() platformPreset {
	switch runtime.GOOS {
	case "darwin":
		architecture := "x86"
		if runtime.GOARCH == "arm64" {
			architecture = "arm"
		}
		return platformPreset{
			navigator: "MacIntel", name: "macOS", version: "14.5.0", architecture: architecture, bitness: "64",
			webgl: []string{
				"ANGLE (Apple, ANGLE Metal Renderer: Apple M1, Unspecified Version)",
				"ANGLE (Intel Inc., Intel(R) Iris(TM) Plus Graphics, OpenGL 4.1)",
			},
		}
	case "linux":
		return platformPreset{
			navigator: "Linux x86_64", name: "Linux", version: "6.6.0", architecture: "x86", bitness: "64",
			webgl: []string{
				"ANGLE (Intel, Mesa Intel(R) UHD Graphics 630 (CFL GT2), OpenGL 4.6)",
				"ANGLE (NVIDIA Corporation, NVIDIA GeForce GTX 1660/PCIe/SSE2, OpenGL 4.6)",
			},
		}
	default:
		return platformPreset{
			navigator: "Win32", name: "Windows", version: "15.0.0", architecture: "x86", bitness: "64",
			webgl: []string{
				"ANGLE (Intel, Intel(R) UHD Graphics 630 (0x00003E92) Direct3D11 vs_5_0 ps_5_0, D3D11)",
				"ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 SUPER (0x000021C4) Direct3D11 vs_5_0 ps_5_0, D3D11)",
				"ANGLE (AMD, AMD Radeon RX 6600 (0x000073FF) Direct3D11 vs_5_0 ps_5_0, D3D11)",
			},
		}
	}
}

func generate(profileID string, createdAt time.Time) (Profile, error) {
	seedBytes := make([]byte, 32)
	if _, err := rand.Read(seedBytes); err != nil {
		return Profile{}, fmt.Errorf("generate fingerprint seed: %w", err)
	}
	locale := localePresets[int(seedBytes[0])%len(localePresets)]
	platform := currentPlatformPreset()
	concurrencyValues := []int64{4, 8, 8, 12, 16}
	memoryValues := []int{8, 8, 16, 16, 32}
	generated := Profile{
		SchemaVersion: CurrentSchemaVersion, ProfileID: strings.TrimSpace(profileID),
		Seed: hex.EncodeToString(seedBytes), Locale: locale.locale,
		AcceptLanguage: locale.acceptLanguage, Timezone: locale.timezone,
		NavigatorPlatform: platform.navigator, Platform: platform.name,
		PlatformVersion: platform.version, Architecture: platform.architecture, Bitness: platform.bitness,
		HardwareConcurrency: concurrencyValues[int(seedBytes[1])%len(concurrencyValues)],
		DeviceMemory:        memoryValues[int(seedBytes[2])%len(memoryValues)],
		WebGLVendor:         "Google Inc. (" + vendorFromRenderer(platform.webgl[int(seedBytes[3])%len(platform.webgl)]) + ")",
		WebGLRenderer:       platform.webgl[int(seedBytes[3])%len(platform.webgl)],
		CreatedAt:           createdAt,
	}
	if err := generated.Validate(); err != nil {
		return Profile{}, err
	}
	return generated, nil
}

func vendorFromRenderer(renderer string) string {
	for _, vendor := range []string{"NVIDIA", "Intel", "AMD", "Apple"} {
		if strings.Contains(renderer, vendor) {
			return vendor
		}
	}
	return "Google"
}
