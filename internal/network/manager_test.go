package network

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if localState["dns_over_https"].(map[string]any)["mode"] != "secure" {
		t.Fatal("direct profile did not receive the normal secure DNS preset")
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
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		connection, err := net.DialTimeout("tcp", proxyAddress, 25*time.Millisecond)
		if err != nil {
			break
		}
		_ = connection.Close()
		if time.Now().After(deadline) {
			t.Fatal("local proxy bridge remained open after session close")
		}
		time.Sleep(10 * time.Millisecond)
	}

	payload := readJSONMap(t, filepath.Join(paths.UserData, "Local State"))
	encoded, _ := json.Marshal(payload)
	if !strings.Contains(string(encoded), `"mode":"off"`) {
		t.Fatalf("local DNS was not disabled: %s", encoded)
	}
}

func TestSavePersistsWhileProfileIsOpenForNextLaunch(t *testing.T) {
	profiles, metadata := createNetworkTestProfile(t)
	store, _ := NewStore(profiles, testProtector{})
	manager, _ := NewManager(store)
	saved, err := manager.Save(context.Background(), metadata.ID, SaveInput{
		Mode: ModeHTTP, Host: "proxy.example.com", Port: 8080,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Mode != ModeHTTP || saved.Host != "proxy.example.com" || saved.Port != 8080 {
		t.Fatalf("unexpected saved route: %+v", saved)
	}
}
