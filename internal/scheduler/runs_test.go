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
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

func writeFinishedRunMeta(t *testing.T, dir, id, jobID, startedAt, endedAt string) {
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
  "status": "success",
  "output": "",
  "error": null,
  "exitCode": 0,
  "outputPath": %q
}`, id, jobID, startedAt, endedAt, id+".out")
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
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
