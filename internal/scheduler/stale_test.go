package scheduler

import (
	"path/filepath"
	"testing"
	"time"
)

// TestSweepStaleRunning_skipsRaceWithWrapper covers the narrow race against
// wrapper.sh's terminal write: the in-memory snapshot the sweep loaded says
// running, but by the time we go to rewrite, the wrapper has finished and
// the on-disk record is already success. Sweep must re-read and skip,
// otherwise a clean success gets clobbered to error.
func TestSweepStaleRunning_skipsRaceWithWrapper(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	store := NewFileRunStore(aosHome)
	old := time.Now().Add(-2 * time.Hour)
	snapshot := Run{
		ID:            "raced",
		AgentID:       "ping",
		Status:        StatusRunning,
		StartedAt:     FormatRunTimestamp(old),
		StartedAtTime: old,
	}
	// Disk reflects the post-race terminal state.
	disk := snapshot
	disk.Status = StatusSuccess
	writeRunFile(t, store.Dir(), disk)

	n, err := SweepStaleRunning(store, []Run{snapshot}, time.Now(), StaleRunningThreshold)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	got, err := store.Get("raced")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Errorf("status = %q, want success (preserved)", got.Status)
	}
}

// TestSweepStaleRunning_skipsClockSkewForward covers the NTP-jump case:
// wall-clock walked forward by days while a wrapper was running. age becomes
// implausibly large; sweep should treat as skew and skip.
func TestSweepStaleRunning_skipsClockSkewForward(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	store := NewFileRunStore(aosHome)
	old := time.Now().Add(-(StaleRunningThreshold + 25*time.Hour))
	r := Run{
		ID:            "skew-fwd",
		AgentID:       "ping",
		Status:        StatusRunning,
		StartedAt:     FormatRunTimestamp(old),
		StartedAtTime: old,
	}
	writeRunFile(t, store.Dir(), r)

	n, err := SweepStaleRunning(store, []Run{r}, time.Now(), StaleRunningThreshold)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0 (skew above bound must not rewrite)", n)
	}
	got, err := store.Get("skew-fwd")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusRunning {
		t.Errorf("status = %q, want running (preserved)", got.Status)
	}
}

// TestSweepStaleRunning_skipsNegativeAge covers a clock walking backward
// (StartedAt in the future relative to now). Don't rewrite.
func TestSweepStaleRunning_skipsNegativeAge(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	store := NewFileRunStore(aosHome)
	future := time.Now().Add(2 * time.Hour)
	r := Run{
		ID:            "skew-neg",
		AgentID:       "ping",
		Status:        StatusRunning,
		StartedAt:     FormatRunTimestamp(future),
		StartedAtTime: future,
	}
	writeRunFile(t, store.Dir(), r)

	n, err := SweepStaleRunning(store, []Run{r}, time.Now(), StaleRunningThreshold)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
}
