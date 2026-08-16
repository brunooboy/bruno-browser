package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const proxyTestTarget = "example.com:443"

type Manager struct {
	store *Store
	clock func() time.Time
}

func NewManager(store *Store) (*Manager, error) {
	if store == nil {
		return nil, errors.New("network store is required")
	}
	return &Manager{store: store, clock: time.Now}, nil
}

func (m *Manager) Get(ctx context.Context, profileID string) (Settings, error) {
	return m.store.Get(ctx, profileID)
}

// RuntimeSettingsForBackup returns a short-lived plaintext copy of the proxy
// password so it can be placed inside an already encrypted backup payload.
// Callers must not persist the returned password anywhere else.
func (m *Manager) RuntimeSettingsForBackup(ctx context.Context, profileID string) (RuntimeSettings, error) {
	return m.store.Resolve(ctx, profileID)
}

func (m *Manager) Save(ctx context.Context, profileID string, input SaveInput) (Settings, error) {
	// Runtime proxy bridges resolve an immutable copy of the settings during
	// profile launch. Persisting a new route is therefore safe while a browser
	// is open; the new route takes effect on its next launch.
	return m.store.Save(ctx, profileID, input)
}

func (m *Manager) Prepare(ctx context.Context, profileID, userDataDir string) (RuntimeSession, error) {
	settings, err := m.store.Resolve(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if err := ApplyChromiumNetworkPreferences(userDataDir, settings.Mode != ModeDirect, settings.DNSPreset); err != nil {
		return nil, err
	}
	if settings.Mode == ModeDirect {
		return &session{}, nil
	}
	bridge, err := startProxyBridge(settings)
	if err != nil {
		return nil, err
	}
	arguments := []string{
		"--proxy-server=http://" + bridge.Address(),
		"--dns-prefetch-disable",
		"--host-resolver-rules=" + hostResolverRules(settings.BypassList),
	}
	if len(settings.BypassList) > 0 {
		arguments = append(arguments, "--proxy-bypass-list="+strings.Join(settings.BypassList, ";"))
	}
	return &session{arguments: arguments, close: bridge.Close}, nil
}

func (m *Manager) TestProxy(ctx context.Context, profileID string) (TestResult, error) {
	settings, err := m.store.Resolve(ctx, profileID)
	if err != nil {
		return TestResult{}, err
	}
	if settings.Mode == ModeDirect {
		return TestResult{
			ProfileID: settings.ProfileID,
			Mode:      settings.Mode,
			Endpoint:  "direct://system",
		}, nil
	}
	testContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	startedAt := m.clock()
	var connection net.Conn
	if settings.Mode == ModeSOCKS5 {
		connection, err = dialSOCKS5(testContext, settings, proxyTestTarget)
	} else {
		connection, err = dialHTTPProxy(testContext, settings, proxyTestTarget)
	}
	if err != nil {
		return TestResult{}, fmt.Errorf("test proxy route: %w", err)
	}
	_ = connection.Close()
	return TestResult{
		ProfileID: settings.ProfileID,
		Mode:      settings.Mode,
		LatencyMs: m.clock().Sub(startedAt).Milliseconds(),
		Endpoint:  proxyEndpoint(settings.Mode, settings.Host, settings.Port),
	}, nil
}

type RuntimeSession interface {
	Arguments() []string
	Close() error
}

type session struct {
	arguments []string
	close     func() error
	once      sync.Once
	err       error
}

func (session *session) Arguments() []string {
	return slices.Clone(session.arguments)
}

func (session *session) Close() error {
	session.once.Do(func() {
		if session.close != nil {
			session.err = session.close()
		}
	})
	return session.err
}

func hostResolverRules(bypassList []string) string {
	rules := []string{"MAP * ~NOTFOUND", "EXCLUDE localhost", "EXCLUDE 127.0.0.1", "EXCLUDE ::1"}
	for _, rule := range bypassList {
		if rule == "<local>" || rule == "<-loopback>" {
			continue
		}
		if _, _, err := net.ParseCIDR(strings.TrimPrefix(rule, "*.")); err == nil {
			continue
		}
		rules = append(rules, "EXCLUDE "+filepath.ToSlash(rule))
	}
	return strings.Join(rules, ", ")
}
