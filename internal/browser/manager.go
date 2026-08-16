package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"

	"github.com/google/uuid"
)

var ErrProfileAlreadyRunning = errors.New("profile is already running")
var ErrProfileNotRunning = errors.New("profile is not running")
var ErrProfileUnderMaintenance = errors.New("profile is under maintenance")

const brunoNewTabURL = "chrome://newtab/"

type Config struct {
	ExecutablePath string
	ExtraArguments []string
	PrepareNetwork NetworkPreparer
	AttachCDP      CDPAttacher
}

type NetworkSession interface {
	Arguments() []string
	Close() error
}

type NetworkPreparer func(context.Context, string, string) (NetworkSession, error)

type CDPSession interface {
	Close() error
}

type gracefulCDPSession interface {
	Shutdown(context.Context) error
}

type CDPAttacher func(context.Context, string, string, string) (CDPSession, error)

type ProfileRepository interface {
	Get(context.Context, string) (domain.Metadata, error)
	Paths(string) (profile.Paths, error)
	RecordLaunch(context.Context, string, time.Time) (domain.Metadata, error)
}

type ProcessInfo struct {
	ProfileID   string    `json:"profileId"`
	ProfileName string    `json:"profileName"`
	Engine      string    `json:"engine"`
	PID         int       `json:"pid"`
	Executable  string    `json:"executable"`
	UserDataDir string    `json:"userDataDir"`
	StartedAt   time.Time `json:"startedAt"`
}

type processState struct {
	command        *exec.Cmd
	info           ProcessInfo
	networkSession NetworkSession
	cdpSession     CDPSession
}

type commandFactory func(string, ...string) *exec.Cmd

type Manager struct {
	repository   ProfileRepository
	config       Config
	clock        func() time.Time
	command      commandFactory
	waitDevTools func(context.Context, string) (string, error)

	mu          sync.RWMutex
	launching   map[string]struct{}
	running     map[string]*processState
	maintaining map[string]struct{}
}

func NewManager(repository ProfileRepository, config Config) (*Manager, error) {
	if repository == nil {
		return nil, errors.New("profile repository is required")
	}
	return &Manager{
		repository:   repository,
		config:       config,
		clock:        time.Now,
		command:      exec.Command,
		waitDevTools: waitForDevToolsEndpoint,
		launching:    make(map[string]struct{}),
		running:      make(map[string]*processState),
		maintaining:  make(map[string]struct{}),
	}, nil
}

