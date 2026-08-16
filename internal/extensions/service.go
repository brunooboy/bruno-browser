package extensions

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	"bruno-browser/internal/profile"
	"bruno-browser/internal/storage"
)

const (
	metadataFileName     = "metadata.json"
	bundledStateFileName = "bundled-state.json"
	maxCRXSize           = 256 << 20
	maxExpandedSize      = 768 << 20
	maxEntries           = 20_000
)

type Extension struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Version            string    `json:"version"`
	Description        string    `json:"description,omitempty"`
	ManifestVersion    int       `json:"manifestVersion"`
	InstalledAt        time.Time `json:"installedAt"`
	Path               string    `json:"path"`
	AssignedProfileIDs []string  `json:"assignedProfileIds"`
	Bundled            bool      `json:"bundled"`
}

type bundledState struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Processed     map[string]time.Time `json:"processed"`
}

type manifest struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Description     string `json:"description"`
	ManifestVersion int    `json:"manifest_version"`
}

type Service struct {
	root     string
	profiles *profile.Store
	clock    func() time.Time
	mu       sync.Mutex
}

func New(dataRoot string, profiles *profile.Store) (*Service, error) {
	if strings.TrimSpace(dataRoot) == "" || profiles == nil {
		return nil, errors.New("data root and profile store are required")
	}
	root, err := filepath.Abs(filepath.Join(dataRoot, "extensions"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create extension library: %w", err)
	}
	return &Service{root: root, profiles: profiles, clock: time.Now}, nil
}

func (s *Service) InstallCRX(ctx context.Context, sourcePath string) (Extension, error) {
	if err := ctx.Err(); err != nil {
		return Extension{}, err
	}
	if !strings.EqualFold(filepath.Ext(sourcePath), ".crx") {
		return Extension{}, errors.New("select a .crx extension package")
	}
	payload, err := readLimitedFile(sourcePath, maxCRXSize)
	if err != nil {
		return Extension{}, err
	}
	zipPayload, err := crxZipPayload(payload)
	if err != nil {
		return Extension{}, err
	}
	hash := sha256.Sum256(payload)
	id := hex.EncodeToString(hash[:16])

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, err := s.load(id); err == nil {
		existing.AssignedProfileIDs, _ = s.assignedProfiles(ctx, existing.Path)
		return existing, nil
	}
	temporaryRoot, err := os.MkdirTemp(s.root, ".install-*")
	if err != nil {
		return Extension{}, fmt.Errorf("create extension staging directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(temporaryRoot)
		}
	}()
	unpacked := filepath.Join(temporaryRoot, "unpacked")
	if err := os.Mkdir(unpacked, 0o700); err != nil {
		return Extension{}, err
	}
	if err := extractZip(ctx, zipPayload, unpacked); err != nil {
		return Extension{}, err
	}
	manifestPayload, err := os.ReadFile(filepath.Join(unpacked, "manifest.json"))
	if err != nil {
		return Extension{}, errors.New("CRX does not contain manifest.json at its root")
	}
	var details manifest
	if err := json.Unmarshal(manifestPayload, &details); err != nil {
		return Extension{}, fmt.Errorf("decode extension manifest: %w", err)
	}
	details.Name = strings.TrimSpace(details.Name)
	details.Version = strings.TrimSpace(details.Version)
	if details.Name == "" || details.Version == "" || (details.ManifestVersion != 2 && details.ManifestVersion != 3) {
		return Extension{}, errors.New("extension manifest is missing a valid name, version or manifest_version")
	}
	if err := storage.WriteFileAtomic(filepath.Join(temporaryRoot, "original.crx"), payload, 0o600); err != nil {
		return Extension{}, err
	}
	extension := Extension{
		ID: id, Name: details.Name, Version: details.Version,
		Description: strings.TrimSpace(details.Description), ManifestVersion: details.ManifestVersion,
		InstalledAt: s.clock().UTC(), Path: filepath.Join(s.root, id, "unpacked"), AssignedProfileIDs: []string{},
	}
	if err := storage.WriteJSONAtomic(filepath.Join(temporaryRoot, metadataFileName), extension, 0o600); err != nil {
		return Extension{}, err
	}
	destination := filepath.Join(s.root, id)
	if err := os.Rename(temporaryRoot, destination); err != nil {
		return Extension{}, fmt.Errorf("commit extension installation: %w", err)
	}
	committed = true
	return extension, nil
}

// EnsureBundled imports a packaged CRX exactly once. If the user later
// uninstalls it, the processed marker remains and it is not silently restored.
func (s *Service) EnsureBundled(ctx context.Context, sourcePath, expectedSHA256 string) (Extension, bool, error) {
	if err := ctx.Err(); err != nil {
		return Extension{}, false, err
	}
	payload, err := readLimitedFile(sourcePath, maxCRXSize)
	if err != nil {
		return Extension{}, false, err
	}
	hash := sha256.Sum256(payload)
	hashText := hex.EncodeToString(hash[:])
	if !strings.EqualFold(strings.TrimSpace(expectedSHA256), hashText) {
		return Extension{}, false, errors.New("bundled extension failed SHA-256 verification")
	}

	s.mu.Lock()
	state, err := s.loadBundledState()
	if err != nil {
		s.mu.Unlock()
		return Extension{}, false, err
	}
	_, processed := state.Processed[hashText]
	s.mu.Unlock()
	if processed {
		return Extension{}, false, nil
	}

	extension, err := s.InstallCRX(ctx, sourcePath)
	if err != nil {
		return Extension{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	extension.Bundled = true
	if err := storage.WriteJSONAtomic(filepath.Join(s.root, extension.ID, metadataFileName), extension, 0o600); err != nil {
		return Extension{}, false, err
	}
	state, err = s.loadBundledState()
	if err != nil {
		return Extension{}, false, err
	}
	state.Processed[hashText] = s.clock().UTC()
	if err := storage.WriteJSONAtomic(filepath.Join(s.root, bundledStateFileName), state, 0o600); err != nil {
		return Extension{}, false, err
	}
	return extension, true, nil
}

func (s *Service) loadBundledState() (bundledState, error) {
	payload, err := readLimitedFile(filepath.Join(s.root, bundledStateFileName), 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return bundledState{SchemaVersion: 1, Processed: make(map[string]time.Time)}, nil
	}
	if err != nil {
		return bundledState{}, err
	}
	var state bundledState
	if err := json.Unmarshal(payload, &state); err != nil {
		return bundledState{}, fmt.Errorf("decode bundled extension state: %w", err)
	}
	if state.SchemaVersion != 1 {
		return bundledState{}, fmt.Errorf("unsupported bundled extension state schema %d", state.SchemaVersion)
	}
	if state.Processed == nil {
		state.Processed = make(map[string]time.Time)
	}
	return state, nil
}

func (s *Service) List(ctx context.Context) ([]Extension, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := make([]Extension, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !validID(entry.Name()) {
			continue
		}
		extension, err := s.load(entry.Name())
		if err != nil {
			return nil, err
		}
		extension.AssignedProfileIDs, err = s.assignedProfiles(ctx, extension.Path)
		if err != nil {
			return nil, err
		}
		result = append(result, extension)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].InstalledAt.After(result[j].InstalledAt) })
	return result, nil
}

// OriginalCRX returns the verified package kept in the local extension vault.
// Backup code uses this package instead of copying an unpacked directory so
// restoration always goes through the regular CRX validation pipeline.
func (s *Service) OriginalCRX(ctx context.Context, extensionID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	extension, err := s.load(extensionID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.root, extension.ID, "original.crx")
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("extension CRX is not a regular vault file")
	}
	return path, nil
}

