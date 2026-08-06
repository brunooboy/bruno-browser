package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDevToolsActivePortAcceptsOnlyLoopbackBrowserEndpoint(t *testing.T) {
	endpoint, err := parseDevToolsActivePort("49152\n/devtools/browser/4c8fa01b\n")
	if err != nil {
		t.Fatalf("parseDevToolsActivePort: %v", err)
	}
	if endpoint != "ws://127.0.0.1:49152/devtools/browser/4c8fa01b" {
		t.Fatalf("unexpected endpoint: %s", endpoint)
	}
	for _, invalid := range []string{"0\n/devtools/browser/id", "9222\n/devtools/page/id", "9222\n/devtools/browser/id?x=1"} {
		if _, err := parseDevToolsActivePort(invalid); err == nil {
			t.Fatalf("expected invalid endpoint to be rejected: %q", invalid)
		}
	}
}

func TestWaitForDevToolsEndpointWaitsForAtomicChromiumFile(t *testing.T) {
	userData := t.TempDir()
	go func() {
		time.Sleep(40 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(userData, devToolsActivePortFile), []byte("53333\n/devtools/browser/test-id\n"), 0o600)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	endpoint, err := waitForDevToolsEndpoint(ctx, userData)
	if err != nil {
		t.Fatalf("waitForDevToolsEndpoint: %v", err)
	}
	if endpoint != "ws://127.0.0.1:53333/devtools/browser/test-id" {
		t.Fatalf("unexpected endpoint: %s", endpoint)
	}
}
