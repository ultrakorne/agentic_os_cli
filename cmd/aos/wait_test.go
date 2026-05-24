package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// writeRun is a local helper mirroring scheduler/runs_test writeRunMeta, sized
// for the few fields finalizeRun and the wait model actually consume.
func writeRun(t *testing.T, dir, id, status string, exitCode *int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	exitJSON := "null"
	if exitCode != nil {
		exitJSON = strconv.Itoa(*exitCode)
	}
	body := `{
  "id": "` + id + `",
  "agentId": "planner",
  "trigger": "manual",
  "startedAt": "2026-05-16T13:09:37.072Z",
  "endedAt": "2026-05-16T13:09:39.103Z",
  "status": "` + status + `",
  "output": "",
  "error": null,
  "exitCode": ` + exitJSON + `,
  "outputPath": "` + id + `.out"
}`
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

// storeAt builds a FileRunStore over the given runs dir. The wait flow's
// tests all parametrize a temp directory; the store wraps it the same way
// the wait command does at runtime.
func storeAt(dir string) *scheduler.FileRunStore {
	return scheduler.NewFileRunStoreFromDir(dir)
}

// TestFinalizeRun_successWritesOutput: a successful run prints its .out to
// the supplied writer and returns nil.
func TestFinalizeRun_successWritesOutput(t *testing.T) {
	dir := t.TempDir()
	exit := 0
	writeRun(t, dir, "r-1", "success", &exit)
	if err := os.WriteFile(filepath.Join(dir, "r-1.out"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write .out: %v", err)
	}
	store := storeAt(dir)
	run, err := store.Get("r-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var buf bytes.Buffer
	if err := finalizeRun(store, "r-1", run, &buf); err != nil {
		t.Fatalf("finalizeRun: %v", err)
	}
	if buf.String() != "hello\n" {
		t.Errorf("stdout = %q, want %q", buf.String(), "hello\n")
	}
}

// TestFinalizeRun_failurePrintsOutputThenErrors: when the agent exited
// non-zero, the .out must still be written (output-first) and finalizeRun
// must return an error so aos run --wait exits non-zero.
func TestFinalizeRun_failurePrintsOutputThenErrors(t *testing.T) {
	dir := t.TempDir()
	exit := 2
	writeRun(t, dir, "r-1", "error", &exit)
	if err := os.WriteFile(filepath.Join(dir, "r-1.out"), []byte("boom\n"), 0o644); err != nil {
		t.Fatalf("write .out: %v", err)
	}
	store := storeAt(dir)
	run, err := store.Get("r-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var buf bytes.Buffer
	err = finalizeRun(store, "r-1", run, &buf)
	if err == nil {
		t.Fatalf("finalizeRun returned nil, want error")
	}
	if !strings.Contains(err.Error(), "exited with code 2") {
		t.Errorf("err = %v, want contains 'exited with code 2'", err)
	}
	if buf.String() != "boom\n" {
		t.Errorf("stdout = %q, want %q (output must come before the error)", buf.String(), "boom\n")
	}
}

// TestFinalizeRun_successNonzeroExitErrors: a "success" status with a
// non-zero exit code (rare but possible if a script reports success but
// the wrapper records the exit) still surfaces as an error.
func TestFinalizeRun_successNonzeroExitErrors(t *testing.T) {
	dir := t.TempDir()
	exit := 7
	writeRun(t, dir, "r-1", "success", &exit)
	store := storeAt(dir)
	run, err := store.Get("r-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var buf bytes.Buffer
	err = finalizeRun(store, "r-1", run, &buf)
	if err == nil {
		t.Fatalf("expected non-nil error for exit 7")
	}
	if !strings.Contains(err.Error(), "code 7") {
		t.Errorf("err = %v, want contains 'code 7'", err)
	}
}

// TestFinalizeRun_missingOutFile: a finished run without a .out file (script
// printed nothing) is not an error — finalizeRun returns nil and writes
// nothing to stdout.
func TestFinalizeRun_missingOutFile(t *testing.T) {
	dir := t.TempDir()
	exit := 0
	writeRun(t, dir, "r-1", "success", &exit)
	store := storeAt(dir)
	run, err := store.Get("r-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var buf bytes.Buffer
	if err := finalizeRun(store, "r-1", run, &buf); err != nil {
		t.Fatalf("finalizeRun: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("stdout = %q, want empty", buf.String())
	}
}

// TestWaitModel_ctrlCCancels: feeding KeyMsg("ctrl+c") into the model marks
// it canceled and asks the program to quit. This guards the contract that
// the wait flow's caller can tell a user cancel apart from completion.
func TestWaitModel_ctrlCCancels(t *testing.T) {
	dir := t.TempDir()
	m := newWaitModel(context.Background(), storeAt(dir), "r-1", "planner", time.Now(), -1)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	wm, ok := updated.(waitModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !wm.canceled {
		t.Error("canceled = false, want true after ctrl+c")
	}
	if cmd == nil {
		t.Error("Update returned nil cmd, want tea.Quit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("Update cmd produced %T, want tea.QuitMsg", cmd())
	}
}

// TestWaitModel_doneMsgSetsFinal: a successful waitDoneMsg stores the run
// record and asks the program to quit.
func TestWaitModel_doneMsgSetsFinal(t *testing.T) {
	m := newWaitModel(context.Background(), storeAt(t.TempDir()), "r-1", "planner", time.Now(), -1)
	exit := 0
	run := scheduler.Run{
		ID:       "r-1",
		AgentID:    "planner",
		Status:   scheduler.StatusSuccess,
		ExitCode: &exit,
	}
	updated, cmd := m.Update(waitDoneMsg{run: run})
	wm := updated.(waitModel)
	if wm.final == nil || wm.final.ID != "r-1" {
		t.Errorf("final = %+v, want id r-1", wm.final)
	}
	if wm.canceled {
		t.Error("canceled = true, want false on normal completion")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("cmd produced %T, want QuitMsg", cmd())
	}
}

// TestWaitModel_doneMsgWithCancelErrSetsCanceled: the polling goroutine
// returns ErrWaitCanceled when ctx is canceled; the model must translate
// that to its own canceled flag.
func TestWaitModel_doneMsgWithCancelErrSetsCanceled(t *testing.T) {
	m := newWaitModel(context.Background(), storeAt(t.TempDir()), "r-1", "planner", time.Now(), -1)
	updated, _ := m.Update(waitDoneMsg{err: scheduler.ErrWaitCanceled})
	wm := updated.(waitModel)
	if !wm.canceled {
		t.Error("canceled = false, want true when wait returned ErrWaitCanceled")
	}
	if wm.err != nil {
		t.Errorf("err = %v, want nil (canceled is a distinct flag, not a bubbled error)", wm.err)
	}
}

// TestWaitModel_doneMsgWithRealErrIsKept: a non-cancel error from the
// poller is forwarded so the caller can return it from waitFlow.
func TestWaitModel_doneMsgWithRealErrIsKept(t *testing.T) {
	m := newWaitModel(context.Background(), storeAt(t.TempDir()), "r-1", "planner", time.Now(), -1)
	real := errors.New("disk on fire")
	updated, _ := m.Update(waitDoneMsg{err: real})
	wm := updated.(waitModel)
	if wm.err != real {
		t.Errorf("err = %v, want %v", wm.err, real)
	}
	if wm.canceled {
		t.Error("canceled = true, want false on non-cancel error")
	}
}

// TestRunCmd_helpMentionsWait makes sure the public docs string reflects the
// --wait flag so users running `aos run --help` see it.
func TestRunCmd_helpMentionsWait(t *testing.T) {
	if !strings.Contains(runCmd.Long, "--wait") {
		t.Error("runCmd.Long does not mention --wait")
	}
	if f := runCmd.Flags().Lookup("wait"); f == nil {
		t.Fatal("--wait flag not registered")
	}
}
