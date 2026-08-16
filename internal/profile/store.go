package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/storage"

	"github.com/google/uuid"
)

const (
	metadataFileName       = "metadata.json"
	metadataBackupFileName = "metadata.backup.json"
	userDataDirName        = "chromium"
	maxMetadataSize        = 1 << 20
)

var (
	ErrInvalidID       = errors.New("invalid profile id")
	ErrNotFound        = errors.New("profile not found")
	ErrMetadataCorrupt = errors.New("profile metadata is corrupt")
)

type Paths struct {
	Root           string
	Metadata       string
	MetadataBackup string
	UserData       string
	Extensions     string
}

type Fields struct {
	Name           string
	Color          string
	Platforms      []domain.Platform
	Status         domain.ProfileStatus
	Tags           []domain.Tag
	Notes          string
	StartURL       string
	ExtensionPaths []string
}

type Store struct {
	root  string
	clock func() time.Time
	mu    sync.RWMutex
}

type Option func(*Store)

func WithClock(clock func() time.Time) Option {
	return func(store *Store) {
		if clock != nil {
			store.clock = clock
		}
	}
}

func NewStore(root string, options ...Option) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("profiles root is required")
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("normalize profiles root: %w", err)
	}
	store := &Store{root: absoluteRoot, clock: time.Now}
	for _, option := range options {
		option(store)
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("create profiles root: %w", err)
	}
	return store, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) Create(ctx context.Context, fields Fields) (metadata domain.Metadata, err error) {
	if err := ctx.Err(); err != nil {
		return domain.Metadata{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock().UTC()
	metadata = domain.Metadata{
		SchemaVersion:  domain.CurrentMetadataVersion,
		ID:             uuid.NewString(),
		Name:           fields.Name,
		Color:          fields.Color,
		CreatedAt:      now,
		UpdatedAt:      now,
		Platforms:      slices.Clone(fields.Platforms),
		Status:         fields.Status,
		Tags:           slices.Clone(fields.Tags),
		Notes:          fields.Notes,
		StartURL:       fields.StartURL,
		ExtensionPaths: slices.Clone(fields.ExtensionPaths),
	}
	if err := normalizeMetadata(&metadata); err != nil {
		return domain.Metadata{}, err
	}
	if err := metadata.Validate(); err != nil {
		return domain.Metadata{}, fmt.Errorf("validate profile: %w", err)
	}

	paths, err := s.pathsForCanonicalID(metadata.ID)
	if err != nil {
		return domain.Metadata{}, err
	}
	if err := os.Mkdir(paths.Root, 0o700); err != nil {
		return domain.Metadata{}, fmt.Errorf("create profile directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(paths.Root)
		}
	}()

	if err := os.MkdirAll(paths.UserData, 0o700); err != nil {
		return domain.Metadata{}, fmt.Errorf("create Chromium user-data directory: %w", err)
	}
	if err := os.MkdirAll(paths.Extensions, 0o700); err != nil {
		return domain.Metadata{}, fmt.Errorf("create extensions directory: %w", err)
	}
	if err := s.writeMetadata(paths.Metadata, metadata); err != nil {
		return domain.Metadata{}, err
	}

	committed = true
	return metadata.Clone(), nil
}

func (s *Store) Get(ctx context.Context, id string) (domain.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return domain.Metadata{}, err
	}
	canonicalID, err := canonicalProfileID(id)
	if err != nil {
		return domain.Metadata{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked(canonicalID)
}

func (s *Store) List(ctx context.Context) ([]domain.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read profiles root: %w", err)
	}
	profiles := make([]domain.Metadata, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		canonicalID, err := canonicalProfileID(entry.Name())
		if err != nil || canonicalID != entry.Name() {
			continue
		}
		metadata, err := s.loadUnlocked(canonicalID)
		if err != nil {
			return nil, fmt.Errorf("load profile %s: %w", canonicalID, err)
		}
		profiles = append(profiles, metadata)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].CreatedAt.After(profiles[j].CreatedAt)
	})
	return profiles, nil
}

