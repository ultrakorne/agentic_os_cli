package scheduler

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func runsDir(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	return home, dir
}

func TestRecordMissedRuns_writesOneMissPerAgent(t *testing.T) {
	home, dir := runsDir(t)
	now := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-2*time.Hour))

	written, err := RecordMissedRuns(home, []Agent{a}, now)
	if err != nil {
		t.Fatalf("RecordMissedRuns: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected 1 newly written miss, got %d", len(written))
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file on disk, got %d (%v)", len(files), files)
	}

	// Round-trip: ReadRuns should surface the miss with StatusMissed and
	// the expected slot in startedAt.
	runs, err := ReadRuns(dir, "", 0)
	if err != nil {
		t.Fatalf("ReadRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != StatusMissed {
		t.Fatalf("ReadRuns: %+v", runs)
	}
	if runs[0].EndedAt != nil {
		t.Fatalf("missed run should have endedAt=nil, got %v", *runs[0].EndedAt)
	}
	if runs[0].ExitCode != nil {
		t.Fatalf("missed run should have exitCode=nil")
	}
}

func TestRecordMissedRuns_isIdempotent(t *testing.T) {
	home, dir := runsDir(t)
	now := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-2*time.Hour))

	if _, err := RecordMissedRuns(home, []Agent{a}, now); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Snapshot mtime to prove the second call doesn't churn the file.
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	missFile := filepath.Join(dir, files[0].Name())
	before, err := os.Stat(missFile)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Sleep so a rewrite would yield a distinguishable mtime.
	time.Sleep(1100 * time.Millisecond)

	written, err := RecordMissedRuns(home, []Agent{a}, now)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(written) != 0 {
		t.Fatalf("expected 0 newly written (already on disk), got %d", len(written))
	}
	after, err := os.Stat(missFile)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("file was rewritten on idempotent call: %v → %v", before.ModTime(), after.ModTime())
	}
}

func TestRecordMissedRuns_replacesPreviousMissForSameAgent(t *testing.T) {
	// Two-tick scenario: outage starts at slot N, next tick slot N+1 is also
	// uncovered. After tick 2, only the slot-N+1 file should remain on disk.
	home, dir := runsDir(t)
	tickOne := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, tickOne.Add(-3*time.Hour))

	if _, err := RecordMissedRuns(home, []Agent{a}, tickOne); err != nil {
		t.Fatalf("tick 1: %v", err)
	}

	tickTwo := tickOne.Add(time.Hour) // 13:30 — slot 13:00 is now the latest.
	written, err := RecordMissedRuns(home, []Agent{a}, tickTwo)
	if err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected 1 new miss recorded on tick 2, got %d", len(written))
	}

	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected exactly 1 miss file after replacement, got %d (%v)", len(files), files)
	}
	expectedNewID := MissedRunID("ping", time.Date(2026, 5, 16, 13, 0, 0, 0, time.UTC))
	if files[0].Name() != expectedNewID+".json" {
		t.Fatalf("expected file %q, got %q", expectedNewID+".json", files[0].Name())
	}

	// The slot-12 file should be gone.
	staleID := MissedRunID("ping", time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC))
	if _, err := os.Stat(filepath.Join(dir, staleID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale miss not deleted: stat err=%v", err)
	}
}

func TestRecordMissedRuns_keepsHistoricalMissAfterRealRun(t *testing.T) {
	// A miss is recorded; then the user manually runs (a real success).
	// On the next tick: DetectMissed returns nothing (the run covers any
	// future slot via rule b/c), so RecordMissedRuns writes nothing and
	// leaves the old miss file in place — the agent's history of having
	// been behind once is preserved.
	home, dir := runsDir(t)
	tickOne := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, tickOne.Add(-2*time.Hour))

	if _, err := RecordMissedRuns(home, []Agent{a}, tickOne); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	beforeFiles, _ := os.ReadDir(dir)
	if len(beforeFiles) != 1 {
		t.Fatalf("setup: expected 1 file, got %d", len(beforeFiles))
	}
	missName := beforeFiles[0].Name()

	// User manually runs at 12:35 — write a success record into runs/.
	runID := "manual-1234"
	endedAt := tickOne.Add(5 * time.Minute).Format(time.RFC3339Nano)
	exit := 0
	manual := JobRun{
		ID:        runID,
		JobID:     "ping",
		Trigger:   "manual",
		StartedAt: tickOne.Add(time.Minute).Format(time.RFC3339Nano),
		EndedAt:   &endedAt,
		Status:    StatusSuccess,
		ExitCode:  &exit,
	}
	buf, _ := json.MarshalIndent(manual, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, runID+".json"), buf, 0o644); err != nil {
		t.Fatalf("write manual run: %v", err)
	}

	// Tick 2: no new miss should be recorded, and the old one stays.
	tickTwo := tickOne.Add(10 * time.Minute) // 12:40 — still inside the 12:00 slot window
	written, err := RecordMissedRuns(home, []Agent{a}, tickTwo)
	if err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if len(written) != 0 {
		t.Fatalf("expected 0 new misses after manual run, got %d", len(written))
	}
	if _, err := os.Stat(filepath.Join(dir, missName)); err != nil {
		t.Fatalf("historical miss file was removed: %v", err)
	}
}

func TestRecordMissedRuns_noScheduleNoOp(t *testing.T) {
	home, _ := runsDir(t)
	a := Agent{ID: "unscheduled"} // no Meta.Schedule

	written, err := RecordMissedRuns(home, []Agent{a}, time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(written) != 0 {
		t.Fatalf("expected 0 misses for unscheduled agent, got %d", len(written))
	}
}
