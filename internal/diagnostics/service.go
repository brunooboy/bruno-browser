package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"bruno-browser/internal/account"
	"bruno-browser/internal/browser"
	"bruno-browser/internal/domain"
	"bruno-browser/internal/extensions"
	"bruno-browser/internal/license"
	"bruno-browser/internal/network"
	"bruno-browser/internal/profile"
	"bruno-browser/internal/storage"
	"bruno-browser/internal/updates"
)

const (
	logFileName     = "diagnostics-log.json"
	maxLogEntries   = 100
	maxLogFileSize  = 2 << 20
	maxMessageRunes = 600
)

var credentialURLPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)([^/@\s]+)@`)

type CheckStatus string

const (
	CheckPass    CheckStatus = "pass"
	CheckWarning CheckStatus = "warning"
	CheckFail    CheckStatus = "fail"
)

type Check struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail"`
}

type Incident struct {
	At      time.Time `json:"at"`
	Scope   string    `json:"scope"`
	Message string    `json:"message"`
}

type Report struct {
	GeneratedAt time.Time  `json:"generatedAt"`
	Status      string     `json:"status"`
	Checks      []Check    `json:"checks"`
	Incidents   []Incident `json:"incidents"`
}

type Service struct {
	dataRoot   string
	profiles   *profile.Store
	browser    *browser.Manager
	network    *network.Manager
	extensions *extensions.Service
	account    *account.Service
	license    *license.Service
	updates    *updates.Service
	clock      func() time.Time
	mu         sync.Mutex
}

func New(
	dataRoot string,
	profiles *profile.Store,
	browserManager *browser.Manager,
	networkManager *network.Manager,
	extensionService *extensions.Service,
	accountService *account.Service,
	licenseService *license.Service,
	updateService *updates.Service,
) (*Service, error) {
	if strings.TrimSpace(dataRoot) == "" || profiles == nil || browserManager == nil || networkManager == nil ||
		extensionService == nil || accountService == nil || licenseService == nil || updateService == nil {
		return nil, errors.New("diagnostics dependencies are incomplete")
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(dataRoot))
	if err != nil {
		return nil, fmt.Errorf("normalize diagnostics root: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create diagnostics root: %w", err)
	}
	return &Service{
		dataRoot: absoluteRoot, profiles: profiles, browser: browserManager,
		network: networkManager, extensions: extensionService, account: accountService,
		license: licenseService, updates: updateService, clock: time.Now,
	}, nil
}

func (s *Service) Run(ctx context.Context) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	checks := make([]Check, 0, 6)
	checks = append(checks, s.checkStorage())
	checks = append(checks, s.checkEngine())

	profiles, profileErr := s.profiles.List(ctx)
	if profileErr != nil {
		checks = append(checks, failed("profiles", "Perfis e metadados", profileErr))
		checks = append(checks, Check{ID: "network", Label: "Proxy e DNS", Status: CheckWarning, Detail: "Aguardando a leitura dos perfis"})
	} else {
		checks = append(checks, s.checkProfiles(profiles))
		checks = append(checks, s.checkNetwork(ctx, profiles))
	}
	checks = append(checks, s.checkExtensions(ctx))
	checks = append(checks, s.checkAccountAndLicense(ctx))
	checks = append(checks, s.checkUpdates(ctx))

	incidents, incidentErr := s.Incidents(ctx)
	if incidentErr != nil {
		checks = append(checks, failed("diagnostic_log", "Registro de falhas", incidentErr))
		incidents = []Incident{}
	}
	return Report{
		GeneratedAt: s.clock().UTC(), Status: overallStatus(checks),
		Checks: checks, Incidents: incidents,
	}, nil
}

func (s *Service) Record(ctx context.Context, scope string, operationErr error) error {
	if operationErr == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	incident := Incident{
		At: s.clock().UTC(), Scope: sanitizeScope(scope), Message: sanitizeMessage(operationErr.Error()),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	incidents, err := s.readIncidents()
	if err != nil {
		// A damaged diagnostic log must never block the operation that is being
		// diagnosed. Starting a fresh bounded log is the recovery path.
		incidents = nil
	}
	incidents = append([]Incident{incident}, incidents...)
	if len(incidents) > maxLogEntries {
		incidents = incidents[:maxLogEntries]
	}
	return storage.WriteJSONAtomic(s.logPath(), incidents, 0o600)
}

func (s *Service) Incidents(ctx context.Context) ([]Incident, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readIncidents()
}

func (s *Service) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.logPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear diagnostics log: %w", err)
	}
	return nil
}

func (s *Service) checkStorage() Check {
	probe, err := os.CreateTemp(s.dataRoot, ".bruno-diagnostic-*")
	if err != nil {
		return failed("storage", "Armazenamento local", err)
	}
	probePath := probe.Name()
	defer os.Remove(probePath)
	_, writeErr := probe.Write([]byte("bruno-browser-storage-check\n"))
	syncErr := probe.Sync()
	closeErr := probe.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return failed("storage", "Armazenamento local", err)
	}
	return Check{ID: "storage", Label: "Armazenamento local", Status: CheckPass, Detail: "Pasta persistente disponível para leitura e gravação"}
}

func (s *Service) checkEngine() Check {
	executable, err := s.browser.Executable()
	if err != nil {
		return failed("engine", "Bruno Engine", err)
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return failed("engine", "Bruno Engine", errors.New("executável do motor não está acessível"))
	}
	return Check{ID: "engine", Label: "Bruno Engine", Status: CheckPass, Detail: fmt.Sprintf("%s validado (%d MB)", filepath.Base(executable), info.Size()/(1<<20))}
}

