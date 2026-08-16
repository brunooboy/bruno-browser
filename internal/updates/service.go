package updates

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed version.json
var currentManifestPayload []byte

type Change struct {
	Version     string `json:"version"`
	Date        string `json:"date"`
	Description string `json:"description"`
}

type Manifest struct {
	Version     string    `json:"version"`
	PublishedAt time.Time `json:"publishedAt"`
	Changelog   []Change  `json:"changelog"`
}

type Status struct {
	CurrentVersion   string    `json:"currentVersion"`
	LatestVersion    string    `json:"latestVersion"`
	UpdateAvailable  bool      `json:"updateAvailable"`
	InstallAvailable bool      `json:"installAvailable"`
	InstallReason    string    `json:"installReason,omitempty"`
	Asset            *Asset    `json:"asset,omitempty"`
	CheckedAt        time.Time `json:"checkedAt"`
	Source           string    `json:"source"`
	Changelog        []Change  `json:"changelog"`
}

type Service struct {
	current        Manifest
	endpoint       string
	dataRoot       string
	client         *http.Client
	downloadClient *http.Client
}

type Option func(*Service)

func WithDataRoot(dataRoot string) Option {
	return func(service *Service) {
		service.dataRoot = strings.TrimSpace(dataRoot)
	}
}

func WithHTTPClients(checkClient, downloadClient *http.Client) Option {
	return func(service *Service) {
		if checkClient != nil {
			service.client = checkClient
		}
		if downloadClient != nil {
			service.downloadClient = downloadClient
		}
	}
}

func New(endpoint string, options ...Option) (*Service, error) {
	var current Manifest
	if err := json.Unmarshal(currentManifestPayload, &current); err != nil {
		return nil, fmt.Errorf("decode embedded version manifest: %w", err)
	}
	if err := validateManifest(current); err != nil {
		return nil, err
	}
	service := &Service{
		current:        current,
		endpoint:       strings.TrimSpace(endpoint),
		client:         &http.Client{Timeout: 12 * time.Second},
		downloadClient: secureDownloadClient(),
	}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *Service) Current(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	return Status{
		CurrentVersion: s.current.Version,
		LatestVersion:  s.current.Version,
		InstallReason:  "nenhuma atualização disponível",
		CheckedAt:      time.Now().UTC(),
		Source:         "local",
		Changelog:      append([]Change(nil), s.current.Changelog...),
	}, nil
}

func (s *Service) Check(ctx context.Context) (Status, error) {
	if s.endpoint == "" {
		return s.Current(ctx)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return Status{}, fmt.Errorf("create update request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "bruno-browser/"+s.current.Version)
	response, err := s.client.Do(request)
	if err != nil {
		return Status{}, fmt.Errorf("check for updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("update endpoint returned status %d", response.StatusCode)
	}
	var remote Manifest
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&remote); err != nil {
		return Status{}, fmt.Errorf("decode update manifest: %w", err)
	}
	if err := validateManifest(remote); err != nil {
		return Status{}, err
	}
	status := Status{
		CurrentVersion:  s.current.Version,
		LatestVersion:   remote.Version,
		UpdateAvailable: compareVersions(remote.Version, s.current.Version) > 0,
		CheckedAt:       time.Now().UTC(),
		Source:          s.endpoint,
		Changelog:       append([]Change(nil), remote.Changelog...),
	}
	if !status.UpdateAvailable {
		status.InstallReason = "nenhuma atualização disponível"
		return status, nil
	}
	asset, assetErr := s.resolveInstallerAsset(ctx, remote.Version)
	if assetErr != nil {
		status.InstallReason = assetErr.Error()
		return status, nil
	}
	status.Asset = &asset
	status.InstallAvailable = s.dataRoot != ""
	if !status.InstallAvailable {
		status.InstallReason = "diretório local de atualizações não configurado"
	}
	return status, nil
}

func validateManifest(manifest Manifest) error {
	if strings.TrimSpace(manifest.Version) == "" {
		return errors.New("version manifest is missing a version")
	}
	if _, err := versionParts(manifest.Version); err != nil {
		return err
	}
	if len(manifest.Changelog) > 200 {
		return errors.New("version manifest changelog is too large")
	}
	for _, change := range manifest.Changelog {
		if strings.TrimSpace(change.Version) == "" || strings.TrimSpace(change.Description) == "" {
			return errors.New("version manifest contains an incomplete changelog entry")
		}
	}
	return nil
}

func compareVersions(left, right string) int {
	a, _ := versionParts(left)
	b, _ := versionParts(right)
	for index := 0; index < 3; index++ {
		if a[index] > b[index] {
			return 1
		}
		if a[index] < b[index] {
			return -1
		}
	}
	return 0
}

func versionParts(value string) ([3]int, error) {
	var result [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value = strings.SplitN(value, "-", 2)[0]
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return result, fmt.Errorf("invalid semantic version %q", value)
	}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return result, fmt.Errorf("invalid semantic version %q", value)
		}
		result[index] = number
	}
	return result, nil
}
