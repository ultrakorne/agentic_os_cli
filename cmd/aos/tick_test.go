package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// installFakeWrapper drops a tiny bash wrapper at <aosHome>/wrapper.sh that
// logs its argv + AGENTIC_OS_TRIGGER to <aosHome>/wrapper.log. Returns the
// log path so tests can poll it.
func installFakeWrapper(t *testing.T, aosHome string) string {
	t.Helper()
	if err := os.MkdirAll(aosHome, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	wrapper := filepath.Join(aosHome, "wrapper.sh")
	log := filepath.Join(aosHome, "wrapper.log")
	body := "#!/usr/bin/env bash\n" +
		"echo \"$1|$2|$3|$4|$5|trigger=$AGENTIC_OS_TRIGGER\" >> \"" + log + "\"\n"
	if err := os.WriteFile(wrapper, []byte(body), 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	return log
}

func writeRunFile(t *testing.T, runsDir string, run scheduler.Run) {
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

func waitForLines(t *testing.T, path string, want int, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lines []string
	for time.Now().Before(deadline) {
		f, err := os.Open(path)
		if err == nil {
			s := bufio.NewScanner(f)
			lines = lines[:0]
			for s.Scan() {
				lines = append(lines, s.Text())
			}
			f.Close()
			if len(lines) >= want {
				return lines
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return lines
}

// TestFireCatchups_spawnsForMissedLatest covers the integration: a real
// agent script + wrapper on disk + a Run{status:"missed"} record → one
// catch-up wrapper invocation with the missed slot as scheduleId.
func TestFireCatchups_spawnsForMissedLatest(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	log := installFakeWrapper(t, aosHome)

	scriptPath := filepath.Join(aosHome, "agents", "ping.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	missedSlot := "2026-05-17T11:00:00Z"
	writeRunFile(t, filepath.Join(aosHome, "runs"), scheduler.Run{
		ID:        "miss-ping-2026-05-17T11-00-00Z",
		AgentID:     "ping",
		StartedAt: missedSlot,
		Status:    scheduler.StatusMissed,
		Trigger:   "schedule",
	})

	agents := []scheduler.Agent{{
		ID:         "ping",
		ScriptPath: scriptPath,
		Meta: scheduler.AgentMeta{
			Schedule: &scheduler.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0},
		},
	}}

	runs, err := scheduler.LoadRuns(filepath.Join(aosHome, "runs"))
	if err != nil {
		t.Fatalf("LoadRuns: %v", err)
	}
	fired, err := fireCatchups(aosHome, agents, runs)
	if err != nil {
		t.Fatalf("fireCatchups: %v", err)
	}
	if fired != 1 {
		t.Fatalf("fired = %d, want 1", fired)
	}

	lines := waitForLines(t, log, 1, 2*time.Second)
	if len(lines) != 1 {
		t.Fatalf("wrapper log lines = %d, want 1: %v", len(lines), lines)
	}
	// argv contract: <aos-home>|<sched-id>|<agent-id>|<script>|<run-id>|trigger=catch-up
	want := aosHome + "|" + missedSlot + "|ping|" + scriptPath + "|"
	if got := lines[0]; len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("wrapper invocation = %q, want prefix %q", got, want)
	}
	if got := lines[0]; got[len(got)-len("|trigger=catch-up"):] != "|trigger=catch-up" {
		t.Errorf("wrapper trigger env = %q, want suffix |trigger=catch-up", got)
	}
}

func TestFireCatchups_noopWhenLatestIsCompleted(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	log := installFakeWrapper(t, aosHome)

	scriptPath := filepath.Join(aosHome, "agents", "ping.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	runsDir := filepath.Join(aosHome, "runs")
	writeRunFile(t, runsDir, scheduler.Run{
		ID:        "miss-ping",
		AgentID:     "ping",
		StartedAt: "2026-05-17T11:00:00Z",
		Status:    scheduler.StatusMissed,
		Trigger:   "schedule",
	})
	writeRunFile(t, runsDir, scheduler.Run{
		ID:        "later-success",
		AgentID:     "ping",
		StartedAt: "2026-05-17T11:30:00Z",
		Status:    scheduler.StatusSuccess,
		Trigger:   "schedule",
	})

	agents := []scheduler.Agent{{
		ID:         "ping",
		ScriptPath: scriptPath,
		Meta: scheduler.AgentMeta{
			Schedule: &scheduler.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0},
		},
	}}

	runs, err := scheduler.LoadRuns(runsDir)
	if err != nil {
		t.Fatalf("LoadRuns: %v", err)
	}
	fired, err := fireCatchups(aosHome, agents, runs)
	if err != nil {
		t.Fatalf("fireCatchups: %v", err)
	}
	if fired != 0 {
		t.Fatalf("fired = %d, want 0", fired)
	}
	// Sleep briefly so a wrongly-spawned wrapper would have time to write.
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Errorf("wrapper.log should not exist (no spawn), stat err = %v", err)
	}
}

func TestFireCatchups_missingWrapperReportsError(t *testing.T) {
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	if err := os.MkdirAll(aosHome, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	// No wrapper.sh on disk.

	if _, err := fireCatchups(aosHome, nil, nil); err == nil {
		t.Error("fireCatchups returned nil error despite missing wrapper")
	}
}
