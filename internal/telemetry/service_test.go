package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestSnapshotUsesRecordedEventsAndRealProfileState(t *testing.T) {
	now := time.Date(2026, 8, 5, 20, 0, 0, 0, time.UTC)
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.clock = func() time.Time { return now }
	if err := service.RecordLaunch(context.Background(), "profile-1", true); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordProxyTest(context.Background(), "profile-1", true, 42); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(context.Background(), []ProfileState{{
		ID: "profile-1", CreatedAt: now.Add(-24 * time.Hour), LaunchCount: 1,
		Running: true, Engine: "bruno", FingerprintReady: true, ProxyConfigured: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.RunningProfiles != 1 || snapshot.Summary.SuccessfulLaunches24h != 1 || snapshot.Summary.MedianProxyLatencyMs != 42 {
		t.Fatalf("unexpected summary: %#v", snapshot.Summary)
	}
	if snapshot.Signals.Fingerprint != 100 || snapshot.Profiles[0].Risk != "low" || snapshot.Profiles[0].FingerprintLabel != "Bruno CDP verificado" {
		t.Fatalf("unexpected signals/profile metrics: %#v %#v", snapshot.Signals, snapshot.Profiles)
	}
}

func TestTelemetryKeepsFailureAsAttentionSignal(t *testing.T) {
	now := time.Date(2026, 8, 5, 20, 0, 0, 0, time.UTC)
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.clock = func() time.Time { return now }
	if err := service.RecordLaunch(context.Background(), "profile-1", false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(context.Background(), []ProfileState{{ID: "profile-1", CreatedAt: now, Engine: "bruno"}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Signals.Sessions != 0 || snapshot.Profiles[0].Risk != "high" || snapshot.Summary.AttentionProfiles != 1 {
		t.Fatalf("failure was not represented: %#v", snapshot)
	}
}

func TestEmptySnapshotDoesNotClaimPerfectSignals(t *testing.T) {
	now := time.Date(2026, 8, 5, 20, 0, 0, 0, time.UTC)
	snapshot := buildSnapshot(now, nil, nil)
	if snapshot.Signals.Overall != 0 || snapshot.Signals.Fingerprint != 0 || snapshot.Signals.Network != 0 || snapshot.Signals.Sessions != 0 {
		t.Fatalf("empty telemetry claimed measured health: %#v", snapshot.Signals)
	}
	if snapshot.Signals.Label != "Sem dados operacionais" {
		t.Fatalf("unexpected empty label: %q", snapshot.Signals.Label)
	}
}
