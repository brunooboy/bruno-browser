//go:build windows

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	helperSwitch     = "--bruno-update-helper"
	healthFilePrefix = "--bruno-update-health-file="
)

func (s *Service) LaunchApplyHelper(ctx context.Context, download DownloadResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !download.Ready || download.Path == "" || download.SHA256 == "" || download.Bytes <= 0 {
		return errors.New("o download da atualização ainda não está pronto")
	}
	updateRoot, err := s.updateRoot()
	if err != nil {
		return err
	}
	installer, err := filepath.Abs(download.Path)
	if err != nil || !pathWithin(updateRoot, installer) {
		return errors.New("o instalador saiu do diretório local de atualizações")
	}
	if err := verifyInstaller(installer, download.Bytes, download.SHA256); err != nil {
		return err
	}
	application, err := os.Executable()
	if err != nil {
		return fmt.Errorf("localizar Bruno Browser em execução: %w", err)
	}
	application, err = filepath.Abs(application)
	if err != nil {
		return err
	}
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	expectedDirectory := filepath.Join(localAppData, "Programs", "Bruno Browser")
	if localAppData == "" || !strings.EqualFold(filepath.Clean(filepath.Dir(application)), filepath.Clean(expectedDirectory)) {
		return errors.New("a atualização automática exige a versão instalada; na versão portátil, use o instalador da release")
	}
	helperDirectory := filepath.Join(updateRoot, "helpers")
	if err := os.MkdirAll(helperDirectory, 0o700); err != nil {
		return fmt.Errorf("criar diretório do atualizador: %w", err)
	}
	helperPath := filepath.Join(helperDirectory, fmt.Sprintf("bruno-update-%d-%d.exe", os.Getpid(), time.Now().UnixNano()))
	if err := copyRegularFile(application, helperPath); err != nil {
		return fmt.Errorf("preparar atualizador isolado: %w", err)
	}
	command := exec.CommandContext(context.WithoutCancel(ctx), helperPath,
		helperSwitch,
		"--installer="+installer,
		"--sha256="+strings.ToLower(download.SHA256),
		"--bytes="+strconv.FormatInt(download.Bytes, 10),
		"--parent-pid="+strconv.Itoa(os.Getpid()),
		"--application="+application,
		"--updates-root="+updateRoot,
		"--version="+download.Version,
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		_ = os.Remove(helperPath)
		return fmt.Errorf("iniciar atualizador isolado: %w", err)
	}
	_ = command.Process.Release()
	return nil
}

