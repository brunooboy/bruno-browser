package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"bruno-browser/internal/storage"

	"github.com/google/uuid"
)

const (
	currentSchemaVersion = 1
	telemetryFileName    = "telemetry.json"
	maxTelemetrySize     = 2 << 20
	maxEvents            = 2048
)

type EventKind string

const (
	LaunchSucceeded EventKind = "launch_succeeded"
	LaunchFailed    EventKind = "launch_failed"
	ProxySucceeded  EventKind = "proxy_succeeded"
	ProxyFailed     EventKind = "proxy_failed"
)

type Event struct {
	ID         string    `json:"id"`
	Kind       EventKind `json:"kind"`
	ProfileID  string    `json:"profileId"`
	OccurredAt time.Time `json:"occurredAt"`
	LatencyMs  int64     `json:"latencyMs,omitempty"`
}

type diskState struct {
	SchemaVersion int     `json:"schemaVersion"`
	Events        []Event `json:"events"`
}

type ProfileState struct {
	ID               string
	CreatedAt        time.Time
	LastLaunchedAt   *time.Time
	LaunchCount      uint64
	Running          bool
	Engine           string
	FingerprintReady bool
	ProxyConfigured  bool
}

type ActivityBucket struct {
	StartedAt       time.Time `json:"startedAt"`
	Launches        int       `json:"launches"`
	UniqueProfiles  int       `json:"uniqueProfiles"`
	ProfilesCreated int       `json:"profilesCreated"`
	ProxyTests      int       `json:"proxyTests"`
	Failures        int       `json:"failures"`
}

type ProfileMetric struct {
	ProfileID        string     `json:"profileId"`
	Engine           string     `json:"engine"`
	FingerprintScore int        `json:"fingerprintScore"`
	FingerprintLabel string     `json:"fingerprintLabel"`
	Risk             string     `json:"risk"`
	RiskReasons      []string   `json:"riskReasons"`
	ProxyLatencyMs   int64      `json:"proxyLatencyMs"`
	ProxyTested      bool       `json:"proxyTested"`
	ProxyHealthy     bool       `json:"proxyHealthy"`
	LastProxyTestAt  *time.Time `json:"lastProxyTestAt,omitempty"`
}

type Summary struct {
	TotalProfiles         int   `json:"totalProfiles"`
	NewProfilesThisMonth  int   `json:"newProfilesThisMonth"`
	RunningProfiles       int   `json:"runningProfiles"`
	SuccessfulLaunches24h int   `json:"successfulLaunches24h"`
	ConfiguredProxies     int   `json:"configuredProxies"`
	HealthyProxies        int   `json:"healthyProxies"`
	ProxyHealthPercent    int   `json:"proxyHealthPercent"`
	MedianProxyLatencyMs  int64 `json:"medianProxyLatencyMs"`
	AttentionProfiles     int   `json:"attentionProfiles"`
}

type Signals struct {
	Overall     int    `json:"overall"`
	Fingerprint int    `json:"fingerprint"`
	Network     int    `json:"network"`
	Sessions    int    `json:"sessions"`
	Label       string `json:"label"`
	Detail      string `json:"detail"`
}

type Snapshot struct {
	GeneratedAt time.Time        `json:"generatedAt"`
	Summary     Summary          `json:"summary"`
	Signals     Signals          `json:"signals"`
	Activity    []ActivityBucket `json:"activity"`
	Profiles    []ProfileMetric  `json:"profiles"`
}

type Service struct {
	path  string
	clock func() time.Time
	mu    sync.Mutex
}

func New(dataRoot string) (*Service, error) {
	if dataRoot == "" {
		return nil, errors.New("telemetry data root is required")
	}
	return &Service{path: filepath.Join(dataRoot, telemetryFileName), clock: time.Now}, nil
}

func (service *Service) RecordLaunch(ctx context.Context, profileID string, succeeded bool) error {
	kind := LaunchFailed
	if succeeded {
		kind = LaunchSucceeded
	}
	return service.record(ctx, Event{ID: uuid.NewString(), Kind: kind, ProfileID: profileID, OccurredAt: service.clock().UTC()})
}

func (service *Service) RecordProxyTest(ctx context.Context, profileID string, succeeded bool, latencyMs int64) error {
	kind := ProxyFailed
	if succeeded {
		kind = ProxySucceeded
	}
	return service.record(ctx, Event{ID: uuid.NewString(), Kind: kind, ProfileID: profileID, OccurredAt: service.clock().UTC(), LatencyMs: latencyMs})
}

