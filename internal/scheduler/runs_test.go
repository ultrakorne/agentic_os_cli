package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeRunMeta seeds <dir>/<id>.json with the minimal shape FileRunStore reads.
// Shared with run_store_test.go for the common setup cases.
func writeRunMeta(t *testing.T, dir, id, jobID, startedAt, status string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := fmt.Sprintf(`{
  "id": %q,
  "agentId": %q,
  "scheduleId": null,
  "trigger": "manual",
  "startedAt": %q,
  "endedAt": null,
  "status": %q,
  "output": "",
  "error": null,
  "exitCode": null,
  "outputPath": %q
}`, id, jobID, startedAt, status, id+".out")
	atomicWriteFile(t, filepath.Join(dir, id+".json"), []byte(body))
}

// atomicWriteFile mirrors the production wrapper's contract: write to a
// sibling .tmp and rename on top of the target so a concurrent reader sees
// either the old bytes or the new bytes, never a truncated mid-write. Tests
// that overwrite a run record while another goroutine is polling rely on
// this — plain os.WriteFile uses O_TRUNC and exposes a 0-byte window that
// makes WaitForRun's poller occasionally observe an empty JSON file.
func atomicWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		t.Fatalf("rename %s -> %s: %v", tmp, path, err)
	}
}

func writeFinishedRunMeta(t *testing.T, dir, id, jobID, startedAt, endedAt string) {
	t.Helper()
	writeFinishedRunMetaStatus(t, dir, id, jobID, startedAt, endedAt, "success", 0)
}

func writeFinishedRunMetaStatus(t *testing.T, dir, id, jobID, startedAt, endedAt, status string, exitCode int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := fmt.Sprintf(`{
  "id": %q,
  "agentId": %q,
  "scheduleId": null,
  "trigger": "manual",
  "startedAt": %q,
  "endedAt": %q,
  "status": %q,
  "output": "",
  "error": null,
  "exitCode": %d,
  "outputPath": %q
}`, id, jobID, startedAt, endedAt, status, exitCode, id+".out")
	atomicWriteFile(t, filepath.Join(dir, id+".json"), []byte(body))
}

// TestReadRun_returnsNotFound ensures FileRunStore.Get surfaces a typed
// NotFoundError that the wait flow can errors.As — the wait poller treats
// "not found yet" as "still spawning" and keeps trying.
func TestReadRun_returnsNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewFileRunStoreFromDir(dir)
	_, err := s.Get("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var nf NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}
