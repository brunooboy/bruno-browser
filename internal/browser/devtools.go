package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const devToolsActivePortFile = "DevToolsActivePort"

func clearDevToolsEndpoint(userDataDir string) error {
	path := filepath.Join(userDataDir, devToolsActivePortFile)
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale DevTools endpoint: %w", err)
	}
	return nil
}

func waitForDevToolsEndpoint(ctx context.Context, userDataDir string) (string, error) {
	deadlineContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	path := filepath.Join(userDataDir, devToolsActivePortFile)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		payload, err := os.ReadFile(path)
		if err == nil {
			endpoint, parseErr := parseDevToolsActivePort(string(payload))
			if parseErr == nil {
				return endpoint, nil
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("read DevTools endpoint: %w", err)
		}
		select {
		case <-deadlineContext.Done():
			return "", fmt.Errorf("wait for Chromium DevTools endpoint: %w", deadlineContext.Err())
		case <-ticker.C:
		}
	}
}

func parseDevToolsActivePort(payload string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return "", errors.New("DevToolsActivePort is incomplete")
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("DevToolsActivePort contains an invalid port")
	}
	path := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(path, "/devtools/browser/") || strings.ContainsAny(path, "\r\n?#") {
		return "", errors.New("DevToolsActivePort contains an invalid browser path")
	}
	return "ws://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + path, nil
}