func (s *Store) Update(ctx context.Context, id string, fields Fields) (domain.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return domain.Metadata{}, err
	}
	canonicalID, err := canonicalProfileID(id)
	if err != nil {
		return domain.Metadata{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, err := s.loadUnlocked(canonicalID)
	if err != nil {
		return domain.Metadata{}, err
	}
	metadata.Name = fields.Name
	metadata.Color = fields.Color
	metadata.Platforms = slices.Clone(fields.Platforms)
	metadata.Status = fields.Status
	metadata.Tags = slices.Clone(fields.Tags)
	metadata.Notes = fields.Notes
	metadata.StartURL = fields.StartURL
	metadata.ExtensionPaths = slices.Clone(fields.ExtensionPaths)
	metadata.UpdatedAt = s.clock().UTC()
	if err := normalizeMetadata(&metadata); err != nil {
		return domain.Metadata{}, err
	}
	if err := metadata.Validate(); err != nil {
		return domain.Metadata{}, fmt.Errorf("validate profile update: %w", err)
	}

	paths, err := s.pathsForCanonicalID(canonicalID)
	if err != nil {
		return domain.Metadata{}, err
	}
	if err := s.writeMetadata(paths.Metadata, metadata); err != nil {
		return domain.Metadata{}, err
	}
	return metadata.Clone(), nil
}

func (s *Store) RecordLaunch(ctx context.Context, id string, launchedAt time.Time) (domain.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return domain.Metadata{}, err
	}
	canonicalID, err := canonicalProfileID(id)
	if err != nil {
		return domain.Metadata{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, err := s.loadUnlocked(canonicalID)
	if err != nil {
		return domain.Metadata{}, err
	}
	launchTime := launchedAt.UTC()
	metadata.LastLaunchedAt = &launchTime
	metadata.LaunchCount++
	metadata.UpdatedAt = launchTime
	if metadata.UpdatedAt.Before(metadata.CreatedAt) {
		metadata.UpdatedAt = metadata.CreatedAt
	}

	paths, err := s.pathsForCanonicalID(canonicalID)
	if err != nil {
		return domain.Metadata{}, err
	}
	if err := s.writeMetadata(paths.Metadata, metadata); err != nil {
		return domain.Metadata{}, err
	}
	return metadata.Clone(), nil
}

// RecordLastURL stores the last committed top-level HTTP(S) navigation. It is
// intentionally kept in metadata.json so a controlled CDP startup can apply
// the profile identity before restoring the page on the next launch.
func (s *Store) RecordLastURL(ctx context.Context, id, rawURL string) (domain.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return domain.Metadata{}, err
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || rawURL == "about:blank" {
		return domain.Metadata{}, errors.New("last URL must be an http or https page")
	}
	if err := domain.ValidateStartURL(rawURL); err != nil {
		return domain.Metadata{}, err
	}
	canonicalID, err := canonicalProfileID(id)
	if err != nil {
		return domain.Metadata{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, err := s.loadUnlocked(canonicalID)
	if err != nil {
		return domain.Metadata{}, err
	}
	if metadata.LastURL == rawURL {
		return metadata.Clone(), nil
	}
	metadata.LastURL = rawURL
	metadata.UpdatedAt = s.clock().UTC()
	if metadata.UpdatedAt.Before(metadata.CreatedAt) {
		metadata.UpdatedAt = metadata.CreatedAt
	}
	paths, err := s.pathsForCanonicalID(canonicalID)
	if err != nil {
		return domain.Metadata{}, err
	}
	if err := s.writeMetadata(paths.Metadata, metadata); err != nil {
		return domain.Metadata{}, err
	}
	return metadata.Clone(), nil
}

// Delete removes one complete profile directory after validating that the ID
// resolves to a profile created by this store. Process-state checks are owned
// by the maintenance service, which calls this method while holding its
// per-profile operation lock.
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	canonicalID, err := canonicalProfileID(id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.loadUnlocked(canonicalID); err != nil {
		return err
	}
	paths, err := s.pathsForCanonicalID(canonicalID)
	if err != nil {
		return err
	}
	info, err := os.Lstat(paths.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("inspect profile directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to delete a symbolic-link profile directory")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.RemoveAll(paths.Root); err != nil {
		return fmt.Errorf("delete profile directory: %w", err)
	}
	if _, err := os.Lstat(paths.Root); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return errors.New("profile directory still exists after deletion")
		}
		return fmt.Errorf("verify profile deletion: %w", err)
	}
	return nil
}

func (s *Store) Paths(id string) (Paths, error) {
	canonicalID, err := canonicalProfileID(id)
	if err != nil {
		return Paths{}, err
	}
	return s.pathsForCanonicalID(canonicalID)
}

// CommitImport atomically promotes a fully validated staging directory into
// the profile store. The staging directory must be a direct .restore-* child
// of the profiles root, which prevents callers from moving arbitrary paths.
// The original UUID is preserved when available; collisions receive a new
// UUID so an import can never overwrite an existing profile.
func (s *Store) CommitImport(ctx context.Context, source domain.Metadata, stagingRoot string, extensionPaths []string) (domain.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return domain.Metadata{}, err
	}
	absoluteStaging, err := filepath.Abs(filepath.Clean(stagingRoot))
	if err != nil {
		return domain.Metadata{}, fmt.Errorf("normalize import staging directory: %w", err)
	}
	relative, err := filepath.Rel(s.root, absoluteStaging)
	if err != nil || filepath.Dir(relative) != "." || !strings.HasPrefix(filepath.Base(relative), ".restore-") {
		return domain.Metadata{}, errors.New("import staging directory must be a direct .restore-* child of the profiles root")
	}
	info, err := os.Lstat(absoluteStaging)
	if err != nil {
		return domain.Metadata{}, fmt.Errorf("inspect import staging directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return domain.Metadata{}, errors.New("import staging path must be a real directory")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	targetID, err := canonicalProfileID(source.ID)
	if err != nil {
		targetID = uuid.NewString()
	}
	destination := filepath.Join(s.root, targetID)
	if _, err := os.Lstat(destination); err == nil {
		targetID = uuid.NewString()
		destination = filepath.Join(s.root, targetID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.Metadata{}, fmt.Errorf("inspect import destination: %w", err)
	}

	metadata := source.Clone()
	metadata.SchemaVersion = domain.CurrentMetadataVersion
	metadata.ID = targetID
	metadata.ExtensionPaths = slices.Clone(extensionPaths)
	if metadata.CreatedAt.IsZero() {
		metadata.CreatedAt = s.clock().UTC()
	}
	if metadata.UpdatedAt.IsZero() || metadata.UpdatedAt.Before(metadata.CreatedAt) {
		metadata.UpdatedAt = metadata.CreatedAt
	}
	if err := normalizeMetadata(&metadata); err != nil {
		return domain.Metadata{}, err
	}
	if err := metadata.Validate(); err != nil {
		return domain.Metadata{}, fmt.Errorf("validate imported profile: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(absoluteStaging, userDataDirName), 0o700); err != nil {
		return domain.Metadata{}, fmt.Errorf("create imported Chromium directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(absoluteStaging, "extensions"), 0o700); err != nil {
		return domain.Metadata{}, fmt.Errorf("create imported extension directory: %w", err)
	}
	if err := s.writeMetadata(filepath.Join(absoluteStaging, metadataFileName), metadata); err != nil {
		return domain.Metadata{}, err
	}
	if err := os.Rename(absoluteStaging, destination); err != nil {
		return domain.Metadata{}, fmt.Errorf("commit imported profile: %w", err)
	}
	return metadata.Clone(), nil
}

func (s *Store) pathsForCanonicalID(id string) (Paths, error) {
	root := filepath.Join(s.root, id)
	relative, err := filepath.Rel(s.root, root)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || relative == ".." {
		return Paths{}, ErrInvalidID
	}
	return Paths{
		Root:           root,
		Metadata:       filepath.Join(root, metadataFileName),
		MetadataBackup: filepath.Join(root, metadataBackupFileName),
		UserData:       filepath.Join(root, userDataDirName),
		Extensions:     filepath.Join(root, "extensions"),
	}, nil
}

func (s *Store) loadUnlocked(canonicalID string) (domain.Metadata, error) {
	paths, err := s.pathsForCanonicalID(canonicalID)
	if err != nil {
		return domain.Metadata{}, err
	}
	metadata, primaryErr := readMetadataFile(paths.Metadata, canonicalID)
	if primaryErr == nil {
		if _, backupErr := readMetadataFile(paths.MetadataBackup, canonicalID); backupErr != nil {
			if err := storage.WriteJSONAtomic(paths.MetadataBackup, metadata, 0o600); err != nil {
				return domain.Metadata{}, fmt.Errorf("repair metadata backup: %w", err)
			}
		}
		return metadata.Clone(), nil
	}
	if !errors.Is(primaryErr, ErrNotFound) && !errors.Is(primaryErr, ErrMetadataCorrupt) {
		return domain.Metadata{}, primaryErr
	}
	metadata, backupErr := readMetadataFile(paths.MetadataBackup, canonicalID)
	if backupErr != nil {
		return domain.Metadata{}, primaryErr
	}
	if err := storage.WriteJSONAtomic(paths.Metadata, metadata, 0o600); err != nil {
		return domain.Metadata{}, fmt.Errorf("restore metadata from backup: %w", err)
	}
	return metadata.Clone(), nil
}

func readMetadataFile(path, canonicalID string) (domain.Metadata, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Metadata{}, ErrNotFound
	}
	if err != nil {
		return domain.Metadata{}, fmt.Errorf("open metadata: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, maxMetadataSize+1))
	decoder.DisallowUnknownFields()
	var metadata domain.Metadata
	if err := decoder.Decode(&metadata); err != nil {
		return domain.Metadata{}, fmt.Errorf("%w: decode metadata: %v", ErrMetadataCorrupt, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return domain.Metadata{}, fmt.Errorf("%w: trailing JSON content", ErrMetadataCorrupt)
	}
	if metadata.ID != canonicalID {
		return domain.Metadata{}, fmt.Errorf("%w: directory id does not match metadata id", ErrMetadataCorrupt)
	}
	if err := metadata.Validate(); err != nil {
		return domain.Metadata{}, fmt.Errorf("%w: %v", ErrMetadataCorrupt, err)
	}
	return metadata.Clone(), nil
}

func (s *Store) writeMetadata(path string, metadata domain.Metadata) error {
	backupPath := filepath.Join(filepath.Dir(path), metadataBackupFileName)
	if err := storage.WriteJSONAtomic(backupPath, metadata, 0o600); err != nil {
		return fmt.Errorf("write metadata backup: %w", err)
	}
	if err := storage.WriteJSONAtomic(path, metadata, 0o600); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

func canonicalProfileID(id string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return "", ErrInvalidID
	}
	return parsed.String(), nil
}

func normalizeMetadata(metadata *domain.Metadata) error {
	metadata.Name = strings.TrimSpace(metadata.Name)
	metadata.Notes = strings.TrimSpace(metadata.Notes)
	metadata.StartURL = strings.TrimSpace(metadata.StartURL)
	metadata.LastURL = strings.TrimSpace(metadata.LastURL)
	metadata.Color = strings.ToLower(strings.TrimSpace(metadata.Color))
	metadata.CreatedAt = metadata.CreatedAt.UTC()
	metadata.UpdatedAt = metadata.UpdatedAt.UTC()

	for index := range metadata.Tags {
		metadata.Tags[index].ID = strings.TrimSpace(metadata.Tags[index].ID)
		metadata.Tags[index].Label = strings.TrimSpace(metadata.Tags[index].Label)
		metadata.Tags[index].Color = strings.ToLower(strings.TrimSpace(metadata.Tags[index].Color))
	}

	normalizedExtensions := make([]string, 0, len(metadata.ExtensionPaths))
	seenExtensions := make(map[string]struct{}, len(metadata.ExtensionPaths))
	for _, extensionPath := range metadata.ExtensionPaths {
		if strings.TrimSpace(extensionPath) == "" {
			return errors.New("extension paths cannot be empty")
		}
		absolutePath, err := filepath.Abs(filepath.Clean(extensionPath))
		if err != nil {
			return fmt.Errorf("normalize extension path: %w", err)
		}
		key := strings.ToLower(absolutePath)
		if _, exists := seenExtensions[key]; exists {
			continue
		}
		seenExtensions[key] = struct{}{}
		normalizedExtensions = append(normalizedExtensions, absolutePath)
	}
	metadata.ExtensionPaths = normalizedExtensions
	return nil
}
