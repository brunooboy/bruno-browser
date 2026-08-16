package license

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bruno-browser/internal/storage"

	"github.com/google/uuid"
)

const (
	activationFileName = "license.json"
	historyFileName    = "keys-history.json"
	maxKeyLength       = 16 << 10
)

var (
	ErrNoActivePlan = errors.New("an active plan is required")
	ErrInvalidKey   = errors.New("invalid activation key")
	ErrCorruptState = errors.New("activation state is corrupt")
)

type Plan string

const (
	PlanLifetime Plan = "VITALICIO"
	Plan30Days   Plan = "30"
	Plan7Days    Plan = "7"
	Plan1Day     Plan = "1"
)

type Claims struct {
	DiscordID string `json:"discord_id"`
	Plan      Plan   `json:"plan"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	KeyID     string `json:"key_id"`
}

type Activation struct {
	Activated   bool   `json:"activated"`
	Plan        Plan   `json:"plan,omitempty"`
	ExpiresAt   int64  `json:"expires_at,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	ActivatedAt int64  `json:"activated_at,omitempty"`
	Status      string `json:"status"`
}

type storedActivation struct {
	Activation Activation `json:"activation"`
	Key        string     `json:"key"`
}

type HistoryEntry struct {
	Claims      Claims `json:"claims"`
	Key         string `json:"key"`
	GeneratedAt int64  `json:"generated_at"`
}

type Service struct {
	activationPath string
	historyPath    string
	clock          func() time.Time
	mu             sync.Mutex
}

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(service *Service) {
		if clock != nil {
			service.clock = clock
		}
	}
}

func New(dataRoot string, options ...Option) (*Service, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return nil, errors.New("data root is required")
	}
	service := &Service{
		activationPath: filepath.Join(dataRoot, activationFileName),
		historyPath:    filepath.Join(dataRoot, historyFileName),
		clock:          time.Now,
	}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *Service) Generate(ctx context.Context, discordID string, plan Plan) (HistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return HistoryEntry{}, err
	}
	discordID = strings.TrimSpace(discordID)
	if discordID != "" && !isDiscordID(discordID) {
		return HistoryEntry{}, errors.New("Discord ID must contain 17 to 20 digits")
	}
	now := s.clock().UTC()
	claims := Claims{
		DiscordID: discordID,
		Plan:      plan,
		CreatedAt: now.Unix(),
		ExpiresAt: expirationFor(plan, now),
		KeyID:     strings.Split(uuid.NewString(), "-")[0],
	}
	if !claims.Plan.Valid() {
		return HistoryEntry{}, errors.New("unsupported plan")
	}
	key, err := encryptClaims(claims)
	if err != nil {
		return HistoryEntry{}, err
	}
	entry := HistoryEntry{Claims: claims, Key: key, GeneratedAt: now.Unix()}
	s.mu.Lock()
	defer s.mu.Unlock()
	history, err := s.readHistory()
	if err != nil {
		return HistoryEntry{}, err
	}
	history = append([]HistoryEntry{entry}, history...)
	if len(history) > 500 {
		history = history[:500]
	}
	if err := storage.WriteJSONAtomic(s.historyPath, history, 0o600); err != nil {
		return HistoryEntry{}, fmt.Errorf("save key history: %w", err)
	}
	return entry, nil
}

func (s *Service) Inspect(ctx context.Context, key string) (Claims, error) {
	if err := ctx.Err(); err != nil {
		return Claims{}, err
	}
	return decryptClaims(key)
}

