package scheduler

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRunID_isUnique(t *testing.T) {
	a := NewRunID()
	b := NewRunID()
	if a == b {
		t.Fatalf("NewRunID returned the same value twice: %s", a)
	}
	if !strings.Contains(a, "-") {
		t.Errorf("NewRunID format unexpected: %q", a)
	}
}

// TestSpawnWrapperDetached_passesArgsAndTriggerEnv runs a fake wrapper that
// records its argv and AGENTIC_OS_TRIGGER to a log file, then asserts both.
// Locks in the wrapper argv contract: <aos_home> <schedule-id> <agent-id>
// <script> <run-id>. Also pins the per-trigger env wiring callers depend on
// (manual / catch-up records distinguishable from schedule runs).
func TestSpawnWrapperDetached_passesArgsAndTriggerEnv(t *testing.T) {
	cases := []struct {
		name       string
		scheduleID string
		trigger    string
	}{
		{"manual", "", "manual"},
		{"catch-up", "2026-05-17T09:00:00Z", "catch-up"},
		{"schedule default", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			wrapper := filepath.Join(tmp, "wrapper.sh")
			log := filepath.Join(tmp, "wrapper.log")
			body := "#!/usr/bin/env bash\n" +
				"echo \"$1\" >> \"" + log + "\"\n" +
				"echo \"$2\" >> \"" + log + "\"\n" +
				"echo \"$3\" >> \"" + log + "\"\n" +
				"echo \"$4\" >> \"" + log + "\"\n" +
				"echo \"$5\" >> \"" + log + "\"\n" +
				"echo \"trigger=$AGENTIC_OS_TRIGGER\" >> \"" + log + "\"\n"
			if err := os.WriteFile(wrapper, []byte(body), 0o755); err != nil {
				t.Fatalf("write wrapper: %v", err)
			}

			aosHome := filepath.Join(tmp, "home")
			if err := os.MkdirAll(aosHome, 0o755); err != nil {
				t.Fatalf("mkdir home: %v", err)
			}
			script := filepath.Join(aosHome, "agents", "planner.sh")
			if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
				t.Fatalf("mkdir agents: %v", err)
			}
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatalf("write script: %v", err)
			}

			err := SpawnWrapperDetached(wrapper, SpawnOpts{
				AosHome:    aosHome,
				ScheduleID: tc.scheduleID,
				AgentID:    "planner",
				ScriptPath: script,
				RunID:      "run-xyz",
				Trigger:    tc.trigger,
			})
			if err != nil {
				t.Fatalf("spawn: %v", err)
			}

			deadline := time.Now().Add(2 * time.Second)
			var lines []string
			for time.Now().Before(deadline) {
				f, err := os.Open(log)
				if err == nil {
					s := bufio.NewScanner(f)
					lines = lines[:0]
					for s.Scan() {
						lines = append(lines, s.Text())
					}
					f.Close()
					if len(lines) >= 6 {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
			if len(lines) < 6 {
				t.Fatalf("wrapper log incomplete after wait: %v", lines)
			}
			expectedTrigger := tc.trigger
			if expectedTrigger == "" {
				expectedTrigger = "schedule"
			}
			want := []string{aosHome, tc.scheduleID, "planner", script, "run-xyz", "trigger=" + expectedTrigger}
			for i, w := range want {
				if lines[i] != w {
					t.Errorf("line %d = %q, want %q", i, lines[i], w)
				}
			}
		})
	}
}