func (service *Service) record(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	state, err := service.loadLocked()
	if err != nil {
		return err
	}
	state.Events = append(state.Events, event)
	cutoff := service.clock().UTC().Add(-30 * 24 * time.Hour)
	kept := state.Events[:0]
	for _, item := range state.Events {
		if !item.OccurredAt.Before(cutoff) {
			kept = append(kept, item)
		}
	}
	state.Events = kept
	if len(state.Events) > maxEvents {
		state.Events = state.Events[len(state.Events)-maxEvents:]
	}
	return storage.WriteJSONAtomic(service.path, state, 0o600)
}

func (service *Service) Snapshot(ctx context.Context, profiles []ProfileState) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	service.mu.Lock()
	state, err := service.loadLocked()
	service.mu.Unlock()
	if err != nil {
		return Snapshot{}, err
	}
	return buildSnapshot(service.clock().UTC(), profiles, state.Events), nil
}

func (service *Service) loadLocked() (diskState, error) {
	file, err := os.Open(service.path)
	if errors.Is(err, os.ErrNotExist) {
		return diskState{SchemaVersion: currentSchemaVersion, Events: []Event{}}, nil
	}
	if err != nil {
		return diskState{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxTelemetrySize+1))
	decoder.DisallowUnknownFields()
	var state diskState
	if err := decoder.Decode(&state); err != nil {
		return diskState{}, fmt.Errorf("decode telemetry: %w", err)
	}
	if state.SchemaVersion != currentSchemaVersion {
		return diskState{}, fmt.Errorf("unsupported telemetry schema version %d", state.SchemaVersion)
	}
	if len(state.Events) > maxEvents {
		return diskState{}, errors.New("telemetry contains too many events")
	}
	return state, nil
}

func buildSnapshot(now time.Time, profiles []ProfileState, events []Event) Snapshot {
	const bucketCount = 12
	const bucketDuration = 2 * time.Hour
	start := now.Truncate(bucketDuration).Add(-(bucketCount - 1) * bucketDuration)
	buckets := make([]ActivityBucket, bucketCount)
	unique := make([]map[string]struct{}, bucketCount)
	for index := range buckets {
		buckets[index].StartedAt = start.Add(time.Duration(index) * bucketDuration)
		unique[index] = map[string]struct{}{}
	}

	latestLaunch := map[string]Event{}
	latestProxy := map[string]Event{}
	recordedSuccessfulLaunch := map[string]bool{}
	launchAttempts, launchSuccesses := 0, 0
	for _, event := range events {
		switch event.Kind {
		case LaunchSucceeded, LaunchFailed:
			if event.Kind == LaunchSucceeded {
				recordedSuccessfulLaunch[event.ProfileID] = true
			}
			if previous, ok := latestLaunch[event.ProfileID]; !ok || event.OccurredAt.After(previous.OccurredAt) {
				latestLaunch[event.ProfileID] = event
			}
		case ProxySucceeded, ProxyFailed:
			if previous, ok := latestProxy[event.ProfileID]; !ok || event.OccurredAt.After(previous.OccurredAt) {
				latestProxy[event.ProfileID] = event
			}
		}
		index := int(event.OccurredAt.Sub(start) / bucketDuration)
		if index < 0 || index >= bucketCount {
			continue
		}
		switch event.Kind {
		case LaunchSucceeded:
			buckets[index].Launches++
			unique[index][event.ProfileID] = struct{}{}
			launchAttempts++
			launchSuccesses++
		case LaunchFailed:
			buckets[index].Failures++
			launchAttempts++
		case ProxySucceeded:
			buckets[index].ProxyTests++
		case ProxyFailed:
			buckets[index].ProxyTests++
			buckets[index].Failures++
		}
	}

	summary := Summary{TotalProfiles: len(profiles)}
	metrics := make([]ProfileMetric, 0, len(profiles))
	fingerprintTotal, networkTotal := 0, 0
	latencies := make([]int64, 0, len(profiles))
	for _, profile := range profiles {
		created := profile.CreatedAt.UTC()
		if created.Year() == now.Year() && created.Month() == now.Month() {
			summary.NewProfilesThisMonth++
		}
		createdBucket := int(created.Sub(start) / bucketDuration)
		if createdBucket >= 0 && createdBucket < bucketCount {
			buckets[createdBucket].ProfilesCreated++
		}
		if profile.Running {
			summary.RunningProfiles++
		}
		if profile.LastLaunchedAt != nil && !recordedSuccessfulLaunch[profile.ID] {
			knownLaunch := profile.LastLaunchedAt.UTC()
			knownBucket := int(knownLaunch.Sub(start) / bucketDuration)
			if knownBucket >= 0 && knownBucket < bucketCount {
				buckets[knownBucket].Launches++
				unique[knownBucket][profile.ID] = struct{}{}
			}
		}
		metric := profileMetric(profile, latestLaunch[profile.ID], latestProxy[profile.ID])
		metrics = append(metrics, metric)
		fingerprintTotal += metric.FingerprintScore
		networkTotal += networkScore(profile.ProxyConfigured, latestProxy[profile.ID])
		if metric.Risk == "high" {
			summary.AttentionProfiles++
		}
		if profile.ProxyConfigured {
			summary.ConfiguredProxies++
			if metric.ProxyHealthy {
				summary.HealthyProxies++
			}
			if metric.ProxyTested && metric.ProxyLatencyMs > 0 {
				latencies = append(latencies, metric.ProxyLatencyMs)
			}
		}
	}
	for index := range buckets {
		buckets[index].UniqueProfiles = len(unique[index])
		summary.SuccessfulLaunches24h += buckets[index].Launches
	}
	if summary.ConfiguredProxies > 0 {
		summary.ProxyHealthPercent = summary.HealthyProxies * 100 / summary.ConfiguredProxies
	}
	if len(latencies) > 0 {
		sort.Slice(latencies, func(left, right int) bool { return latencies[left] < latencies[right] })
		summary.MedianProxyLatencyMs = latencies[len(latencies)/2]
	}

	fingerprintSignal, networkSignal, sessionSignal := 0, 0, 0
	if len(profiles) > 0 {
		fingerprintSignal = fingerprintTotal / len(profiles)
		networkSignal = networkTotal / len(profiles)
		for _, profile := range profiles {
			if profile.LaunchCount > 0 {
				sessionSignal = 100
				break
			}
		}
	}
	if launchAttempts > 0 {
		sessionSignal = launchSuccesses * 100 / launchAttempts
	}
	overall := (fingerprintSignal*45 + networkSignal*30 + sessionSignal*25) / 100
	label, detail := "Ambiente estável", "Sinais locais verificados agora"
	if len(profiles) == 0 {
		label, detail = "Sem dados operacionais", "Crie e abra um perfil para iniciar as medições"
	} else if overall < 70 {
		label, detail = "Atenção necessária", "Existem falhas ou identidades pendentes"
	} else if overall < 90 {
		label, detail = "Ambiente em observação", "Alguns vetores ainda precisam de validação"
	}
	return Snapshot{
		GeneratedAt: now, Summary: summary, Activity: buckets, Profiles: metrics,
		Signals: Signals{Overall: overall, Fingerprint: fingerprintSignal, Network: networkSignal, Sessions: sessionSignal, Label: label, Detail: detail},
	}
}

