package domain

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const CurrentMetadataVersion = 1

type Platform string

const (
	PlatformInstagram Platform = "instagram"
	PlatformX         Platform = "x"
	PlatformOutlook   Platform = "outlook"
	PlatformFacebook  Platform = "facebook"
	PlatformGoogle    Platform = "google"
	PlatformTikTok    Platform = "tiktok"
)

var supportedPlatforms = []Platform{
	PlatformInstagram,
	PlatformX,
	PlatformOutlook,
	PlatformFacebook,
	PlatformGoogle,
	PlatformTikTok,
}

type ProfileStatus string

const (
	StatusStarting ProfileStatus = "starting"
	StatusWarming  ProfileStatus = "warming"
	StatusFixed    ProfileStatus = "fixed"
	StatusFarm     ProfileStatus = "farm"
)

type TagKind string

const (
	TagKindStatus TagKind = "status"
	TagKindCustom TagKind = "custom"
)

type Tag struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Color string  `json:"color"`
	Kind  TagKind `json:"kind"`
}

// Metadata is the durable, human-readable profile record stored next to the
// Chromium user-data directory. Authentication secrets are intentionally not
// represented here; Chromium owns cookies and login state in UserDataDir.
type Metadata struct {
	SchemaVersion  int           `json:"schemaVersion"`
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Color          string        `json:"color"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
	LastLaunchedAt *time.Time    `json:"lastLaunchedAt,omitempty"`
	LaunchCount    uint64        `json:"launchCount"`
	Platforms      []Platform    `json:"platforms"`
	Status         ProfileStatus `json:"status"`
	Tags           []Tag         `json:"tags"`
	Notes          string        `json:"notes"`
	StartURL       string        `json:"startUrl,omitempty"`
	LastURL        string        `json:"lastUrl,omitempty"`
	ExtensionPaths []string      `json:"extensionPaths,omitempty"`
}

type Maturity struct {
	Age        time.Duration
	TotalHours int
	Days       int
	Hours      int
}

var (
	hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	idPattern       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
)

func (m Metadata) Validate() error {
	if m.SchemaVersion != CurrentMetadataVersion {
		return fmt.Errorf("unsupported metadata schema version %d", m.SchemaVersion)
	}
	if strings.TrimSpace(m.ID) == "" {
		return errors.New("profile id is required")
	}
	nameLength := len([]rune(strings.TrimSpace(m.Name)))
	if nameLength < 3 || nameLength > 80 {
		return errors.New("profile name must contain between 3 and 80 characters")
	}
	if !hexColorPattern.MatchString(m.Color) {
		return errors.New("profile color must use #RRGGBB format")
	}
	if m.CreatedAt.IsZero() || m.UpdatedAt.IsZero() {
		return errors.New("profile timestamps are required")
	}
	if m.UpdatedAt.Before(m.CreatedAt) {
		return errors.New("updatedAt cannot be before createdAt")
	}
	if !m.Status.Valid() {
		return fmt.Errorf("unsupported profile status %q", m.Status)
	}
	if len(m.Platforms) == 0 {
		return errors.New("at least one platform is required")
	}
	if err := validatePlatforms(m.Platforms); err != nil {
		return err
	}
	if err := validateTags(m.Tags); err != nil {
		return err
	}
	if err := ValidateStartURL(m.StartURL); err != nil {
		return err
	}
	if err := ValidateStartURL(m.LastURL); err != nil {
		return fmt.Errorf("invalid last URL: %w", err)
	}
	for _, extensionPath := range m.ExtensionPaths {
		if strings.TrimSpace(extensionPath) == "" {
			return errors.New("extension paths cannot be empty")
		}
	}
	return nil
}

func (m Metadata) Clone() Metadata {
	clone := m
	clone.Platforms = slices.Clone(m.Platforms)
	clone.Tags = slices.Clone(m.Tags)
	clone.ExtensionPaths = slices.Clone(m.ExtensionPaths)
	if m.LastLaunchedAt != nil {
		lastLaunchedAt := *m.LastLaunchedAt
		clone.LastLaunchedAt = &lastLaunchedAt
	}
	return clone
}

func (m Metadata) Maturity(now time.Time) Maturity {
	age := now.Sub(m.CreatedAt)
	if age < 0 {
		age = 0
	}
	totalHours := int(age / time.Hour)
	return Maturity{
		Age:        age,
		TotalHours: totalHours,
		Days:       totalHours / 24,
		Hours:      totalHours % 24,
	}
}

func (s ProfileStatus) Valid() bool {
	switch s {
	case StatusStarting, StatusWarming, StatusFixed, StatusFarm:
		return true
	default:
		return false
	}
}

func ValidateStartURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || rawURL == "about:blank" {
		return nil
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid start URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("start URL must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("start URL host is required")
	}
	return nil
}

func validatePlatforms(platforms []Platform) error {
	seen := make(map[Platform]struct{}, len(platforms))
	for _, platform := range platforms {
		if !slices.Contains(supportedPlatforms, platform) {
			return fmt.Errorf("unsupported platform %q", platform)
		}
		if _, exists := seen[platform]; exists {
			return fmt.Errorf("duplicate platform %q", platform)
		}
		seen[platform] = struct{}{}
	}
	return nil
}

func validateTags(tags []Tag) error {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if !idPattern.MatchString(tag.ID) {
			return fmt.Errorf("invalid tag id %q", tag.ID)
		}
		labelLength := len([]rune(strings.TrimSpace(tag.Label)))
		if labelLength < 2 || labelLength > 32 {
			return fmt.Errorf("tag %q label must contain between 2 and 32 characters", tag.ID)
		}
		if !hexColorPattern.MatchString(tag.Color) {
			return fmt.Errorf("tag %q color must use #RRGGBB format", tag.ID)
		}
		if tag.Kind != TagKindStatus && tag.Kind != TagKindCustom {
			return fmt.Errorf("tag %q has unsupported kind %q", tag.ID, tag.Kind)
		}
		if _, exists := seen[tag.ID]; exists {
			return fmt.Errorf("duplicate tag id %q", tag.ID)
		}
		seen[tag.ID] = struct{}{}
	}
	return nil
}
