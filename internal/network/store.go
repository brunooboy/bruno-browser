package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"
	"bruno-browser/internal/storage"
)

const (
	networkFileName = "network.json"
	maxNetworkSize  = 1 << 20
)

type ProfileRepository interface {
	Get(context.Context, string) (domain.Metadata, error)
	Paths(string) (profile.Paths, error)
}

type networkRecord struct {
	SchemaVersion     int       `json:"schemaVersion"`
	ProfileID         string    `json:"profileId"`
	Mode              Mode      `json:"mode"`
	Host              string    `json:"host,omitempty"`
	Port              uint16    `json:"port,omitempty"`
	Username          string    `json:"username,omitempty"`
	ProtectedPassword string    `json:"protectedPassword,omitempty"`
	BypassList        []string  `json:"bypassList,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type Store struct {
	profiles  ProfileRepository
	protector SecretProtector
	clock     func() time.Time
	mu        sync.RWMutex
}

type StoreOption func(*Store)

func WithClock(clock func() time.Time) StoreOption {
	return func(store *Store) {
		if clock != nil {
			store.clock = clock
		}
	}
}

func NewStore(profiles ProfileRepository, protector SecretProtector, options ...StoreOption) (*Store, error) {
	if profiles == nil {
		return nil, errors.New("profile repository is required")
	}
	if protector == nil {
		return nil, errors.New("secret protector is required")
	}
	store := &Store{profiles: profiles, protector: protector, clock: time.Now}
	for _, option := range options {
		option(store)
	}
	return store, nil
}

func (s *Store) Get(ctx context.Context, profileID string) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	metadata, err := s.profiles.Get(ctx, profileID)
	if err != nil {
		return Settings{}, err
	}
	paths, err := s.profiles.Paths(metadata.ID)
	if err != nil {
		return Settings{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	record, err := s.loadRecord(filepath.Join(paths.Root, networkFileName), metadata.ID)
	if err != nil {
		return Settings{}, err
	}
	return record.settings(), nil
}

func (s *Store) Save(ctx context.Context, profileID string, input SaveInput) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	normalized, err := normalizeInput(input)
	if err != nil {
		return Settings{}, err
	}
	metadata, err := s.profiles.Get(ctx, profileID)
	if err != nil {
		return Settings{}, err
	}
	paths, err := s.profiles.Paths(metadata.ID)
	if err != nil {
		return Settings{}, err
	}
	path := filepath.Join(paths.Root, networkFileName)

	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.loadRecord(path, metadata.ID)
	if err != nil {
		return Settings{}, err
	}
	record := networkRecord{
		SchemaVersion: CurrentSchemaVersion,
		ProfileID:     metadata.ID,
		Mode:          normalized.Mode,
		Host:          normalized.Host,
		Port:          normalized.Port,
		Username:      normalized.Username,
		BypassList:    slices.Clone(normalized.BypassList),
		UpdatedAt:     s.clock().UTC(),
	}
	if normalized.Mode != ModeDirect && !normalized.ClearPassword {
		record.ProtectedPassword = existing.ProtectedPassword
	}
	if normalized.Password != "" {
		record.ProtectedPassword, err = s.protector.Protect([]byte(normalized.Password))
		if err != nil {
			return Settings{}, fmt.Errorf("protect proxy password: %w", err)
		}
	}
	if record.ProtectedPassword != "" && record.Username == "" {
		return Settings{}, errors.New("proxy username is required while a password is stored")
	}
	if err := storage.WriteJSONAtomic(path, record, 0o600); err != nil {
		return Settings{}, fmt.Errorf("write network settings: %w", err)
	}
	return record.settings(), nil
}

func (s *Store) Resolve(ctx context.Context, profileID string) (RuntimeSettings, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeSettings{}, err
	}
	metadata, err := s.profiles.Get(ctx, profileID)
	if err != nil {
		return RuntimeSettings{}, err
	}
	paths, err := s.profiles.Paths(metadata.ID)
	if err != nil {
		return RuntimeSettings{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	record, err := s.loadRecord(filepath.Join(paths.Root, networkFileName), metadata.ID)
	if err != nil {
		return RuntimeSettings{}, err
	}
	runtimeSettings := RuntimeSettings{Settings: record.settings()}
	if record.ProtectedPassword != "" {
		plaintext, err := s.protector.Unprotect(record.ProtectedPassword)
		if err != nil {
			return RuntimeSettings{}, fmt.Errorf("unprotect proxy password: %w", err)
		}
		runtimeSettings.Password = string(plaintext)
		for index := range plaintext {
			plaintext[index] = 0
		}
	}
	return runtimeSettings, nil
}

func (s *Store) loadRecord(path, profileID string) (networkRecord, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return networkRecord{
			SchemaVersion: CurrentSchemaVersion,
			ProfileID:     profileID,
			Mode:          ModeDirect,
		}, nil
	}
	if err != nil {
		return networkRecord{}, fmt.Errorf("open network settings: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, maxNetworkSize+1))
	decoder.DisallowUnknownFields()
	var record networkRecord
	if err := decoder.Decode(&record); err != nil {
		return networkRecord{}, fmt.Errorf("decode network settings: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return networkRecord{}, errors.New("network settings contain trailing JSON content")
	}
	if record.SchemaVersion != CurrentSchemaVersion {
		return networkRecord{}, fmt.Errorf("unsupported network schema version %d", record.SchemaVersion)
	}
	if record.ProfileID != profileID {
		return networkRecord{}, errors.New("network settings profile id does not match its directory")
	}
	input, err := normalizeInput(SaveInput{
		Mode:       record.Mode,
		Host:       record.Host,
		Port:       record.Port,
		Username:   record.Username,
		BypassList: record.BypassList,
	})
	if err != nil {
		return networkRecord{}, fmt.Errorf("validate network settings: %w", err)
	}
	record.Mode = input.Mode
	record.Host = input.Host
	record.Port = input.Port
	record.Username = input.Username
	record.BypassList = input.BypassList
	return record, nil
}

func (record networkRecord) settings() Settings {
	return Settings{
		SchemaVersion: record.SchemaVersion,
		ProfileID:     record.ProfileID,
		Mode:          record.Mode,
		Host:          record.Host,
		Port:          record.Port,
		Username:      record.Username,
		HasPassword:   record.ProtectedPassword != "",
		BypassList:    slices.Clone(record.BypassList),
		UpdatedAt:     record.UpdatedAt,
	}
}
