package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRunFile(t *testing.T, runsDir string, run Run) {
	t.Helper()
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	buf, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, run.ID+".json"), buf, 0o644); err != nil {
		t.Fatalf("write run: %v", err)
	}
}

// TestSweepStaleRunning_rewritesPastThreshold covers the new tick behavior
// that replaced the catch-up dispatcher: a `running` record older than
// StaleRunningThreshold gets rewritten as error so the dashboard stops
// showing a phantom in-flight wrapper.
func TestSweepStaleRunning_rewritesPastThreshold(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	if err := os.MkdirAll(filepath.Join(aosHome, "runs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := NewFileRunStore(aosHome)
	startedAt := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	r := Run{
		ID:            "stale-run",
		AgentID:       "ping",
		StartedAt:     FormatRunTimestamp(startedAt),
		StartedAtTime: startedAt,
		Status:        StatusRunning,
		Trigger:       "schedule",
	}
	writeRunFile(t, store.Dir(), r)

	now := startedAt.Add(2 * time.Hour)
	runs, _ := store.Load()
	n, err := SweepStaleRunning(store, runs, now, StaleRunningThreshold)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 sweep, got %d", n)
	}
	got, err := store.Get("stale-run")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusError {
		t.Errorf("status = %s, want error", got.Status)
	}
	if got.Error == nil || *got.Error != "no completion record" {
		t.Errorf("error = %v, want %q", got.Error, "no completion record")
	}
	if got.EndedAt == nil {
		t.Errorf("endedAt not set")
	}
}

func TestSweepStaleRunning_skipsUnderThreshold(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	if err := os.MkdirAll(filepath.Join(aosHome, "runs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := NewFileRunStore(aosHome)
	startedAt := time.Now().Add(-10 * time.Minute)
	writeRunFile(t, store.Dir(), Run{
		ID: "fresh-run", AgentID: "ping",
		StartedAt: FormatRunTimestamp(startedAt), StartedAtTime: startedAt,
		Status: StatusRunning,
	})
	runs, _ := store.Load()
	n, err := SweepStaleRunning(store, runs, time.Now(), StaleRunningThreshold)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 sweeps for fresh run, got %d", n)
	}
}

func TestSweepStaleRunning_skipsTerminal(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	if err := os.MkdirAll(filepath.Join(aosHome, "runs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := NewFileRunStore(aosHome)
	startedAt := time.Now().Add(-3 * time.Hour)
	for _, status := range []RunStatus{StatusSuccess, StatusError, StatusMissed} {
		writeRunFile(t, store.Dir(), Run{
			ID:        "done-" + string(status),
			AgentID:   "ping",
			StartedAt: FormatRunTimestamp(startedAt), StartedAtTime: startedAt,
			Status: status,
		})
	}
	runs, _ := store.Load()
	n, err := SweepStaleRunning(store, runs, time.Now(), StaleRunningThreshold)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 sweeps for terminal records, got %d", n)
	}
}