// RunApplyHelper handles the isolated updater mode before the Wails runtime is
// initialized. The copied helper can replace the installed executable without
// keeping the original Bruno Browser binary locked.
func RunApplyHelper(args []string) (bool, error) {
	if !containsArgument(args, helperSwitch) {
		return false, nil
	}
	installer := argumentValue(args, "--installer=")
	expectedHash := argumentValue(args, "--sha256=")
	bytesText := argumentValue(args, "--bytes=")
	parentText := argumentValue(args, "--parent-pid=")
	application := argumentValue(args, "--application=")
	updateRoot := argumentValue(args, "--updates-root=")
	version := argumentValue(args, "--version=")
	expectedBytes, bytesErr := strconv.ParseInt(bytesText, 10, 64)
	parentPID, parentErr := strconv.ParseUint(parentText, 10, 32)
	if installer == "" || application == "" || updateRoot == "" || bytesErr != nil || parentErr != nil {
		return true, errors.New("argumentos do atualizador estão incompletos")
	}
	updateRoot, _ = filepath.Abs(updateRoot)
	installer, _ = filepath.Abs(installer)
	application, _ = filepath.Abs(application)
	if !pathWithin(updateRoot, installer) {
		return true, errors.New("instalador fora do diretório permitido")
	}
	if err := validateInstalledApplication(application); err != nil {
		return true, err
	}
	if err := verifyInstaller(installer, expectedBytes, expectedHash); err != nil {
		return true, err
	}
	if err := waitForProcessExit(uint32(parentPID), 2*time.Minute); err != nil {
		return true, err
	}

	installDirectory := filepath.Dir(application)
	if pathWithin(installDirectory, updateRoot) {
		return true, errors.New("o cache de atualização não pode ficar dentro da instalação")
	}
	backupDirectory := filepath.Join(updateRoot, "rollback", sanitizeVersion(version)+"-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	healthDirectory := filepath.Join(updateRoot, "health")
	if err := os.MkdirAll(healthDirectory, 0o700); err != nil {
		return true, fmt.Errorf("preparar verificação da nova versão: %w", err)
	}
	if err := copyDirectory(installDirectory, backupDirectory); err != nil {
		return true, fmt.Errorf("criar ponto de restauração da instalação: %w", err)
	}
	installCommand := exec.Command(installer, "/S")
	installCommand.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := installCommand.Run(); err != nil {
		restoreErr := restoreInstallation(backupDirectory, installDirectory)
		_, _ = startApplication(application, "")
		return true, errors.Join(fmt.Errorf("instalador retornou falha: %w", err), restoreErr)
	}

	healthMarker := filepath.Join(healthDirectory, fmt.Sprintf("%d-%d.ok", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(healthMarker)
	newProcess, err := startApplication(application, healthMarker)
	if err == nil {
		err = waitForHealthyApplication(newProcess, healthMarker, 60*time.Second)
	}
	if err != nil {
		if newProcess != nil && newProcess.Process != nil {
			_ = newProcess.Process.Kill()
			_ = waitForProcessExit(uint32(newProcess.Process.Pid), 10*time.Second)
		}
		restoreErr := restoreInstallation(backupDirectory, installDirectory)
		_, _ = startApplication(application, "")
		return true, errors.Join(fmt.Errorf("a nova versão não iniciou corretamente: %w", err), restoreErr)
	}
	_ = os.Remove(healthMarker)
	_ = os.RemoveAll(backupDirectory)
	return true, nil
}

func UpdateHealthMarker(args []string) string {
	return argumentValue(args, healthFilePrefix)
}

func MarkHealthy(marker, dataRoot string) error {
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return nil
	}
	allowedRoot, err := filepath.Abs(filepath.Join(dataRoot, "updates", "health"))
	if err != nil {
		return err
	}
	marker, err = filepath.Abs(marker)
	if err != nil || !pathWithin(allowedRoot, marker) {
		return errors.New("marcador de atualização fora do diretório permitido")
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write([]byte("healthy\n"))
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func waitForProcessExit(pid uint32, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return fmt.Errorf("acompanhar encerramento do aplicativo: %w", err)
	}
	defer windows.CloseHandle(handle)
	result, err := windows.WaitForSingleObject(handle, uint32(timeout/time.Millisecond))
	if err != nil {
		return err
	}
	if result == uint32(windows.WAIT_TIMEOUT) {
		return errors.New("o aplicativo não encerrou a tempo para atualizar")
	}
	return nil
}

func startApplication(application, healthMarker string) (*exec.Cmd, error) {
	arguments := []string{}
	if healthMarker != "" {
		arguments = append(arguments, healthFilePrefix+healthMarker)
	}
	command := exec.Command(application, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return command, nil
}

func waitForHealthyApplication(command *exec.Cmd, marker string, timeout time.Duration) error {
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	deadline := time.NewTimer(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case err := <-exited:
			if err == nil {
				return errors.New("a nova versão encerrou antes de confirmar a inicialização")
			}
			return err
		case <-ticker.C:
			if info, err := os.Stat(marker); err == nil && info.Mode().IsRegular() {
				return nil
			}
		case <-deadline.C:
			return errors.New("tempo esgotado aguardando a interface da nova versão")
		}
	}
}

func verifyInstaller(path string, expectedBytes int64, expectedHash string) error {
	expectedHash, err := normalizedSHA256(expectedHash)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("abrir instalador verificado: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != expectedBytes || expectedBytes <= 0 || expectedBytes > maxUpdateBytes {
		return errors.New("tamanho do instalador não corresponde à release")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expectedHash) {
		return errors.New("SHA-256 do instalador não corresponde à release")
	}
	return nil
}

func validateInstalledApplication(application string) error {
	if !strings.EqualFold(filepath.Base(application), "Bruno Browser.exe") {
		return errors.New("executável instalado do Bruno Browser não reconhecido")
	}
	installDirectory := filepath.Dir(application)
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	expectedDirectory := filepath.Join(localAppData, "Programs", "Bruno Browser")
	if localAppData == "" || !strings.EqualFold(filepath.Clean(installDirectory), filepath.Clean(expectedDirectory)) {
		return errors.New("diretório de instalação do Bruno Browser não reconhecido")
	}
	if _, err := os.Stat(filepath.Join(installDirectory, "engine", "chrome-win", "chrome.exe")); err != nil {
		return errors.New("Bruno Engine ausente na instalação atual")
	}
	return nil
}

func restoreInstallation(backupDirectory, installDirectory string) error {
	if err := validateInstalledApplication(filepath.Join(installDirectory, "Bruno Browser.exe")); err != nil {
		// The failed installer may have removed the engine. The destination is
		// still constrained to the exact per-user install directory below.
		localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		expected := filepath.Join(localAppData, "Programs", "Bruno Browser")
		if localAppData == "" || !strings.EqualFold(filepath.Clean(installDirectory), filepath.Clean(expected)) {
			return err
		}
	}
	if err := os.RemoveAll(installDirectory); err != nil {
		return err
	}
	return copyDirectory(backupDirectory, installDirectory)
}

func copyDirectory(source, destination string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	if pathWithin(source, destination) {
		return errors.New("destino de cópia não pode ficar dentro da origem")
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("a instalação contém um link simbólico inesperado")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return errors.New("item da instalação saiu da origem permitida")
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		return copyRegularFile(path, target)
	})
}

func copyRegularFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("arquivo de origem inválido")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}

func pathWithin(root, target string) bool {
	root, rootErr := filepath.Abs(filepath.Clean(root))
	target, targetErr := filepath.Abs(filepath.Clean(target))
	if rootErr != nil || targetErr != nil {
		return false
	}
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func containsArgument(args []string, target string) bool {
	for _, argument := range args {
		if argument == target {
			return true
		}
	}
	return false
}

func argumentValue(args []string, prefix string) string {
	for _, argument := range args {
		if strings.HasPrefix(argument, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(argument, prefix))
		}
	}
	return ""
}

func sanitizeVersion(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	var result strings.Builder
	for _, character := range version {
		if (character >= '0' && character <= '9') || character == '.' || character == '-' {
			result.WriteRune(character)
		}
	}
	if result.Len() == 0 {
		return "unknown"
	}
	return result.String()
}
