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
	store := NewFileRunStoreFromDir(t.TempDir())
	a := store.NewID()
	b := store.NewID()
	if a == b {
		t.Fatalf("NewID returned the same value twice: %s", a)
	}
	if !strings.Contains(a, "-") {
		t.Errorf("NewID format unexpected: %q", a)
	}
}

// TestSpawnWrapperDetached_passesEnvContract runs a fake wrapper that
// records the run-context env vars to a log file, then asserts the contract.
// Locks in the wrapper env contract: AGENTIC_OS_DATA_DIR / AGENT_ID /
// AGENT_SCRIPT / RUN_ID / TRIGGER. Also pins the per-trigger wiring callers
// depend on (manual runs distinguishable from schedule runs).
func TestSpawnWrapperDetached_passesEnvContract(t *testing.T) {
	cases := []struct {
		name    string
		trigger string
	}{
		{"manual", "manual"},
		{"schedule explicit", "schedule"},
		{"schedule default", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			wrapper := filepath.Join(tmp, "wrapper.sh")
			log := filepath.Join(tmp, "wrapper.log")
			body := "#!/usr/bin/env bash\n" +
				"echo \"data_dir=$AGENTIC_OS_DATA_DIR\" >> \"" + log + "\"\n" +
				"echo \"agent_id=$AGENTIC_OS_AGENT_ID\" >> \"" + log + "\"\n" +
				"echo \"script=$AGENTIC_OS_AGENT_SCRIPT\" >> \"" + log + "\"\n" +
				"echo \"run_id=$AGENTIC_OS_RUN_ID\" >> \"" + log + "\"\n" +
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
					if len(lines) >= 5 {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
			if len(lines) < 5 {
				t.Fatalf("wrapper log incomplete after wait: %v", lines)
			}
			expectedTrigger := tc.trigger
			if expectedTrigger == "" {
				expectedTrigger = "schedule"
			}
			want := []string{
				"data_dir=" + aosHome,
				"agent_id=planner",
				"script=" + script,
				"run_id=run-xyz",
				"trigger=" + expectedTrigger,
			}
			for i, w := range want {
				if lines[i] != w {
					t.Errorf("line %d = %q, want %q", i, lines[i], w)
				}
			}
		})
	}
}
