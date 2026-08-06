package browser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var ErrBrowserNotFound = errors.New("Chromium executable not found")

func FindExecutable(explicitPath string) (string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		path, err := validateExecutablePath(explicitPath)
		if err != nil {
			return "", fmt.Errorf("configured browser executable: %w", err)
		}
		return path, nil
	}
	if runtime.GOOS == "windows" {
		if path, ok := findDownloadedDonutWayfern(); ok {
			return path, nil
		}
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

// findDownloadedDonutWayfern reuses an already-downloaded free Wayfern engine.
// Bruno never writes to Donut's directory and falls back to the installed
// Chromium family when no compatible binary is present.
func findDownloadedDonutWayfern() (string, bool) {
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if localAppData == "" {
		return "", false
	}
	versionsRoot := filepath.Join(localAppData, "DonutBrowser", "binaries", "wayfern")
	versions, err := os.ReadDir(versionsRoot)
	if err != nil {
		return "", false
	}
	sort.Slice(versions, func(left, right int) bool {
		return versions[left].Name() > versions[right].Name()
	})
	for _, version := range versions {
		if !version.IsDir() {
			continue
		}
		versionRoot := filepath.Join(versionsRoot, version.Name())
		for _, directory := range []string{"", "bin", "wayfern", "wayfern-win", "chrome-win"} {
			for _, name := range []string{"wayfern.exe", "chromium.exe", "chrome.exe"} {
				candidate := filepath.Join(versionRoot, directory, name)
				if path, validationErr := validateExecutablePath(candidate); validationErr == nil {
					return path, true
				}
			}
		}
	}
	return "", false
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