func (s *Service) SetAssignments(ctx context.Context, extensionID string, profileIDs []string) (Extension, error) {
	if err := ctx.Err(); err != nil {
		return Extension{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	extension, err := s.load(extensionID)
	if err != nil {
		return Extension{}, err
	}
	targets := make(map[string]struct{}, len(profileIDs))
	for _, profileID := range profileIDs {
		metadata, err := s.profiles.Get(ctx, profileID)
		if err != nil {
			return Extension{}, err
		}
		targets[metadata.ID] = struct{}{}
	}
	profiles, err := s.profiles.List(ctx)
	if err != nil {
		return Extension{}, err
	}
	for _, metadata := range profiles {
		paths := removePath(metadata.ExtensionPaths, extension.Path)
		if _, selected := targets[metadata.ID]; selected {
			paths = append(paths, extension.Path)
		}
		if slices.Equal(paths, metadata.ExtensionPaths) {
			continue
		}
		if _, err := s.profiles.Update(ctx, metadata.ID, fieldsFromMetadata(metadata, paths)); err != nil {
			return Extension{}, fmt.Errorf("update extension assignment for profile %s: %w", metadata.Name, err)
		}
	}
	extension.AssignedProfileIDs, err = s.assignedProfiles(ctx, extension.Path)
	return extension, err
}

func (s *Service) Remove(ctx context.Context, extensionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	extension, err := s.load(extensionID)
	if err != nil {
		return err
	}
	target := filepath.Join(s.root, extensionID)
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("invalid extension directory")
	}

	profileList, err := s.profiles.List(ctx)
	if err != nil {
		return err
	}
	changed := make([]domain.Metadata, 0)
	for _, metadata := range profileList {
		paths := removePath(metadata.ExtensionPaths, extension.Path)
		if slices.Equal(paths, metadata.ExtensionPaths) {
			continue
		}
		if _, err := s.profiles.Update(ctx, metadata.ID, fieldsFromMetadata(metadata, paths)); err != nil {
			rollbackErr := s.restoreAssignments(context.WithoutCancel(ctx), changed)
			return errors.Join(fmt.Errorf("detach extension from profile %s: %w", metadata.Name, err), rollbackErr)
		}
		changed = append(changed, metadata)
	}
	if err := os.RemoveAll(target); err != nil {
		rollbackErr := s.restoreAssignments(context.WithoutCancel(ctx), changed)
		return errors.Join(fmt.Errorf("remove extension files: %w", err), rollbackErr)
	}
	return nil
}

func (s *Service) restoreAssignments(ctx context.Context, changed []domain.Metadata) error {
	rollbackErrors := make([]error, 0)
	for index := len(changed) - 1; index >= 0; index-- {
		metadata := changed[index]
		if _, err := s.profiles.Update(ctx, metadata.ID, fieldsFromMetadata(metadata, metadata.ExtensionPaths)); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore extension assignment for profile %s: %w", metadata.Name, err))
		}
	}
	return errors.Join(rollbackErrors...)
}