func (s *Service) checkProfiles(metadataList []domain.Metadata) Check {
	for _, metadata := range metadataList {
		paths, err := s.profiles.Paths(metadata.ID)
		if err != nil {
			return failed("profiles", "Perfis e metadados", err)
		}
		for _, required := range []struct {
			path      string
			directory bool
		}{
			{path: paths.Metadata},
			{path: paths.MetadataBackup},
			{path: paths.UserData, directory: true},
			{path: paths.Extensions, directory: true},
		} {
			info, statErr := os.Stat(required.path)
			if statErr != nil || info.IsDir() != required.directory {
				return Check{ID: "profiles", Label: "Perfis e metadados", Status: CheckFail, Detail: fmt.Sprintf("Estrutura incompleta no perfil %s", metadata.Name)}
			}
		}
	}
	return Check{ID: "profiles", Label: "Perfis e metadados", Status: CheckPass, Detail: fmt.Sprintf("%d perfil(is) com estrutura física válida", len(metadataList))}
}

func (s *Service) checkNetwork(ctx context.Context, metadataList []domain.Metadata) Check {
	configured := 0
	for _, metadata := range metadataList {
		settings, err := s.network.Get(ctx, metadata.ID)
		if err != nil {
			return Check{ID: "network", Label: "Proxy e DNS", Status: CheckFail, Detail: fmt.Sprintf("Configuração inválida no perfil %s: %s", metadata.Name, sanitizeMessage(err.Error()))}
		}
		if !settings.DNSPreset.Valid() || !settings.Mode.Valid() {
			return Check{ID: "network", Label: "Proxy e DNS", Status: CheckFail, Detail: fmt.Sprintf("Rota não reconhecida no perfil %s", metadata.Name)}
		}
		if settings.Mode != network.ModeDirect {
			configured++
		}
	}
	return Check{ID: "network", Label: "Proxy e DNS", Status: CheckPass, Detail: fmt.Sprintf("%d rota(s) proxy e %d política(s) DNS validadas", configured, len(metadataList))}
}

func (s *Service) checkExtensions(ctx context.Context) Check {
	installed, err := s.extensions.List(ctx)
	if err != nil {
		return failed("extensions", "Biblioteca de extensões", err)
	}
	for _, extension := range installed {
		manifestPath := filepath.Join(extension.Path, "manifest.json")
		if info, statErr := os.Stat(manifestPath); statErr != nil || !info.Mode().IsRegular() {
			return Check{ID: "extensions", Label: "Biblioteca de extensões", Status: CheckFail, Detail: fmt.Sprintf("%s está incompleta", extension.Name)}
		}
	}
	return Check{ID: "extensions", Label: "Biblioteca de extensões", Status: CheckPass, Detail: fmt.Sprintf("%d extensão(ões) validada(s)", len(installed))}
}

func (s *Service) checkAccountAndLicense(ctx context.Context) Check {
	user, err := s.account.Get(ctx)
	if err != nil {
		return failed("license", "Conta e licença", err)
	}
	if user == nil {
		return Check{ID: "license", Label: "Conta e licença", Status: CheckWarning, Detail: "Entre com Discord para habilitar os recursos premium"}
	}
	activation, err := s.license.Status(ctx, user.ID)
	if err != nil {
		return failed("license", "Conta e licença", err)
	}
	if !activation.Activated || activation.Status != "active" {
		return Check{ID: "license", Label: "Conta e licença", Status: CheckWarning, Detail: "Conta local válida, mas sem plano ativo"}
	}
	return Check{ID: "license", Label: "Conta e licença", Status: CheckPass, Detail: fmt.Sprintf("Plano %s validado novamente pelo núcleo", activation.Plan)}
}

func (s *Service) checkUpdates(ctx context.Context) Check {
	status, err := s.updates.Current(ctx)
	if err != nil {
		return failed("updates", "Canal de atualização", err)
	}
	return Check{ID: "updates", Label: "Canal de atualização", Status: CheckPass, Detail: "Manifesto local v" + status.CurrentVersion + " íntegro"}
}

func (s *Service) readIncidents() ([]Incident, error) {
	file, err := os.Open(s.logPath())
	if errors.Is(err, os.ErrNotExist) {
		return []Incident{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open diagnostics log: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxLogFileSize+1))
	decoder.DisallowUnknownFields()
	var incidents []Incident
	if err := decoder.Decode(&incidents); err != nil {
		return nil, fmt.Errorf("decode diagnostics log: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("diagnostics log contains trailing JSON content")
	}
	if len(incidents) > maxLogEntries {
		incidents = incidents[:maxLogEntries]
	}
	return incidents, nil
}

func (s *Service) logPath() string { return filepath.Join(s.dataRoot, logFileName) }

func failed(id, label string, err error) Check {
	return Check{ID: id, Label: label, Status: CheckFail, Detail: sanitizeMessage(err.Error())}
}

func overallStatus(checks []Check) string {
	status := "ready"
	for _, check := range checks {
		if check.Status == CheckFail {
			return "blocked"
		}
		if check.Status == CheckWarning {
			status = "attention"
		}
	}
	return status
}

func sanitizeScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	var result strings.Builder
	for _, character := range scope {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-' {
			result.WriteRune(character)
		}
	}
	if result.Len() == 0 {
		return "core"
	}
	return result.String()
}

func sanitizeMessage(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	message = credentialURLPattern.ReplaceAllString(message, `${1}***@`)
	runes := []rune(message)
	if len(runes) > maxMessageRunes {
		message = string(runes[:maxMessageRunes]) + "…"
	}
	if message == "" {
		return "falha sem detalhes"
	}
	return message
}
