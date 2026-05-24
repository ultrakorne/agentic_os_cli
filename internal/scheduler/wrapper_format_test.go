package scheduler

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/ultrakorne/aos_cli/internal/resources"
)

// TestWrapperShFormat_matchesStore drives the real wrapper.sh from
// internal/resources end-to-end against a trivial script and asserts the
// resulting run record loads cleanly via FileRunStore.Get with both
// StartedAt and EndedAt matching RunTimestampFormat. This is the contract
// test that keeps the shell writer and the Go reader from drifting on the
// timestamp format — see the bug history at scheduler.RunTimestampFormat.
//
// Skipped when:
//   - GOOS is windows (bash not portable to that test runner),
//   - bash is missing from PATH,
//   - python3 is missing (wrapper.sh shells out to it for iso_now / JSON).
func TestWrapperShFormat_matchesStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapper.sh is bash; skipping on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available; skipping wrapper format test")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; skipping wrapper format test")
	}

	aosHome := t.TempDir()
	wrapperPath := filepath.Join(aosHome, "wrapper.sh")
	if err := os.WriteFile(wrapperPath, resources.WrapperSh, 0o755); err != nil {
		t.Fatalf("write wrapper.sh: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(aosHome, "runs"), 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	script := filepath.Join(aosHome, "agents", "ping.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	store := NewFileRunStore(aosHome)
	runID := store.NewID()
	cmd := exec.Command(wrapperPath)
	cmd.Env = append(os.Environ(),
		"AGENTIC_OS_DATA_DIR="+aosHome,
		"AGENTIC_OS_AGENT_ID=ping",
		"AGENTIC_OS_AGENT_SCRIPT="+script,
		"AGENTIC_OS_RUN_ID="+runID,
		"AGENTIC_OS_TRIGGER=manual",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wrapper invocation failed: %v\noutput: %s", err, out)
	}

	run, err := store.Get(runID)
	if err != nil {
		t.Fatalf("store.Get(%s): %v", runID, err)
	}

	// Shape check: millisecond UTC with trailing Z. Same regex would match
	// every Go-side writer (FormatRunTimestamp output). If wrapper.sh's
	// iso_now changes — say someone switches to RFC3339Nano via Python's
	// isoformat() default — this lights up before the sort regression bites.
	tsRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)
	if !tsRegex.MatchString(run.StartedAt) {
		t.Errorf("startedAt = %q does not match RunTimestampFormat (%s)", run.StartedAt, RunTimestampFormat)
	}
	if run.EndedAt == nil || *run.EndedAt == "" {
		t.Fatalf("endedAt missing — wrapper should have written a terminal record")
	}
	if !tsRegex.MatchString(*run.EndedAt) {
		t.Errorf("endedAt = %q does not match RunTimestampFormat (%s)", *run.EndedAt, RunTimestampFormat)
	}

	// Sanity: parse both with time.Parse(RunTimestampFormat) — the Go side
	// reads timestamps via RFC3339Nano in Get() to stay tolerant of legacy
	// data, but new writes must be tighter.
	if _, err := time.Parse(RunTimestampFormat, run.StartedAt); err != nil {
		t.Errorf("startedAt %q not parseable as RunTimestampFormat: %v", run.StartedAt, err)
	}
	if run.Status != StatusSuccess {
		t.Errorf("status = %q, want success (script exits 0)", run.Status)
	}
}