func (m *Manager) Launch(ctx context.Context, profileID, requestedURL string) (ProcessInfo, error) {
	if err := ctx.Err(); err != nil {
		return ProcessInfo{}, err
	}
	parsedID, err := uuid.Parse(profileID)
	if err != nil {
		return ProcessInfo{}, profile.ErrInvalidID
	}
	canonicalID := parsedID.String()
	if err := domain.ValidateStartURL(requestedURL); err != nil {
		return ProcessInfo{}, err
	}

	m.mu.Lock()
	if _, exists := m.launching[canonicalID]; exists {
		m.mu.Unlock()
		return ProcessInfo{}, ErrProfileAlreadyRunning
	}
	if _, exists := m.running[canonicalID]; exists {
		m.mu.Unlock()
		return ProcessInfo{}, ErrProfileAlreadyRunning
	}
	if _, exists := m.maintaining[canonicalID]; exists {
		m.mu.Unlock()
		return ProcessInfo{}, ErrProfileUnderMaintenance
	}
	m.launching[canonicalID] = struct{}{}
	m.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			m.mu.Lock()
			delete(m.launching, canonicalID)
			m.mu.Unlock()
		}
	}()

	metadata, err := m.repository.Get(ctx, canonicalID)
	if err != nil {
		return ProcessInfo{}, fmt.Errorf("load profile before launch: %w", err)
	}
	paths, err := m.repository.Paths(metadata.ID)
	if err != nil {
		return ProcessInfo{}, fmt.Errorf("resolve profile paths: %w", err)
	}
	executable, err := FindExecutable(m.config.ExecutablePath)
	if err != nil {
		return ProcessInfo{}, err
	}
	if err := EnsureProfileIdentity(paths.UserData, metadata.Name); err != nil {
		return ProcessInfo{}, err
	}
	attacher := m.config.AttachCDP
	controlledStartup := attacher != nil
	if controlledStartup {
		if err := EnsureControlledStartup(paths.UserData); err != nil {
			return ProcessInfo{}, err
		}
		if err := clearDevToolsEndpoint(paths.UserData); err != nil {
			return ProcessInfo{}, err
		}
	} else if err := EnsureRestoreSession(paths.UserData); err != nil {
		return ProcessInfo{}, err
	}
	var networkSession NetworkSession
	if m.config.PrepareNetwork != nil {
		networkSession, err = m.config.PrepareNetwork(ctx, metadata.ID, paths.UserData)
		if err != nil {
			return ProcessInfo{}, fmt.Errorf("prepare profile network: %w", err)
		}
		defer func() {
			if reserved && networkSession != nil {
				_ = networkSession.Close()
			}
		}()
	}
	startURL := requestedURL
	if controlledStartup {
		if startURL == "" {
			startURL = metadata.LastURL
		}
		if startURL == "" {
			startURL = metadata.StartURL
		}
		if startURL == "" {
			startURL = neutralControlledStartURL()
		}
	} else {
		hasPreviousSession, sessionErr := HasPreviousSession(paths.UserData)
		if sessionErr != nil {
			return ProcessInfo{}, sessionErr
		}
		if startURL == "" && !hasPreviousSession {
			startURL = metadata.StartURL
		}
	}
	extensions, err := validateExtensionDirectories(metadata.ExtensionPaths)
	if err != nil {
		return ProcessInfo{}, err
	}
	if builtInExtension, exists := findBrunoStartExtension(executable); exists {
		extensions = append([]string{builtInExtension}, extensions...)
	}
	arguments, err := BuildArguments(LaunchOptions{
		UserDataDir:      paths.UserData,
		StartURL:         startupArgument(startURL, controlledStartup),
		Restore:          !controlledStartup,
		RemoteDebugging:  controlledStartup,
		Extensions:       extensions,
		ManagedArguments: networkArguments(networkSession),
		ExtraArguments:   m.config.ExtraArguments,
	})
	if err != nil {
		return ProcessInfo{}, err
	}
	command := m.command(executable, arguments...)
	command.Dir = paths.Root
	if err := command.Start(); err != nil {
		return ProcessInfo{}, fmt.Errorf("start Chromium: %w", err)
	}
	var cdpSession CDPSession
	if controlledStartup {
		websocketURL, waitErr := m.waitDevTools(ctx, paths.UserData)
		if waitErr != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return ProcessInfo{}, waitErr
		}
		cdpSession, err = attacher(ctx, metadata.ID, websocketURL, startURL)
		if err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return ProcessInfo{}, fmt.Errorf("attach fingerprint CDP: %w", err)
		}
		defer func() {
			if reserved && cdpSession != nil {
				_ = cdpSession.Close()
			}
		}()
	}
	startedAt := m.clock().UTC()
	if _, err := m.repository.RecordLaunch(ctx, metadata.ID, startedAt); err != nil {
		if cdpSession != nil {
			_ = cdpSession.Close()
		}
		_ = command.Process.Kill()
		_ = command.Wait()
		return ProcessInfo{}, fmt.Errorf("record profile launch: %w", err)
	}

	info := ProcessInfo{
		ProfileID:   metadata.ID,
		ProfileName: metadata.Name,
		Engine:      "bruno",
		PID:         command.Process.Pid,
		Executable:  executable,
		UserDataDir: paths.UserData,
		StartedAt:   startedAt,
	}
	m.mu.Lock()
	delete(m.launching, canonicalID)
	reserved = false
	m.running[metadata.ID] = &processState{
		command: command, info: info, networkSession: networkSession, cdpSession: cdpSession,
	}
	m.mu.Unlock()

	go m.wait(metadata.ID, command)
	return info, nil
}

func (m *Manager) IsRunning(profileID string) bool {
	parsedID, err := uuid.Parse(profileID)
	if err != nil {
		return false
	}
	canonicalID := parsedID.String()
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, running := m.running[canonicalID]
	_, launching := m.launching[canonicalID]
	return running || launching
}