func (s *Service) load(id string) (Extension, error) {
	if !validID(id) {
		return Extension{}, errors.New("invalid extension id")
	}
	path := filepath.Join(s.root, id, metadataFileName)
	payload, err := readLimitedFile(path, 1<<20)
	if err != nil {
		return Extension{}, err
	}
	var extension Extension
	if err := json.Unmarshal(payload, &extension); err != nil {
		return Extension{}, fmt.Errorf("decode extension metadata: %w", err)
	}
	if extension.ID != id {
		return Extension{}, errors.New("extension metadata id mismatch")
	}
	extension.Path = filepath.Join(s.root, id, "unpacked")
	return extension, nil
}

func (s *Service) assignedProfiles(ctx context.Context, extensionPath string) ([]string, error) {
	profiles, err := s.profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	assigned := make([]string, 0)
	for _, metadata := range profiles {
		for _, path := range metadata.ExtensionPaths {
			if strings.EqualFold(filepath.Clean(path), filepath.Clean(extensionPath)) {
				assigned = append(assigned, metadata.ID)
				break
			}
		}
	}
	sort.Strings(assigned)
	return assigned, nil
}

func fieldsFromMetadata(metadata domain.Metadata, extensionPaths []string) profile.Fields {
	return profile.Fields{
		Name: metadata.Name, Color: metadata.Color, Platforms: metadata.Platforms,
		Status: metadata.Status, Tags: metadata.Tags, Notes: metadata.Notes,
		StartURL: metadata.StartURL, ExtensionPaths: extensionPaths,
	}
}

func removePath(paths []string, target string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if !strings.EqualFold(filepath.Clean(path), filepath.Clean(target)) {
			result = append(result, path)
		}
	}
	return result
}

func crxZipPayload(payload []byte) ([]byte, error) {
	if len(payload) < 12 || string(payload[:4]) != "Cr24" {
		return nil, errors.New("file is not a valid CRX package")
	}
	version := binary.LittleEndian.Uint32(payload[4:8])
	offset := 0
	switch version {
	case 2:
		if len(payload) < 16 {
			return nil, errors.New("truncated CRX2 header")
		}
		keyLength := uint64(binary.LittleEndian.Uint32(payload[8:12]))
		signatureLength := uint64(binary.LittleEndian.Uint32(payload[12:16]))
		headerEnd := uint64(16) + keyLength + signatureLength
		if headerEnd > uint64(len(payload)) {
			return nil, errors.New("invalid CRX2 header length")
		}
		offset = int(headerEnd)
	case 3:
		headerLength := uint64(binary.LittleEndian.Uint32(payload[8:12]))
		headerEnd := uint64(12) + headerLength
		if headerEnd > uint64(len(payload)) {
			return nil, errors.New("invalid CRX3 header length")
		}
		offset = int(headerEnd)
	default:
		return nil, fmt.Errorf("unsupported CRX version %d", version)
	}
	if len(payload)-offset < 4 || !bytes.Equal(payload[offset:offset+4], []byte{'P', 'K', 3, 4}) {
		return nil, errors.New("CRX ZIP payload is missing")
	}
	return payload[offset:], nil
}

func extractZip(ctx context.Context, payload []byte, destination string) error {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return fmt.Errorf("open CRX ZIP payload: %w", err)
	}
	if len(reader.File) > maxEntries {
		return errors.New("extension contains too many files")
	}
	var total uint64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("extension cannot contain symbolic links")
		}
		total += entry.UncompressedSize64
		if total > maxExpandedSize {
			return errors.New("expanded extension exceeds the safe size limit")
		}
		cleanName := filepath.Clean(filepath.FromSlash(entry.Name))
		if cleanName == "." || filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) {
			return errors.New("extension contains an unsafe path")
		}
		target := filepath.Join(destination, cleanName)
		relative, err := filepath.Rel(destination, target)
		if err != nil || strings.HasPrefix(relative, "..") {
			return errors.New("extension path escapes destination")
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		destinationFile, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(destinationFile, io.LimitReader(source, int64(entry.UncompressedSize64)+1))
		closeErr := destinationFile.Close()
		_ = source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("file exceeds the safe size limit")
	}
	return io.ReadAll(io.LimitReader(file, limit+1))
}

func validID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