func (s *Service) Activate(ctx context.Context, key, loggedDiscordID string) (Activation, error) {
	if err := ctx.Err(); err != nil {
		return Activation{}, err
	}
	loggedDiscordID = strings.TrimSpace(loggedDiscordID)
	if !isDiscordID(loggedDiscordID) {
		return Activation{}, errors.New("Discord login is required before activation")
	}
	claims, err := decryptClaims(key)
	if err != nil {
		return Activation{}, err
	}
	if claims.DiscordID != "" && claims.DiscordID != loggedDiscordID {
		return Activation{}, errors.New("this key belongs to another Discord account")
	}
	now := s.clock().UTC().Unix()
	if claims.ExpiresAt != 0 && now >= claims.ExpiresAt {
		return Activation{}, errors.New("this key has expired")
	}
	activation := Activation{
		Activated: true, Plan: claims.Plan, ExpiresAt: claims.ExpiresAt,
		KeyID: claims.KeyID, ActivatedAt: now, Status: "active",
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := storage.WriteJSONAtomic(s.activationPath, storedActivation{Activation: activation, Key: strings.TrimSpace(key)}, 0o600); err != nil {
		return Activation{}, fmt.Errorf("save activation: %w", err)
	}
	return activation, nil
}

func (s *Service) Status(ctx context.Context, loggedDiscordID string) (Activation, error) {
	if err := ctx.Err(); err != nil {
		return Activation{}, err
	}
	loggedDiscordID = strings.TrimSpace(loggedDiscordID)
	if loggedDiscordID == "" {
		return Activation{Status: "none"}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, err := s.readActivation()
	if errors.Is(err, os.ErrNotExist) {
		return Activation{Status: "none"}, nil
	}
	if errors.Is(err, ErrCorruptState) {
		_ = os.Remove(s.activationPath)
		return Activation{Status: "none"}, nil
	}
	if err != nil {
		return Activation{}, err
	}
	claims, err := decryptClaims(stored.Key)
	if err != nil || claims.KeyID != stored.Activation.KeyID || claims.Plan != stored.Activation.Plan {
		_ = os.Remove(s.activationPath)
		return Activation{Status: "none"}, nil
	}
	if claims.DiscordID != "" && claims.DiscordID != loggedDiscordID {
		return Activation{Status: "none"}, nil
	}
	if claims.ExpiresAt != 0 && s.clock().UTC().Unix() >= claims.ExpiresAt {
		_ = os.Remove(s.activationPath)
		return Activation{Status: "expired", Plan: claims.Plan, ExpiresAt: claims.ExpiresAt, KeyID: claims.KeyID, ActivatedAt: stored.Activation.ActivatedAt}, nil
	}
	stored.Activation.Activated = true
	stored.Activation.Status = "active"
	return stored.Activation, nil
}

func (s *Service) RequireActive(ctx context.Context, loggedDiscordID string) error {
	status, err := s.Status(ctx, loggedDiscordID)
	if err != nil {
		return err
	}
	if !status.Activated || status.Status != "active" {
		return ErrNoActivePlan
	}
	return nil
}

func (s *Service) Deactivate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.activationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove activation: %w", err)
	}
	return nil
}

func (s *Service) History(ctx context.Context) ([]HistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readHistory()
}

func (s *Service) readActivation() (storedActivation, error) {
	file, err := os.Open(s.activationPath)
	if err != nil {
		return storedActivation{}, err
	}
	defer file.Close()
	var stored storedActivation
	decoder := json.NewDecoder(io.LimitReader(file, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return storedActivation{}, fmt.Errorf("%w: decode activation: %v", ErrCorruptState, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return storedActivation{}, fmt.Errorf("%w: trailing JSON content", ErrCorruptState)
	}
	if strings.TrimSpace(stored.Key) == "" || stored.Activation.KeyID == "" || !stored.Activation.Plan.Valid() {
		return storedActivation{}, fmt.Errorf("%w: activation fields are incomplete", ErrCorruptState)
	}
	return stored, nil
}

func (s *Service) readHistory() ([]HistoryEntry, error) {
	file, err := os.Open(s.historyPath)
	if errors.Is(err, os.ErrNotExist) {
		return []HistoryEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open key history: %w", err)
	}
	defer file.Close()
	var history []HistoryEntry
	decoder := json.NewDecoder(io.LimitReader(file, 8<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil {
		return nil, fmt.Errorf("decode key history: %w", err)
	}
	return history, nil
}

func encryptClaims(claims Claims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode key claims: %w", err)
	}
	block, err := aes.NewCipher(derivedKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("create key nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, payload, []byte("bruno-browser-key-v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptClaims(encoded string) (Claims, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" || len(encoded) > maxKeyLength {
		return Claims{}, ErrInvalidKey
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Claims{}, ErrInvalidKey
	}
	block, _ := aes.NewCipher(derivedKey())
	gcm, _ := cipher.NewGCM(block)
	if len(payload) <= gcm.NonceSize() {
		return Claims{}, ErrInvalidKey
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte("bruno-browser-key-v1"))
	if err != nil {
		return Claims{}, ErrInvalidKey
	}
	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(plain)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil || !claims.Plan.Valid() || claims.CreatedAt <= 0 || claims.KeyID == "" {
		return Claims{}, ErrInvalidKey
	}
	if claims.DiscordID != "" && !isDiscordID(claims.DiscordID) {
		return Claims{}, ErrInvalidKey
	}
	return claims, nil
}

func derivedKey() []byte {
	// Kept exclusively in the Go binary; it is never bundled into frontend assets.
	secret := "Bruno" + "1204" + "$"
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

func (p Plan) Valid() bool {
	return p == PlanLifetime || p == Plan30Days || p == Plan7Days || p == Plan1Day
}

func expirationFor(plan Plan, now time.Time) int64 {
	switch plan {
	case Plan1Day:
		return now.AddDate(0, 0, 1).Unix()
	case Plan7Days:
		return now.AddDate(0, 0, 7).Unix()
	case Plan30Days:
		return now.AddDate(0, 0, 30).Unix()
	default:
		return 0
	}
}

func isDiscordID(value string) bool {
	if len(value) < 17 || len(value) > 20 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
