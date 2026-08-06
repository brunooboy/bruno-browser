package network

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareDirectConfiguresDNSAndWebRTCWithoutProxy(t *testing.T) {
	profiles, metadata := createNetworkTestProfile(t)
	store, _ := NewStore(profiles, testProtector{})
	manager, _ := NewManager(store)
	paths, _ := profiles.Paths(metadata.ID)
	session, err := manager.Prepare(context.Background(), metadata.ID, paths.UserData)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if len(session.Arguments()) != 0 {
		t.Fatalf("direct mode received proxy flags: %v", session.Arguments())
	}
	localState := readJSONMap(t, filepath.Join(paths.UserData, "Local State"))
	if localState["dns_over_https"].(map[string]any)["mode"] != "automatic" {
		t.Fatal("direct profile did not receive automatic DNS")
	}
}

func TestPrepareProxyStartsLocalBridgeAndBlocksLocalDNS(t *testing.T) {
	profiles, metadata := createNetworkTestProfile(t)
	store, _ := NewStore(profiles, testProtector{})
	if _, err := store.Save(context.Background(), metadata.ID, SaveInput{
		Mode: ModeHTTP, Host: "proxy.example.com", Port: 8080,
	}); err != nil {
		t.Fatal(err)
	}
	manager, _ := NewManager(store)
	paths, _ := profiles.Paths(metadata.ID)
	session, err := manager.Prepare(context.Background(), metadata.ID, paths.UserData)
	if err != nil {
		t.Fatal(err)
	}
	arguments := strings.Join(session.Arguments(), "\n")
	if !strings.Contains(arguments, "--proxy-server=http://127.0.0.1:") ||
		!strings.Contains(arguments, "--dns-prefetch-disable") ||
		!strings.Contains(arguments, "MAP * ~NOTFOUND") {
		t.Fatalf("proxy privacy arguments missing: %v", session.Arguments())
	}
	proxyAddress := strings.TrimPrefix(session.Arguments()[0], "--proxy-server=http://")
	if connection, err := net.Dial("tcp", proxyAddress); err != nil {
		t.Fatalf("local proxy bridge is not listening: %v", err)
	} else {
		_ = connection.Close()
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := net.Dial("tcp", proxyAddress); err == nil {
		t.Fatal("local proxy bridge remained open after session close")
	}

	payload := readJSONMap(t, filepath.Join(paths.UserData, "Local State"))
	encoded, _ := json.Marshal(payload)
	if !strings.Contains(string(encoded), `"mode":"off"`) {
		t.Fatalf("local DNS was not disabled: %s", encoded)
	}
}

func TestSaveRejectsProfileReservedByBrowser(t *testing.T) {
	profiles, metadata := createNetworkTestProfile(t)
	store, _ := NewStore(profiles, testProtector{})
	manager, _ := NewManager(store)
	if err := manager.SetProcessState(busyProfileReservation{}); err != nil {
		t.Fatal(err)
	}
	_, err := manager.Save(context.Background(), metadata.ID, SaveInput{
		Mode: ModeHTTP, Host: "proxy.example.com", Port: 8080,
	})
	if !errors.Is(err, ErrProfileRunning) {
		t.Fatalf("expected ErrProfileRunning, got %v", err)
	}
}

type busyProfileReservation struct{}

func (busyProfileReservation) BeginMaintenance(string) (func(), bool) {
	return nil, false
}
