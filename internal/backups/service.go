package backups

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"bruno-browser/internal/browser"
	"bruno-browser/internal/domain"
	"bruno-browser/internal/extensions"
	"bruno-browser/internal/fingerprint"
	"bruno-browser/internal/network"
	"bruno-browser/internal/profile"
	"bruno-browser/internal/storage"

	"github.com/google/uuid"
)

const (
	manifestEntry    = "manifest.json"
	maxArchiveFiles  = 250_000
	maxExpandedBytes = int64(128) << 30
	maxManifestBytes = 4 << 20
	maxHistoryBytes  = 4 << 20
)

type Service struct {
	profiles   *profile.Store
	browser    *browser.Manager
	network    *network.Manager
	extensions *extensions.Service
	root       string
	tempRoot   string
	history    string
	clock      func() time.Time
	mu         sync.Mutex
}

func New(dataRoot string, profiles *profile.Store, browserManager *browser.Manager, networkManager *network.Manager, extensionService *extensions.Service) (*Service, error) {
	if strings.TrimSpace(dataRoot) == "" || profiles == nil || browserManager == nil || networkManager == nil || extensionService == nil {
		return nil, errors.New("backup service dependencies are required")
	}
	root, err := filepath.Abs(filepath.Join(dataRoot, "backups"))
	if err != nil {
		return nil, err
	}
	tempRoot := filepath.Join(root, "staging")
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}
	return &Service{
		profiles: profiles, browser: browserManager, network: networkManager, extensions: extensionService,
		root: root, tempRoot: tempRoot, history: filepath.Join(root, "history.json"), clock: time.Now,
	}, nil
}

func (s *Service) History(ctx context.Context) ([]HistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readHistory()
}

func (s *Service) Export(ctx context.Context, profileIDs []string, destination, password string) (result ExportResult, operationErr error) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return ExportResult{}, errors.New("backup destination is required")
	}
	if !strings.EqualFold(filepath.Ext(destination), ".bruno-profile") {
		destination += ".bruno-profile"
	}
	if err := validatePassword(password); err != nil {
		return ExportResult{}, err
	}
	metadataList, releases, err := s.reserveProfiles(ctx, profileIDs)
	if err != nil {
		return ExportResult{}, err
	}
	defer func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}()
	names := make([]string, len(metadataList))
	for index, metadata := range metadataList {
		names[index] = metadata.Name
	}
	defer func() {
		entry := s.newHistory("export", destination, names, operationErr)
		if operationErr == nil {
			if info, statErr := os.Stat(destination); statErr == nil {
				entry.Bytes = info.Size()
			}
		}
		_ = s.recordHistory(entry)
		if operationErr == nil {
			result.History = entry
		}
	}()

	zipFile, err := os.CreateTemp(s.tempRoot, ".export-*.zip")
	if err != nil {
		return ExportResult{}, err
	}
	zipPath := zipFile.Name()
	_ = zipFile.Close()
	defer os.Remove(zipPath)
	manifest, err := s.writeArchive(ctx, zipPath, metadataList)
	if err != nil {
		return ExportResult{}, err
	}
	bytesWritten, err := encryptFile(zipPath, destination, password)
	if err != nil {
		return ExportResult{}, fmt.Errorf("encrypt backup: %w", err)
	}
	return ExportResult{Path: destination, Bytes: bytesWritten, Profiles: len(manifest.Profiles)}, nil
}

