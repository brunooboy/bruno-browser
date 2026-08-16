package browser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var ErrBrowserNotFound = errors.New("Bruno Engine executable was not found")

func FindExecutable(explicitPath string) (string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		path, err := validateExecutablePath(explicitPath)
		if err != nil {
			return "", fmt.Errorf("configured browser executable: %w", err)
		}
		return path, nil
	}
	if runtime.GOOS == "windows" {
		if path, ok := findBundledBrunoEngine(); ok {
			return path, nil
		}
		// Windows releases are intentionally self-contained. Falling back to an
		// arbitrary Chrome, Edge or Donut installation would make fingerprints,
		// extensions and command-line behavior differ between computers.
		return "", ErrBrowserNotFound
	}

	for _, name := range executableNames(runtime.GOOS) {
		path, err := exec.LookPath(name)
		if err == nil {
			return filepath.Abs(path)
		}
	}
	for _, candidate := range executableCandidates(runtime.GOOS) {
		path, err := validateExecutablePath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", ErrBrowserNotFound
}

func findBundledBrunoEngine() (string, bool) {
	roots := make([]string, 0, 2)
	if executable, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(executable))
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		roots = append(roots, workingDirectory)
	}
	return findBundledBrunoEngineInRoots(roots)
}

func findBundledBrunoEngineInRoots(roots []string) (string, bool) {
	for _, root := range roots {
		for _, relativePath := range []string{
			filepath.Join("engine", "chrome-win", "chrome.exe"),
			filepath.Join("build", "bin", "engine", "chrome-win", "chrome.exe"),
			filepath.Join("bruno-engine", "chrome-win", "chrome.exe"),
			filepath.Join("chrome-win", "chrome.exe"),
		} {
			candidate := filepath.Join(root, relativePath)
			if path, validationErr := validateBrunoEnginePath(candidate); validationErr == nil {
				return path, true
			}
		}
	}
	return "", false
}

func validateBrunoEnginePath(path string) (string, error) {
	executable, err := validateExecutablePath(path)
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(executable)
	for _, required := range []string{
		"chrome.dll",
		"resources.pak",
		filepath.Join("locales", "en-US.pak"),
	} {
		info, statErr := os.Stat(filepath.Join(directory, required))
		if statErr != nil {
			return "", fmt.Errorf("Bruno Engine is incomplete: %s: %w", required, statErr)
		}
		if info.IsDir() {
			return "", fmt.Errorf("Bruno Engine is incomplete: %s is a directory", required)
		}
	}
	return executable, nil
}

func validateExecutablePath(path string) (string, error) {
	absolutePath, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("path points to a directory")
	}
	return absolutePath, nil
}

func executableNames(goos string) []string {
	switch goos {
	case "windows":
		return []string{"chromium.exe", "chrome.exe", "brave.exe", "msedge.exe"}
	case "darwin":
		return []string{"chromium", "google-chrome", "brave-browser"}
	default:
		return []string{"chromium", "chromium-browser", "google-chrome-stable", "google-chrome", "brave-browser"}
	}
}

func executableCandidates(goos string) []string {
	var candidates []string
	switch goos {
	case "windows":
		appendWindows := func(environmentName string, suffixes ...string) {
			base := strings.TrimSpace(os.Getenv(environmentName))
			if base == "" {
				return
			}
			for _, suffix := range suffixes {
				candidates = append(candidates, filepath.Join(base, suffix))
			}
		}
		appendWindows("LOCALAPPDATA",
			`Google\Chrome\Application\chrome.exe`,
			`Chromium\Application\chrome.exe`,
			`BraveSoftware\Brave-Browser\Application\brave.exe`,
			`Microsoft\Edge\Application\msedge.exe`,
		)
		for _, environmentName := range []string{"PROGRAMFILES", "PROGRAMW6432", "PROGRAMFILES(X86)"} {
			appendWindows(environmentName,
				`Google\Chrome\Application\chrome.exe`,
				`Chromium\Application\chrome.exe`,
				`BraveSoftware\Brave-Browser\Application\brave.exe`,
				`Microsoft\Edge\Application\msedge.exe`,
			)
		}
	case "darwin":
		candidates = append(candidates,
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		)
	default:
		candidates = append(candidates,
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/brave-browser",
		)
	}
	return candidates
}
