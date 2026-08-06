package browser

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"bruno-browser/internal/domain"
)

type LaunchOptions struct {
	UserDataDir      string
	StartURL         string
	Restore          bool
	RemoteDebugging  bool
	Extensions       []string
	WayfernLabel     string
	WayfernColor     string
	ManagedArguments []string
	ExtraArguments   []string
}

var forbiddenArgumentPrefixes = []string{
	"--user-data-dir",
	"--profile-directory",
	"--incognito",
	"--guest",
	"--load-extension",
	"--disable-extensions-except",
	"--proxy-server",
	"--proxy-bypass-list",
	"--proxy-pac-url",
	"--no-proxy-server",
	"--host-resolver-rules",
	"--dns-prefetch-disable",
	"--remote-debugging-port",
	"--remote-debugging-address",
}

func BuildArguments(options LaunchOptions) ([]string, error) {
	if strings.TrimSpace(options.UserDataDir) == "" {
		return nil, errors.New("Chromium user-data directory is required")
	}
	absoluteUserDataDir, err := filepath.Abs(filepath.Clean(options.UserDataDir))
	if err != nil {
		return nil, fmt.Errorf("normalize Chromium user-data directory: %w", err)
	}
	if err := domain.ValidateStartURL(options.StartURL); err != nil {
		return nil, err
	}

	arguments := []string{
		"--user-data-dir=" + absoluteUserDataDir,
		"--profile-directory=Default",
		"--new-window",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-default-apps",
		"--disable-background-mode",
		"--disable-session-crashed-bubble",
		"--start-maximized",
	}
	if options.Restore {
		arguments = append(arguments, "--restore-last-session")
	}
	if options.RemoteDebugging {
		arguments = append(arguments,
			"--remote-debugging-address=127.0.0.1",
			"--remote-debugging-port=0",
		)
	}

	if label := strings.TrimSpace(options.WayfernLabel); label != "" {
		arguments = append(arguments, "--wayfern-profile-label="+label)
	}
	if color := strings.TrimSpace(options.WayfernColor); color != "" {
		color = strings.TrimPrefix(color, "#")
		if len(color) != 6 || strings.IndexFunc(color, func(character rune) bool {
			return !strings.ContainsRune("0123456789abcdefABCDEF", character)
		}) >= 0 {
			return nil, fmt.Errorf("wayfern profile color must use RRGGBB format")
		}
		arguments = append(arguments, "--wayfern-profile-color="+strings.ToUpper(color))
	}

	if len(options.Extensions) > 0 {
		extensionPaths := make([]string, 0, len(options.Extensions))
		for _, extensionPath := range options.Extensions {
			absolutePath, err := filepath.Abs(filepath.Clean(extensionPath))
			if err != nil {
				return nil, fmt.Errorf("normalize extension path: %w", err)
			}
			extensionPaths = append(extensionPaths, absolutePath)
		}
		arguments = append(arguments, "--load-extension="+strings.Join(extensionPaths, ","))
	}
	for _, argument := range options.ManagedArguments {
		argument = strings.TrimSpace(argument)
		if argument == "" || !strings.HasPrefix(argument, "--") {
			return nil, fmt.Errorf("invalid managed Chromium argument %q", argument)
		}
		arguments = append(arguments, argument)
	}

	for _, argument := range options.ExtraArguments {
		argument = strings.TrimSpace(argument)
		if argument == "" || !strings.HasPrefix(argument, "--") {
			return nil, fmt.Errorf("invalid Chromium argument %q", argument)
		}
		lowerArgument := strings.ToLower(argument)
		for _, prefix := range forbiddenArgumentPrefixes {
			if lowerArgument == prefix || strings.HasPrefix(lowerArgument, prefix+"=") {
				return nil, fmt.Errorf("Chromium argument %q would override profile persistence", argument)
			}
		}
		arguments = append(arguments, argument)
	}

	if options.StartURL != "" {
		arguments = append(arguments, strings.TrimSpace(options.StartURL))
	}
	return arguments, nil
}