func (s *Service) Import(ctx context.Context, source, password string) (result ImportResult, operationErr error) {
	source = strings.TrimSpace(source)
	if source == "" || !strings.EqualFold(filepath.Ext(source), ".bruno-profile") {
		return ImportResult{}, errors.New("select a .bruno-profile backup")
	}
	if err := validatePassword(password); err != nil {
		return ImportResult{}, err
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		return ImportResult{}, errors.New("backup file is unavailable")
	}
	var historyNames []string
	defer func() {
		entry := s.newHistory("import", source, historyNames, operationErr)
		entry.Bytes = info.Size()
		_ = s.recordHistory(entry)
		if operationErr == nil {
			result.History = entry
		}
	}()
	workingRoot, err := os.MkdirTemp(s.tempRoot, ".import-*")
	if err != nil {
		return ImportResult{}, err
	}
	defer os.RemoveAll(workingRoot)
	zipPath := filepath.Join(workingRoot, "payload.zip")
	if err := decryptFile(source, zipPath, password); err != nil {
		return ImportResult{}, err
	}
	manifest, stages, extensionFiles, err := s.extractArchive(ctx, zipPath)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() {
		for _, stage := range stages {
			_ = os.RemoveAll(stage)
		}
		for _, path := range extensionFiles {
			_ = os.RemoveAll(filepath.Dir(path))
			break
		}
	}()
	historyNames = make([]string, len(manifest.Profiles))
	for index, item := range manifest.Profiles {
		historyNames[index] = item.Metadata.Name
	}
	extensionPaths := make(map[string]string, len(manifest.Extensions))
	for _, item := range manifest.Extensions {
		installed, err := s.extensions.InstallCRX(ctx, extensionFiles[item.ID])
		if err != nil {
			return ImportResult{}, fmt.Errorf("restore extension %s: %w", item.Name, err)
		}
		if installed.ID != item.ID {
			return ImportResult{}, fmt.Errorf("%w: extension package id mismatch", ErrInvalidBackup)
		}
		extensionPaths[item.ID] = installed.Path
	}
	committed := make([]domain.Metadata, 0, len(manifest.Profiles))
	rollback := func() {
		for index := len(committed) - 1; index >= 0; index-- {
			_ = s.profiles.Delete(context.WithoutCancel(ctx), committed[index].ID)
		}
	}
	for _, item := range manifest.Profiles {
		paths := make([]string, 0, len(item.ExtensionIDs))
		for _, extensionID := range item.ExtensionIDs {
			path, exists := extensionPaths[extensionID]
			if !exists {
				rollback()
				return ImportResult{}, fmt.Errorf("%w: missing extension %s", ErrInvalidBackup, extensionID)
			}
			paths = append(paths, path)
		}
		imported, err := s.profiles.CommitImport(ctx, item.Metadata, stages[item.Metadata.ID], paths)
		if err != nil {
			rollback()
			return ImportResult{}, err
		}
		committed = append(committed, imported)
		importedPaths, _ := s.profiles.Paths(imported.ID)
		fingerprintPath := filepath.Join(importedPaths.Root, "fingerprint.json")
		if _, err := os.Stat(fingerprintPath); err == nil {
			if err := fingerprint.RebindFile(fingerprintPath, imported.ID); err != nil {
				rollback()
				return ImportResult{}, fmt.Errorf("restore fingerprint: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			rollback()
			return ImportResult{}, err
		}
		networkInput := network.SaveInput{
			Mode: item.Network.Mode, DNSPreset: item.Network.DNSPreset, Host: item.Network.Host,
			Port: item.Network.Port, Username: item.Network.Username, Password: item.Network.Password,
			ClearPassword: item.Network.Password == "", BypassList: slices.Clone(item.Network.BypassList),
		}
		if _, err := s.network.Save(ctx, imported.ID, networkInput); err != nil {
			rollback()
			return ImportResult{}, fmt.Errorf("restore network settings: %w", err)
		}
		result.Profiles = append(result.Profiles, ImportedProfile{
			SourceID: item.Metadata.ID, ID: imported.ID, Name: imported.Name, Rekeyed: imported.ID != item.Metadata.ID,
		})
	}
	return resultWithImportDetails(result, source, info.Size()), nil
}

func resultWithImportDetails(result ImportResult, source string, bytes int64) ImportResult {
	result.Path = source
	result.Bytes = bytes
	return result
}

func (s *Service) reserveProfiles(ctx context.Context, profileIDs []string) ([]domain.Metadata, []func(), error) {
	if len(profileIDs) == 0 {
		return nil, nil, errors.New("select at least one profile")
	}
	unique := make(map[string]domain.Metadata, len(profileIDs))
	for _, id := range profileIDs {
		metadata, err := s.profiles.Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		unique[metadata.ID] = metadata
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	releases := make([]func(), 0, len(ids))
	for _, id := range ids {
		release, ok := s.browser.BeginMaintenance(id)
		if !ok {
			for index := len(releases) - 1; index >= 0; index-- {
				releases[index]()
			}
			return nil, nil, ErrProfileRunning
		}
		releases = append(releases, release)
	}
	metadataList := make([]domain.Metadata, 0, len(ids))
	for _, id := range ids {
		metadataList = append(metadataList, unique[id])
	}
	return metadataList, releases, nil
}

func (s *Service) writeArchive(ctx context.Context, destination string, metadataList []domain.Metadata) (archiveManifest, error) {
	extensionList, err := s.extensions.List(ctx)
	if err != nil {
		return archiveManifest{}, err
	}
	extensionByPath := make(map[string]extensions.Extension, len(extensionList))
	for _, extension := range extensionList {
		extensionByPath[strings.ToLower(filepath.Clean(extension.Path))] = extension
	}
	manifest := archiveManifest{SchemaVersion: CurrentSchemaVersion, AppVersion: AppVersion, ExportedAt: s.clock().UTC()}
	requiredExtensions := make(map[string]extensions.Extension)
	for _, metadata := range metadataList {
		runtimeSettings, err := s.network.RuntimeSettingsForBackup(ctx, metadata.ID)
		if err != nil {
			return archiveManifest{}, fmt.Errorf("read proxy settings for %s: %w", metadata.Name, err)
		}
		item := archiveProfile{Metadata: metadata.Clone(), Network: archiveNetwork{
			Mode: runtimeSettings.Mode, DNSPreset: runtimeSettings.DNSPreset, Host: runtimeSettings.Host,
			Port: runtimeSettings.Port, Username: runtimeSettings.Username, Password: runtimeSettings.Password,
			BypassList: slices.Clone(runtimeSettings.BypassList),
		}}
		item.Metadata.ExtensionPaths = nil
		for _, path := range metadata.ExtensionPaths {
			extension, exists := extensionByPath[strings.ToLower(filepath.Clean(path))]
			if !exists {
				return archiveManifest{}, fmt.Errorf("profile %s references an extension outside the managed vault", metadata.Name)
			}
			item.ExtensionIDs = append(item.ExtensionIDs, extension.ID)
			requiredExtensions[extension.ID] = extension
		}
		sort.Strings(item.ExtensionIDs)
		manifest.Profiles = append(manifest.Profiles, item)
	}
	extensionIDs := make([]string, 0, len(requiredExtensions))
	for id := range requiredExtensions {
		extensionIDs = append(extensionIDs, id)
	}
	sort.Strings(extensionIDs)
	for _, id := range extensionIDs {
		extension := requiredExtensions[id]
		manifest.Extensions = append(manifest.Extensions, archiveExtension{
			ID: extension.ID, Name: extension.Name, Version: extension.Version, Entry: "extensions/" + extension.ID + ".crx",
		})
	}
	file, err := os.Create(destination)
	if err != nil {
		return archiveManifest{}, err
	}
	writer := zip.NewWriter(file)
	closed := false
	defer func() {
		if !closed {
			_ = writer.Close()
			_ = file.Close()
		}
	}()
	for _, metadata := range metadataList {
		paths, _ := s.profiles.Paths(metadata.ID)
		prefix := "profiles/" + metadata.ID + "/chromium"
		if err := addDirectoryTree(ctx, writer, paths.UserData, prefix); err != nil {
			return archiveManifest{}, fmt.Errorf("archive profile %s: %w", metadata.Name, err)
		}
		fingerprintPath := filepath.Join(paths.Root, "fingerprint.json")
		if _, err := os.Stat(fingerprintPath); err == nil {
			if err := addDiskFile(ctx, writer, fingerprintPath, "profiles/"+metadata.ID+"/fingerprint.json"); err != nil {
				return archiveManifest{}, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return archiveManifest{}, err
		}
	}
	for _, item := range manifest.Extensions {
		path, err := s.extensions.OriginalCRX(ctx, item.ID)
		if err != nil {
			return archiveManifest{}, err
		}
		if err := addDiskFile(ctx, writer, path, item.Entry); err != nil {
			return archiveManifest{}, err
		}
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return archiveManifest{}, err
	}
	manifestPayload = append(manifestPayload, '\n')
	if err := addBytes(writer, manifestEntry, manifestPayload); err != nil {
		return archiveManifest{}, err
	}
	if err := writer.Close(); err != nil {
		return archiveManifest{}, err
	}
	if err := file.Sync(); err != nil {
		return archiveManifest{}, err
	}
	if err := file.Close(); err != nil {
		return archiveManifest{}, err
	}
	closed = true
	return manifest, nil
}

func (s *Service) extractArchive(ctx context.Context, source string) (archiveManifest, map[string]string, map[string]string, error) {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return archiveManifest{}, nil, nil, ErrInvalidBackup
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > maxArchiveFiles {
		return archiveManifest{}, nil, nil, ErrInvalidBackup
	}
	entries := make(map[string]*zip.File, len(reader.File))
	var expanded uint64
	for _, entry := range reader.File {
		name, err := validateEntryName(entry.Name)
		if err != nil || entry.Mode()&os.ModeSymlink != 0 {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
		if _, duplicate := entries[name]; duplicate {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
		entries[name] = entry
		expanded += entry.UncompressedSize64
		if expanded > uint64(maxExpandedBytes) {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
	}
	manifestFile := entries[manifestEntry]
	if manifestFile == nil || manifestFile.UncompressedSize64 > maxManifestBytes {
		return archiveManifest{}, nil, nil, ErrInvalidBackup
	}
	manifestPayload, err := readZipEntry(manifestFile, maxManifestBytes)
	if err != nil {
		return archiveManifest{}, nil, nil, ErrInvalidBackup
	}
	var manifest archiveManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestPayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.SchemaVersion != CurrentSchemaVersion || len(manifest.Profiles) == 0 {
		return archiveManifest{}, nil, nil, ErrInvalidBackup
	}
	profileMap := make(map[string]archiveProfile, len(manifest.Profiles))
	for _, item := range manifest.Profiles {
		parsed, err := uuid.Parse(item.Metadata.ID)
		if err != nil || parsed.String() != item.Metadata.ID || item.Metadata.Validate() != nil {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
		if _, duplicate := profileMap[item.Metadata.ID]; duplicate {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
		profileMap[item.Metadata.ID] = item
	}
	extensionMap := make(map[string]archiveExtension, len(manifest.Extensions))
	for _, item := range manifest.Extensions {
		if len(item.ID) != 32 || item.Entry != "extensions/"+item.ID+".crx" {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
		if _, duplicate := extensionMap[item.ID]; duplicate {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
		extensionMap[item.ID] = item
		if entries[item.Entry] == nil {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
	}
	for _, item := range manifest.Profiles {
		for _, extensionID := range item.ExtensionIDs {
			if _, exists := extensionMap[extensionID]; !exists {
				return archiveManifest{}, nil, nil, ErrInvalidBackup
			}
		}
	}
	stages := make(map[string]string, len(manifest.Profiles))
	for id := range profileMap {
		stage, err := os.MkdirTemp(s.profiles.Root(), ".restore-*")
		if err != nil {
			return archiveManifest{}, nil, nil, err
		}
		stages[id] = stage
	}
	cleanupStages := true
	defer func() {
		if cleanupStages {
			for _, stage := range stages {
				_ = os.RemoveAll(stage)
			}
		}
	}()
	extensionRoot, err := os.MkdirTemp(s.tempRoot, ".extensions-*")
	if err != nil {
		return archiveManifest{}, nil, nil, err
	}
	defer os.RemoveAll(extensionRoot)
	extensionFiles := make(map[string]string, len(extensionMap))
	for name, entry := range entries {
		if err := ctx.Err(); err != nil {
			return archiveManifest{}, nil, nil, err
		}
		if name == manifestEntry || strings.HasSuffix(name, "/") {
			continue
		}
		var destination string
		if strings.HasPrefix(name, "extensions/") {
			id := strings.TrimSuffix(strings.TrimPrefix(name, "extensions/"), ".crx")
			if _, exists := extensionMap[id]; !exists || name != extensionMap[id].Entry {
				return archiveManifest{}, nil, nil, ErrInvalidBackup
			}
			destination = filepath.Join(extensionRoot, id+".crx")
			extensionFiles[id] = destination
		} else if strings.HasPrefix(name, "profiles/") {
			parts := strings.Split(name, "/")
			if len(parts) < 3 {
				return archiveManifest{}, nil, nil, ErrInvalidBackup
			}
			stage, exists := stages[parts[1]]
			if !exists || (parts[2] != "chromium" && !(parts[2] == "fingerprint.json" && len(parts) == 3)) {
				return archiveManifest{}, nil, nil, ErrInvalidBackup
			}
			destination = filepath.Join(stage, filepath.FromSlash(strings.Join(parts[2:], "/")))
		} else {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
		if err := extractEntry(ctx, entry, destination); err != nil {
			return archiveManifest{}, nil, nil, err
		}
	}
	for id := range extensionMap {
		if extensionFiles[id] == "" {
			return archiveManifest{}, nil, nil, ErrInvalidBackup
		}
	}
	// Extension packages are copied to a persistent operation temp directory;
	// the extraction root can then be cleaned without invalidating paths.
	persistentExtensions, err := os.MkdirTemp(s.tempRoot, ".restore-crx-*")
	if err != nil {
		return archiveManifest{}, nil, nil, err
	}
	for id, path := range extensionFiles {
		target := filepath.Join(persistentExtensions, id+".crx")
		if err := copyRegularFile(ctx, path, target); err != nil {
			_ = os.RemoveAll(persistentExtensions)
			return archiveManifest{}, nil, nil, err
		}
		extensionFiles[id] = target
	}
	cleanupStages = false
	return manifest, stages, extensionFiles, nil
}

func addDirectoryTree(ctx context.Context, writer *zip.Writer, root, prefix string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are not allowed in profile backups")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if shouldSkipChromiumFile(filepath.ToSlash(relative)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := prefix + "/" + filepath.ToSlash(relative)
		if entry.IsDir() {
			_, err = writer.Create(name + "/")
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("profile contains an unsupported special file")
		}
		return addDiskFile(ctx, writer, path, name)
	})
}

func shouldSkipChromiumFile(relative string) bool {
	base := filepath.Base(filepath.FromSlash(relative))
	return base == "DevToolsActivePort" || strings.HasPrefix(base, "Singleton") || base == "lockfile"
}

func addDiskFile(ctx context.Context, writer *zip.Writer, source, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("backup source is not a regular file")
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = name
	header.Method = zip.Deflate
	output, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, &contextReader{ctx: ctx, reader: input})
	return err
}

func addBytes(writer *zip.Writer, name string, payload []byte) error {
	output, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = output.Write(payload)
	return err
}

func validateEntryName(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.ContainsRune(name, '\x00') {
		return "", ErrInvalidBackup
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != strings.TrimSuffix(name, "/") {
		if !(strings.HasSuffix(name, "/") && clean+"/" == name) {
			return "", ErrInvalidBackup
		}
	}
	if strings.Contains(clean, ":") {
		return "", ErrInvalidBackup
	}
	if strings.HasSuffix(name, "/") {
		return clean + "/", nil
	}
	return clean, nil
}

func readZipEntry(entry *zip.File, maximum uint64) ([]byte, error) {
	if entry.UncompressedSize64 > maximum {
		return nil, ErrInvalidBackup
	}
	input, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer input.Close()
	return io.ReadAll(io.LimitReader(input, int64(maximum)+1))
}

func extractEntry(ctx context.Context, entry *zip.File, destination string) error {
	if entry.UncompressedSize64 > uint64(maxExpandedBytes) {
		return ErrInvalidBackup
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(&contextReader{ctx: ctx, reader: input}, int64(entry.UncompressedSize64)+1))
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != int64(entry.UncompressedSize64) {
		return ErrInvalidBackup
	}
	return nil
}

func copyRegularFile(ctx context.Context, source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, &contextReader{ctx: ctx, reader: input})
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(payload []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(payload)
}

func (s *Service) newHistory(operation, path string, names []string, operationErr error) HistoryEntry {
	entry := HistoryEntry{
		ID: uuid.NewString(), Operation: operation, Status: "success", ArchivePath: path,
		ProfileCount: len(names), ProfileNames: slices.Clone(names), CreatedAt: s.clock().UTC(),
	}
	if operationErr != nil {
		entry.Status = "failed"
		entry.Message = operationErr.Error()
	}
	return entry
}

func (s *Service) recordHistory(entry HistoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	history, err := s.readHistory()
	if err != nil {
		return err
	}
	history = append([]HistoryEntry{entry}, history...)
	if len(history) > 100 {
		history = history[:100]
	}
	return storage.WriteJSONAtomic(s.history, history, 0o600)
}

func (s *Service) readHistory() ([]HistoryEntry, error) {
	payload, err := os.ReadFile(s.history)
	if errors.Is(err, os.ErrNotExist) {
		return []HistoryEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(payload) > maxHistoryBytes {
		return nil, errors.New("backup history is too large")
	}
	var history []HistoryEntry
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil {
		return nil, fmt.Errorf("decode backup history: %w", err)
	}
	return history, nil
}
