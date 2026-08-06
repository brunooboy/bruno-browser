package network

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type proxyBridge struct {
	settings  RuntimeSettings
	listener  net.Listener
	server    *http.Server
	transport *http.Transport

	mu          sync.Mutex
	connections map[net.Conn]struct{}
	closeOnce   sync.Once
}

func startProxyBridge(settings RuntimeSettings) (*proxyBridge, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local proxy bridge: %w", err)
	}
	bridge := &proxyBridge{
		settings:    settings,
		listener:    listener,
		connections: make(map[net.Conn]struct{}),
	}
	bridge.transport = bridge.newTransport()
	bridge.server = &http.Server{
		Handler:           bridge,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		_ = bridge.server.Serve(listener)
	}()
	return bridge, nil
}

func (bridge *proxyBridge) Address() string {
	return bridge.listener.Addr().String()
}

func (bridge *proxyBridge) Close() error {
	var closeErr error
	bridge.closeOnce.Do(func() {
		closeErr = bridge.server.Close()
		if errors.Is(closeErr, http.ErrServerClosed) {
			closeErr = nil
		}
		bridge.transport.CloseIdleConnections()
		bridge.mu.Lock()
		connections := make([]net.Conn, 0, len(bridge.connections))
		for connection := range bridge.connections {
			connections = append(connections, connection)
		}
		bridge.mu.Unlock()
		for _, connection := range connections {
			_ = connection.Close()
		}
	})
	return closeErr
}

func (bridge *proxyBridge) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodConnect {
		bridge.serveTunnel(writer, request)
		return
	}
	bridge.serveHTTP(writer, request)
}

func (bridge *proxyBridge) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL == nil || request.URL.Host == "" {
		http.Error(writer, "absolute proxy URL is required", http.StatusBadRequest)
		return
	}
	outbound := request.Clone(request.Context())
	outbound.RequestURI = ""
	outbound.Header = request.Header.Clone()
	removeHopByHopHeaders(outbound.Header)
	response, err := bridge.transport.RoundTrip(outbound)
	if err != nil {
		http.Error(writer, "upstream proxy request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	removeHopByHopHeaders(response.Header)
	for key, values := range response.Header {
		for _, value := range values {
			writer.Header().Add(key, value)
		}
	}
	writer.WriteHeader(response.StatusCode)
	_, _ = io.Copy(writer, response.Body)
}

func (bridge *proxyBridge) serveTunnel(writer http.ResponseWriter, request *http.Request) {
	target := request.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		http.Error(writer, "CONNECT target must include a port", http.StatusBadRequest)
		return
	}
	upstream, err := bridge.dialTarget(request.Context(), target)
	if err != nil {
		http.Error(writer, "upstream proxy connection failed", http.StatusBadGateway)
		return
	}
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(writer, "proxy tunnel is unavailable", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	if err := buffered.Flush(); err != nil {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	bridge.track(client, true)
	bridge.track(upstream, true)
	defer func() {
		bridge.track(client, false)
		bridge.track(upstream, false)
		_ = client.Close()
		_ = upstream.Close()
	}()

	finished := make(chan struct{}, 2)
	go copyTunnel(upstream, client, finished)
	go copyTunnel(client, upstream, finished)
	<-finished
}

func (bridge *proxyBridge) dialTarget(ctx context.Context, target string) (net.Conn, error) {
	if bridge.settings.Mode == ModeSOCKS5 {
		return dialSOCKS5(ctx, bridge.settings, target)
	}
	return dialHTTPProxy(ctx, bridge.settings, target)
}

func (bridge *proxyBridge) newTransport() *http.Transport {
	transport := &http.Transport{
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	if bridge.settings.Mode == ModeSOCKS5 {
		transport.DialContext = func(ctx context.Context, _, address string) (net.Conn, error) {
			return dialSOCKS5(ctx, bridge.settings, address)
		}
		return transport
	}
	proxyURL := &url.URL{
		Scheme: string(ModeHTTP),
		Host:   net.JoinHostPort(bridge.settings.Host, strconv.Itoa(int(bridge.settings.Port))),
	}
	if bridge.settings.Username != "" {
		proxyURL.User = url.UserPassword(bridge.settings.Username, bridge.settings.Password)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return transport
}

func (bridge *proxyBridge) track(connection net.Conn, add bool) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if add {
		bridge.connections[connection] = struct{}{}
	} else {
		delete(bridge.connections, connection)
	}
}

func dialHTTPProxy(ctx context.Context, settings RuntimeSettings, target string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(settings.Host, strconv.Itoa(int(settings.Port))))
	if err != nil {
		return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = connection.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
	}
	headers := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\nProxy-Connection: keep-alive\r\n"
	if settings.Username != "" {
		credentials := base64.StdEncoding.EncodeToString([]byte(settings.Username + ":" + settings.Password))
		headers += "Proxy-Authorization: Basic " + credentials + "\r\n"
	}
	if _, err := io.WriteString(connection, headers+"\r\n"); err != nil {
		return nil, fmt.Errorf("send HTTP proxy CONNECT: %w", err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, fmt.Errorf("read HTTP proxy CONNECT response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		return nil, fmt.Errorf("HTTP proxy CONNECT failed with status %d", response.StatusCode)
	}
	_ = connection.SetDeadline(time.Time{})
	keep = true
	return &readerConn{Conn: connection, reader: reader}, nil
}

type readerConn struct {
	net.Conn
	reader *bufio.Reader
}

func (connection *readerConn) Read(payload []byte) (int, error) {
	return connection.reader.Read(payload)
}

func copyTunnel(destination, source net.Conn, finished chan<- struct{}) {
	_, _ = io.Copy(destination, source)
	if tcp, ok := destination.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	finished <- struct{}{}
}

func removeHopByHopHeaders(header http.Header) {
	connectionValues := append([]string(nil), header.Values("Connection")...)
	for _, name := range []string{
		"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		header.Del(name)
	}
	for _, value := range connectionValues {
		for _, name := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(name))
		}
	}
}
