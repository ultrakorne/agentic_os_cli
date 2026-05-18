package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// storeFor wraps a raw runs-directory path into a FileRunStore. WaitForRun
// now takes the store directly; this helper keeps each test body terse.
func storeFor(dir string) *FileRunStore {
	return NewFileRunStoreFromDir(dir)
}

// writeRunMetaWithExit writes a finished record (status from caller, optional
// exit code) so WaitForRun can observe a terminal state. Goes through
// atomicWriteFile so an overwrite (e.g. running → success) never exposes a
// 0-byte truncated window to a concurrent poller.
func writeTerminalRunMeta(t *testing.T, dir, id, status string, exitCode int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := fmt.Sprintf(`{
  "id": %q,
  "agentId": "planner",
  "scheduleId": null,
  "trigger": "manual",
  "startedAt": "2026-05-16T13:09:37.072Z",
  "endedAt": "2026-05-16T13:09:39.103Z",
  "status": %q,
  "output": "",
  "error": null,
  "exitCode": %d,
  "outputPath": %q
}`, id, status, exitCode, id+".out")
	atomicWriteFile(t, filepath.Join(dir, id+".json"), []byte(body))
}

// TestWaitForRun_toleratesMissingInitialRecord: the wrapper takes ~100ms to
// drop the first record. WaitForRun must keep polling, not error out, when
// the file does not yet exist.
func TestWaitForRun_toleratesMissingInitialRecord(t *testing.T) {
	dir := t.TempDir()
	go func() {
		time.Sleep(60 * time.Millisecond)
		writeTerminalRunMeta(t, dir, "r-1", "success", 0)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	run, err := WaitForRun(ctx, storeFor(dir), "r-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForRun: %v", err)
	}
	if run.Status != StatusSuccess {
		t.Errorf("status = %q, want success", run.Status)
	}
}

// TestWaitForRun_stopsOnSuccess: terminal "success" record returns immediately.
func TestWaitForRun_stopsOnSuccess(t *testing.T) {
	dir := t.TempDir()
	writeTerminalRunMeta(t, dir, "r-1", "success", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	run, err := WaitForRun(ctx, storeFor(dir), "r-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForRun: %v", err)
	}
	if run.Status != StatusSuccess {
		t.Errorf("status = %q, want success", run.Status)
	}
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Errorf("exitCode = %v, want 0", run.ExitCode)
	}
}

// TestWaitForRun_stopsOnError: terminal "error" record is also a stop signal.
func TestWaitForRun_stopsOnError(t *testing.T) {
	dir := t.TempDir()
	writeTerminalRunMeta(t, dir, "r-1", "error", 2)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	run, err := WaitForRun(ctx, storeFor(dir), "r-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForRun: %v", err)
	}
	if run.Status != StatusError {
		t.Errorf("status = %q, want error", run.Status)
	}
	if run.ExitCode == nil || *run.ExitCode != 2 {
		t.Errorf("exitCode = %v, want 2", run.ExitCode)
	}
}

// TestWaitForRun_keepsPollingWhileRunning: a "running" record alone is not a
// terminal state — WaitForRun must keep polling until the wrapper overwrites
// it with success/error.
func TestWaitForRun_keepsPollingWhileRunning(t *testing.T) {
	dir := t.TempDir()
	writeRunMeta(t, dir, "r-1", "planner", "2026-05-16T13:09:37.072Z", "running")
	go func() {
		time.Sleep(60 * time.Millisecond)
		writeTerminalRunMeta(t, dir, "r-1", "success", 0)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	run, err := WaitForRun(ctx, storeFor(dir), "r-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForRun: %v", err)
	}
	if run.Status != StatusSuccess {
		t.Errorf("status = %q, want success", run.Status)
	}
}

// TestWaitForRun_contextCancel: cancel before the record appears → ErrWaitCanceled.
func TestWaitForRun_contextCancel(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := WaitForRun(ctx, storeFor(dir), "never-appears", 10*time.Millisecond)
	if !errors.Is(err, ErrWaitCanceled) {
		t.Fatalf("err = %v, want ErrWaitCanceled", err)
	}
}

// TestWaitForRun_outputAvailableAfterMetadata: once the metadata is terminal,
// ReadRunOutput sees the .out written before it. Locks in the ordering the
// run --wait path depends on: read metadata, then read .out.
func TestWaitForRun_outputAvailableAfterMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r-1.out"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write .out: %v", err)
	}
	writeTerminalRunMeta(t, dir, "r-1", "success", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := WaitForRun(ctx, storeFor(dir), "r-1", 10*time.Millisecond); err != nil {
		t.Fatalf("WaitForRun: %v", err)
	}
	data, err := storeFor(dir).Output("r-1")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if string(data) != "hello\n" {
		t.Errorf("output = %q, want hello", string(data))
	}
}
