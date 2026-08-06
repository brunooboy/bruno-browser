package preferences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"bruno-browser/internal/storage"
)

const fileName = "preferences.json"

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Preferences struct {
	AccentColor string `json:"accentColor"`
}

type Service struct {
	path string
	mu   sync.Mutex
}

func New(dataRoot string) (*Service, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return nil, errors.New("data root is required")
	}
	return &Service{path: filepath.Join(dataRoot, fileName)}, nil
}

func (s *Service) Get(ctx context.Context) (Preferences, error) {
	if err := ctx.Err(); err != nil {
		return Preferences{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Service) Save(ctx context.Context, value Preferences) (Preferences, error) {
	if err := ctx.Err(); err != nil {
		return Preferences{}, err
	}
	value.AccentColor = strings.ToLower(strings.TrimSpace(value.AccentColor))
	if !colorPattern.MatchString(value.AccentColor) {
		return Preferences{}, errors.New("accent color must use #RRGGBB format")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := storage.WriteJSONAtomic(s.path, value, 0o600); err != nil {
		return Preferences{}, fmt.Errorf("save preferences: %w", err)
	}
	return value, nil
}

func (s *Service) read() (Preferences, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Preferences{AccentColor: "#42ff91"}, nil
	}
	if err != nil {
		return Preferences{}, fmt.Errorf("open preferences: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	var value Preferences
	if err := decoder.Decode(&value); err != nil {
		return Preferences{}, fmt.Errorf("decode preferences: %w", err)
	}
	if !colorPattern.MatchString(value.AccentColor) {
		return Preferences{}, errors.New("stored accent color is invalid")
	}
	value.AccentColor = strings.ToLower(value.AccentColor)
	return value, nil
}
