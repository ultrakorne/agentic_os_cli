package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeRunMeta(t *testing.T, dir, id, jobID, startedAt, status string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := fmt.Sprintf(`{
  "id": %q,
  "jobId": %q,
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

func TestReadRuns_emptyDirIsNoError(t *testing.T) {
	dir := t.TempDir()
	runs, err := ReadRuns(dir, "", 100)
	if err != nil {
		t.Fatalf("ReadRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected zero runs, got %d", len(runs))
	}
}

func TestReadRuns_missingDirIsNoError(t *testing.T) {
	// dir that does not exist
	runs, err := ReadRuns(filepath.Join(t.TempDir(), "nope"), "", 0)
	if err != nil {
		t.Fatalf("ReadRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected empty, got %v", runs)
	}
}

func TestReadRuns_sortsDescendingAndFiltersAndLimits(t *testing.T) {
	dir := t.TempDir()
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "success")
	writeRunMeta(t, dir, "r2", "pong", "2026-05-16T13:05:00.000Z", "success")
	writeRunMeta(t, dir, "r3", "ping", "2026-05-16T13:10:00.000Z", "running")
	// non-json sibling should be ignored
	if err := os.WriteFile(filepath.Join(dir, "r1.out"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write out: %v", err)
	}
	// malformed file should be skipped
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}

	all, err := ReadRuns(dir, "", 0)
	if err != nil {
		t.Fatalf("ReadRuns all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	// descending by startedAt: r3, r2, r1
	wantOrder := []string{"r3", "r2", "r1"}
	for i, w := range wantOrder {
		if all[i].ID != w {
			t.Errorf("order[%d] = %s, want %s", i, all[i].ID, w)
		}
	}

	filtered, err := ReadRuns(dir, "ping", 0)
	if err != nil {
		t.Fatalf("ReadRuns ping: %v", err)
	}
	if len(filtered) != 2 || filtered[0].ID != "r3" || filtered[1].ID != "r1" {
		t.Errorf("ping filter result wrong: %+v", filtered)
	}

	limited, err := ReadRuns(dir, "", 1)
	if err != nil {
		t.Fatalf("ReadRuns limit: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "r3" {
		t.Errorf("limit result wrong: %+v", limited)
	}
}

func TestReadRuns_skipsRecordsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	// id absent
	if err := os.WriteFile(filepath.Join(dir, "x.json"),
		[]byte(`{"jobId":"a","startedAt":"2026-01-01T00:00:00Z","status":"success"}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	// jobId absent
	if err := os.WriteFile(filepath.Join(dir, "y.json"),
		[]byte(`{"id":"y","startedAt":"2026-01-01T00:00:00Z","status":"success"}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	// startedAt absent
	if err := os.WriteFile(filepath.Join(dir, "z.json"),
		[]byte(`{"id":"z","jobId":"a","status":"success"}`),
		0o644); err != nil {
		t.Fatal(err)
	}

	runs, err := ReadRuns(dir, "", 0)
	if err != nil {
		t.Fatalf("ReadRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected zero (all incomplete), got %d: %+v", len(runs), runs)
	}
}

func TestReadRun_returnsNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadRun(dir, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var nf NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestReadRun_parsesAndFills(t *testing.T) {
	dir := t.TempDir()
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "success")
	r, err := ReadRun(dir, "r1")
	if err != nil {
		t.Fatalf("ReadRun: %v", err)
	}
	if r.ID != "r1" || r.JobID != "ping" || r.Status != "success" {
		t.Errorf("record wrong: %+v", r)
	}
	if r.StartedAtTime.IsZero() {
		t.Errorf("StartedAtTime should be parsed, got zero")
	}
}

func TestReadRunOutput_returnsContent(t *testing.T) {
	dir := t.TempDir()
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "success")
	if err := os.WriteFile(filepath.Join(dir, "r1.out"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRunOutput(dir, "r1")
	if err != nil {
		t.Fatalf("ReadRunOutput: %v", err)
	}
	if string(got) != "hello\n" {
		t.Errorf("got %q, want %q", got, "hello\n")
	}
}

func TestReadRunOutput_missingFileIsNilNotError(t *testing.T) {
	dir := t.TempDir()
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "running")
	got, err := ReadRunOutput(dir, "r1")
	if err != nil {
		t.Fatalf("ReadRunOutput: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing .out, got %q", got)
	}
}

func TestReadRunOutput_missingRunIsNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadRunOutput(dir, "missing")
	var nf NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}
