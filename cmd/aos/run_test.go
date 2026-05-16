package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJobRunStub_shape(t *testing.T) {
	stub := jobRunStub("r-1", "planner", "2026-01-01T00:00:00.000Z")
	buf, err := json.Marshal(stub)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]any{
		"id":         "r-1",
		"jobId":      "planner",
		"scheduleId": nil,
		"trigger":    "manual",
		"startedAt":  "2026-01-01T00:00:00.000Z",
		"endedAt":    nil,
		"status":     "running",
		"output":     "",
		"error":      nil,
		"exitCode":   nil,
		"outputPath": "r-1.out",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("stub[%q] = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected keys: got=%v want=%v", got, want)
	}
}

func TestIsoMillisUTC_format(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339Nano, "2026-05-16T14:30:25.123456789Z")
	got := isoMillisUTC(ts)
	if got != "2026-05-16T14:30:25.123Z" {
		t.Errorf("isoMillisUTC = %q, want 2026-05-16T14:30:25.123Z", got)
	}
}

func TestNewRunID_isUnique(t *testing.T) {
	a := newRunID()
	b := newRunID()
	if a == b {
		t.Fatalf("newRunID returned the same value twice: %s", a)
	}
	if !strings.Contains(a, "-") {
		t.Errorf("newRunID format unexpected: %q", a)
	}
}

// TestSpawnWrapperDetached_passesArgsAndTriggerEnv runs a fake wrapper that
// records its argv and AGENTIC_OS_TRIGGER to a log file, then asserts both.
// Locks in the cron/manual argv contract: <aos_home> '' <agent_id> <script> <run_id>.
func TestSpawnWrapperDetached_passesArgsAndTriggerEnv(t *testing.T) {
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

	if err := spawnWrapperDetached(wrapper, aosHome, "planner", script, "run-xyz"); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// The wrapper is detached; poll briefly for the log to appear.
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
	want := []string{aosHome, "", "planner", script, "run-xyz", "trigger=manual"}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}
