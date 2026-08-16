package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Draft   bool                 `json:"draft"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
	State              string `json:"state"`
}

func (s *Service) resolveInstallerAsset(ctx context.Context, version string) (Asset, error) {
	apiURL, err := githubReleaseAPI(s.endpoint, version)
	if err != nil {
		return Asset{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return Asset{}, fmt.Errorf("criar consulta da release: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "bruno-browser/"+s.current.Version)
	response, err := s.client.Do(request)
	if err != nil {
		return Asset{}, fmt.Errorf("consultar pacote da atualização: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Asset{}, fmt.Errorf("release da atualização retornou status %d", response.StatusCode)
	}
	var release githubRelease
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(&release); err != nil {
		return Asset{}, fmt.Errorf("decodificar release da atualização: %w", err)
	}
	if release.Draft {
		return Asset{}, errors.New("a release encontrada ainda é um rascunho")
	}
	return selectInstallerAsset(release.Assets)
}

func githubReleaseAPI(endpoint, version string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", errors.New("URL do manifesto de atualização inválida")
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	var owner, repository string
	switch strings.ToLower(parsed.Hostname()) {
	case "raw.githubusercontent.com":
		if len(segments) >= 3 {
			owner, repository = segments[0], segments[1]
		}
	case "github.com", "www.github.com":
		if len(segments) >= 2 {
			owner, repository = segments[0], segments[1]
		}
	}
	if owner == "" || repository == "" {
		return "", errors.New("o endpoint atual não permite localizar o instalador da release")
	}
	tag := strings.TrimSpace(version)
	if !strings.HasPrefix(strings.ToLower(tag), "v") {
		tag = "v" + tag
	}
	return "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/releases/tags/" + url.PathEscape(tag), nil
}

func selectInstallerAsset(assets []githubReleaseAsset) (Asset, error) {
	candidates := make([]githubReleaseAsset, 0)
	for _, candidate := range assets {
		lowerName := strings.ToLower(strings.TrimSpace(candidate.Name))
		if candidate.State != "uploaded" || candidate.Size <= 0 ||
			!strings.HasSuffix(lowerName, ".exe") || !strings.Contains(lowerName, "bruno") || !strings.Contains(lowerName, "setup") {
			continue
		}
		if _, err := normalizedSHA256(candidate.Digest); err != nil {
			continue
		}
		if err := validateAssetURL(candidate.BrowserDownloadURL); err != nil {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return Asset{}, errors.New("a release não possui um instalador Bruno Browser com SHA-256 verificável")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := strings.ToLower(candidates[i].Name)
		right := strings.ToLower(candidates[j].Name)
		leftSpecific := strings.Contains(left, "amd64") || strings.Contains(left, "arm64")
		rightSpecific := strings.Contains(right, "amd64") || strings.Contains(right, "arm64")
		if leftSpecific != rightSpecific {
			return !leftSpecific
		}
		return left < right
	})
	selected := candidates[0]
	digest, _ := normalizedSHA256(selected.Digest)
	return Asset{
		Name: strings.TrimSpace(selected.Name), URL: selected.BrowserDownloadURL,
		SHA256: digest, Size: selected.Size,
	}, nil
}

func validateAssetURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return errors.New("URL do instalador inválida")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "github.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return errors.New("o instalador não está hospedado em um domínio oficial do GitHub")
	}
	return nil
}

func secureDownloadClient() *http.Client {
	return &http.Client{
		Timeout: 45 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return errors.New("muitos redirecionamentos no download da atualização")
			}
			return validateAssetURL(request.URL.String())
		},
	}
}

func normalizedSHA256(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) != 64 {
		return "", errors.New("SHA-256 do instalador inválido")
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", errors.New("SHA-256 do instalador inválido")
		}
	}
	return value, nil
}
