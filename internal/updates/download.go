package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxUpdateBytes int64 = 2 << 30

type DownloadResult struct {
	Version string `json:"version"`
	Path    string `json:"path"`
	Name    string `json:"name"`
	SHA256  string `json:"sha256"`
	Bytes   int64  `json:"bytes"`
	Ready   bool   `json:"ready"`
}

func (s *Service) DownloadLatest(ctx context.Context) (DownloadResult, error) {
	if err := ctx.Err(); err != nil {
		return DownloadResult{}, err
	}
	status, err := s.Check(ctx)
	if err != nil {
		return DownloadResult{}, err
	}
	if !status.UpdateAvailable {
		return DownloadResult{}, errors.New("nenhuma atualização está disponível")
	}
	if !status.InstallAvailable || status.Asset == nil {
		reason := strings.TrimSpace(status.InstallReason)
		if reason == "" {
			reason = "o instalador automático não está disponível para esta release"
		}
		return DownloadResult{}, errors.New(reason)
	}
	root, err := s.updateRoot()
	if err != nil {
		return DownloadResult{}, err
	}
	versionDirectory := filepath.Join(root, "downloads", status.LatestVersion)
	if err := os.MkdirAll(versionDirectory, 0o700); err != nil {
		return DownloadResult{}, fmt.Errorf("criar diretório da atualização: %w", err)
	}
	name := filepath.Base(strings.TrimSpace(status.Asset.Name))
	if name == "." || name == "" || !strings.EqualFold(filepath.Ext(name), ".exe") {
		return DownloadResult{}, errors.New("nome do instalador da atualização inválido")
	}
	destination := filepath.Join(versionDirectory, name)
	if result, err := verifiedDownload(destination, status.LatestVersion, *status.Asset); err == nil {
		return result, nil
	}
	if status.Asset.Size > maxUpdateBytes {
		return DownloadResult{}, errors.New("o instalador excede o limite seguro de 2 GiB")
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return DownloadResult{}, fmt.Errorf("remover atualização inválida do cache: %w", err)
	}
	partial := destination + ".partial"
	_ = os.Remove(partial)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, status.Asset.URL, nil)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("criar download da atualização: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "bruno-browser/"+s.current.Version)
	response, err := s.downloadClient.Do(request)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("baixar atualização: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return DownloadResult{}, fmt.Errorf("download da atualização retornou status %d", response.StatusCode)
	}
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("criar arquivo temporário da atualização: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxUpdateBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written > maxUpdateBytes {
		_ = os.Remove(partial)
		if copyErr != nil {
			return DownloadResult{}, fmt.Errorf("gravar atualização: %w", copyErr)
		}
		if closeErr != nil {
			return DownloadResult{}, fmt.Errorf("fechar atualização: %w", closeErr)
		}
		return DownloadResult{}, errors.New("o instalador excede o limite seguro de 2 GiB")
	}
	actualHash := hex.EncodeToString(hash.Sum(nil))
	if written != status.Asset.Size || !strings.EqualFold(actualHash, status.Asset.SHA256) {
		_ = os.Remove(partial)
		return DownloadResult{}, errors.New("a integridade do instalador não corresponde à release publicada")
	}
	if err := os.Rename(partial, destination); err != nil {
		_ = os.Remove(partial)
		return DownloadResult{}, fmt.Errorf("finalizar download da atualização: %w", err)
	}
	return DownloadResult{
		Version: status.LatestVersion, Path: destination, Name: name,
		SHA256: actualHash, Bytes: written, Ready: true,
	}, nil
}

func (s *Service) updateRoot() (string, error) {
	if strings.TrimSpace(s.dataRoot) == "" {
		return "", errors.New("diretório local de atualizações não configurado")
	}
	root, err := filepath.Abs(filepath.Join(s.dataRoot, "updates"))
	if err != nil {
		return "", fmt.Errorf("normalizar diretório de atualizações: %w", err)
	}
	return root, nil
}

func verifiedDownload(path, version string, asset Asset) (DownloadResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return DownloadResult{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size || info.Size() > maxUpdateBytes {
		return DownloadResult{}, errors.New("arquivo de atualização em cache inválido")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return DownloadResult{}, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(digest, asset.SHA256) {
		return DownloadResult{}, errors.New("hash da atualização em cache inválido")
	}
	return DownloadResult{
		Version: version, Path: path, Name: filepath.Base(path),
		SHA256: digest, Bytes: info.Size(), Ready: true,
	}, nil
}