func profileMetric(profile ProfileState, launch, proxy Event) ProfileMetric {
	metric := ProfileMetric{ProfileID: profile.ID, Engine: profile.Engine, FingerprintScore: 0, FingerprintLabel: "Identidade pendente", RiskReasons: []string{}}
	points := 0
	if profile.FingerprintReady {
		metric.FingerprintScore = 100
		if profile.Engine == "wayfern" {
			metric.FingerprintLabel = "Wayfern nativo verificado"
		} else {
			metric.FingerprintLabel = "CDP verificado"
		}
	} else if profile.LaunchCount == 0 {
		metric.FingerprintLabel = "Gerada na primeira abertura"
		metric.RiskReasons = append(metric.RiskReasons, "perfil ainda não foi aberto")
		points++
	} else {
		metric.RiskReasons = append(metric.RiskReasons, "fingerprint ainda não foi validado")
		points += 3
	}
	if launch.Kind == LaunchFailed {
		metric.RiskReasons = append(metric.RiskReasons, "última tentativa de abertura falhou")
		points += 3
	}
	if profile.ProxyConfigured {
		if proxy.Kind == "" {
			metric.RiskReasons = append(metric.RiskReasons, "proxy ainda não foi testado")
			points++
		} else {
			occurredAt := proxy.OccurredAt
			metric.LastProxyTestAt = &occurredAt
			metric.ProxyTested = true
			metric.ProxyLatencyMs = proxy.LatencyMs
			metric.ProxyHealthy = proxy.Kind == ProxySucceeded && proxy.LatencyMs < 500
			if proxy.Kind == ProxyFailed {
				metric.RiskReasons = append(metric.RiskReasons, "último teste do proxy falhou")
				points += 3
			} else if proxy.LatencyMs >= 500 {
				metric.RiskReasons = append(metric.RiskReasons, "proxy com latência alta")
				points += 2
			}
		}
	}
	switch {
	case points >= 3:
		metric.Risk = "high"
	case points > 0:
		metric.Risk = "medium"
	default:
		metric.Risk = "low"
	}
	return metric
}

func networkScore(configured bool, proxy Event) int {
	if !configured {
		return 100
	}
	if proxy.Kind == "" {
		return 50
	}
	if proxy.Kind == ProxyFailed {
		return 0
	}
	switch {
	case proxy.LatencyMs < 100:
		return 100
	case proxy.LatencyMs < 250:
		return 80
	case proxy.LatencyMs < 500:
		return 60
	default:
		return 30
	}
}