func (m *Manager) Engine() string {
	_, err := FindExecutable(m.config.ExecutablePath)
	if err != nil {
		return "unavailable"
	}
	return "bruno"
}

// Stop closes the Chromium process owned by a Bruno profile. CDP-capable
// sessions receive Browser.close so Chromium can flush its disk-backed state.
func (m *Manager) Stop(ctx context.Context, profileID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parsedID, err := uuid.Parse(profileID)
	if err != nil {
		return profile.ErrInvalidID
	}
	canonicalID := parsedID.String()

	m.mu.RLock()
	state := m.running[canonicalID]
	_, launching := m.launching[canonicalID]
	m.mu.RUnlock()
	if state == nil {
		if launching {
			return ErrProfileAlreadyRunning
		}
		return ErrProfileNotRunning
	}
	if state.command == nil || state.command.Process == nil {
		return ErrProfileNotRunning
	}
	if graceful, ok := state.cdpSession.(gracefulCDPSession); ok {
		if err := graceful.Shutdown(ctx); err == nil {
			return nil
		}
	}
	if err := state.command.Process.Kill(); err != nil {
		return fmt.Errorf("stop Chromium profile: %w", err)
	}
	return nil
}

// BeginMaintenance atomically reserves a closed profile so a Chromium launch
// cannot start while destructive maintenance is in progress.
func (m *Manager) BeginMaintenance(profileID string) (release func(), ok bool) {
	parsedID, err := uuid.Parse(profileID)
	if err != nil {
		return nil, false
	}
	canonicalID := parsedID.String()
	m.mu.Lock()
	if _, exists := m.launching[canonicalID]; exists {
		m.mu.Unlock()
		return nil, false
	}
	if _, exists := m.running[canonicalID]; exists {
		m.mu.Unlock()
		return nil, false
	}
	if _, exists := m.maintaining[canonicalID]; exists {
		m.mu.Unlock()
		return nil, false
	}
	m.maintaining[canonicalID] = struct{}{}
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.maintaining, canonicalID)
			m.mu.Unlock()
		})
	}, true
}

func (m *Manager) Running() []ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ProcessInfo, 0, len(m.running))
	for _, process := range m.running {
		result = append(result, process.info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.Before(result[j].StartedAt)
	})
	return result
}

func (m *Manager) wait(profileID string, command *exec.Cmd) {
	_ = command.Wait()
	m.mu.Lock()
	if process, exists := m.running[profileID]; exists && process.command == command {
		delete(m.running, profileID)
		m.mu.Unlock()
		if process.cdpSession != nil {
			_ = process.cdpSession.Close()
		}
		if process.networkSession != nil {
			_ = process.networkSession.Close()
		}
		return
	}
	m.mu.Unlock()
}

func startupArgument(initialURL string, controlled bool) string {
	if controlled {
		return "about:blank"
	}
	return initialURL
}

func neutralControlledStartURL() string {
	return brunoNewTabURL
}

func networkArguments(session NetworkSession) []string {
	if session == nil {
		return nil
	}
	return session.Arguments()
}

func validateExtensionDirectories(paths []string) ([]string, error) {
	validated := make([]string, 0, len(paths))
	for _, path := range paths {
		absolutePath, err := filepath.Abs(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("normalize extension directory: %w", err)
		}
		info, err := os.Stat(absolutePath)
		if err != nil {
			return nil, fmt.Errorf("inspect extension directory %q: %w", absolutePath, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("extension path %q is not a directory", absolutePath)
		}
		validated = append(validated, absolutePath)
	}
	return validated, nil
}

func findBrunoStartExtension(executable string) (string, bool) {
	engineRoot := filepath.Dir(filepath.Dir(filepath.Clean(executable)))
	extensionRoot := filepath.Join(engineRoot, "bruno-start")
	for _, required := range []string{"manifest.json", "newtab.html", "newtab.css", "icon.png"} {
		info, err := os.Stat(filepath.Join(extensionRoot, required))
		if err != nil || info.IsDir() {
			return "", false
		}
	}
	absolute, err := filepath.Abs(extensionRoot)
	return absolute, err == nil
}
