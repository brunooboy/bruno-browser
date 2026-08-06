package network

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDialHTTPProxySuppliesCredentials(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer connection.Close()
		request, err := http.ReadRequest(bufio.NewReader(connection))
		if err != nil {
			result <- err
			return
		}
		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("operator:secret"))
		if request.Method != http.MethodConnect || request.Host != "account.example:443" {
			result <- fmt.Errorf("unexpected CONNECT request: %s %s", request.Method, request.Host)
			return
		}
		if request.Header.Get("Proxy-Authorization") != expected {
			result <- fmt.Errorf("missing proxy credentials")
			return
		}
		_, err = io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\n\r\n")
		result <- err
	}()

	host, port := splitListenerAddress(t, listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, err := dialHTTPProxy(ctx, RuntimeSettings{
		Settings: Settings{Mode: ModeHTTP, Host: host, Port: port, Username: "operator"},
		Password: "secret",
	}, "account.example:443")
	if err != nil {
		t.Fatalf("dialHTTPProxy: %v", err)
	}
	_ = connection.Close()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestDialSOCKS5SendsDomainAndCredentialsToProxy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		header := make([]byte, 2)
		if _, err := io.ReadFull(reader, header); err != nil {
			result <- err
			return
		}
		methods := make([]byte, int(header[1]))
		if _, err := io.ReadFull(reader, methods); err != nil || !strings.Contains(string(methods), string([]byte{0x02})) {
			result <- fmt.Errorf("SOCKS5 username/password method missing: %v", err)
			return
		}
		_, _ = connection.Write([]byte{0x05, 0x02})
		if _, err := io.ReadFull(reader, header); err != nil {
			result <- err
			return
		}
		username := make([]byte, int(header[1]))
		_, _ = io.ReadFull(reader, username)
		passwordLength, _ := reader.ReadByte()
		password := make([]byte, int(passwordLength))
		_, _ = io.ReadFull(reader, password)
		if string(username) != "operator" || string(password) != "secret" {
			result <- fmt.Errorf("unexpected SOCKS5 credentials")
			return
		}
		_, _ = connection.Write([]byte{0x01, 0x00})
		connectHeader := make([]byte, 5)
		if _, err := io.ReadFull(reader, connectHeader); err != nil {
			result <- err
			return
		}
		if connectHeader[0] != 0x05 || connectHeader[1] != 0x01 || connectHeader[3] != 0x03 {
			result <- fmt.Errorf("unexpected SOCKS5 connect header: %v", connectHeader)
			return
		}
		hostLength := int(connectHeader[4])
		domain := make([]byte, hostLength)
		_, _ = io.ReadFull(reader, domain)
		portBytes := make([]byte, 2)
		_, _ = io.ReadFull(reader, portBytes)
		if string(domain) != "account.example" {
			result <- fmt.Errorf("target was not sent as a domain: %q", domain)
			return
		}
		_, err = connection.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0, 0})
		result <- err
	}()

	host, port := splitListenerAddress(t, listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, err := dialSOCKS5(ctx, RuntimeSettings{
		Settings: Settings{Mode: ModeSOCKS5, Host: host, Port: port, Username: "operator"},
		Password: "secret",
	}, "account.example:443")
	if err != nil {
		t.Fatalf("dialSOCKS5: %v", err)
	}
	_ = connection.Close()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func splitListenerAddress(t *testing.T, address string) (string, uint16) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	return host, uint16(port)
}
