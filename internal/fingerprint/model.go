package fingerprint

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const CurrentSchemaVersion = 1

var seedPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Profile is a stable browser identity stored beside one physical Chromium
// profile. Values do not change between launches; only the Chromium version
// used to construct the runtime User-Agent follows browser updates.
type Profile struct {
	SchemaVersion       int       `json:"schemaVersion"`
	ProfileID           string    `json:"profileId"`
	Seed                string    `json:"seed"`
	Locale              string    `json:"locale"`
	AcceptLanguage      string    `json:"acceptLanguage"`
	Timezone            string    `json:"timezone"`
	NavigatorPlatform   string    `json:"navigatorPlatform"`
	Platform            string    `json:"platform"`
	PlatformVersion     string    `json:"platformVersion"`
	Architecture        string    `json:"architecture"`
	Bitness             string    `json:"bitness"`
	HardwareConcurrency int64     `json:"hardwareConcurrency"`
	DeviceMemory        int       `json:"deviceMemory"`
	WebGLVendor         string    `json:"webglVendor"`
	WebGLRenderer       string    `json:"webglRenderer"`
	CreatedAt           time.Time `json:"createdAt"`
}

func (profile Profile) Validate() error {
	if profile.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported fingerprint schema version %d", profile.SchemaVersion)
	}
	if strings.TrimSpace(profile.ProfileID) == "" {
		return errors.New("fingerprint profile id is required")
	}
	if !seedPattern.MatchString(profile.Seed) {
		return errors.New("fingerprint seed must contain 32 hexadecimal bytes")
	}
	for name, value := range map[string]string{
		"locale": profile.Locale, "acceptLanguage": profile.AcceptLanguage,
		"timezone": profile.Timezone, "navigatorPlatform": profile.NavigatorPlatform,
		"platform": profile.Platform, "platformVersion": profile.PlatformVersion,
		"architecture": profile.Architecture, "bitness": profile.Bitness,
		"webglVendor": profile.WebGLVendor, "webglRenderer": profile.WebGLRenderer,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("fingerprint %s is required", name)
		}
	}
	if profile.HardwareConcurrency < 2 || profile.HardwareConcurrency > 64 {
		return errors.New("fingerprint hardware concurrency is outside the safe range")
	}
	if profile.DeviceMemory < 2 || profile.DeviceMemory > 64 {
		return errors.New("fingerprint device memory is outside the safe range")
	}
	if profile.CreatedAt.IsZero() {
		return errors.New("fingerprint creation time is required")
	}
	return nil
}

// Identity contains the final values applied to one running Chromium target.
type Identity struct {
	Profile
	UserAgent           string
	BrowserVersion      string
	BrowserMajor        string
	PrimaryBrand        string
	PrimaryBrandVersion string
}
