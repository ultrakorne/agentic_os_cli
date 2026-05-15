package scheduler

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func readMissFile(t *testing.T, path string) MissRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var r MissRecord
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return r
}

func listJSON(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read %s: %v", dir, err)
	}
	out := []string{}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func TestMissFileNameIsStableAndPortable(t *testing.T) {
	exp := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	got := MissFileName("ping", exp)
	want := "ping__2026-05-15T09-00-00Z.json"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Same inputs → same name (idempotency contract).
	again := MissFileName("ping", exp)
	if again != got {
		t.Fatalf("non-deterministic filename: %q vs %q", got, again)
	}
}

func TestWriteMissesCreatesFilesAndRecord(t *testing.T) {
	dir := t.TempDir()
	expA := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	expB := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	res, err := WriteMisses(dir, []MissedRun{
		{AgentID: "ping", ExpectedAt: expA},
		{AgentID: "deploy", ExpectedAt: expB},
	})
	if err != nil {
		t.Fatalf("WriteMisses: %v", err)
	}
	if res.Wrote != 2 || res.Deleted != 0 {
		t.Fatalf("counts: wrote=%d deleted=%d", res.Wrote, res.Deleted)
	}
	names := listJSON(t, dir)
	if len(names) != 2 {
		t.Fatalf("file count: got %v want 2", names)
	}
	rec := readMissFile(t, filepath.Join(dir, MissFileName("ping", expA)))
	if rec.AgentID != "ping" || rec.ExpectedAt != "2026-05-15T09:00:00Z" {
		t.Fatalf("ping record: %+v", rec)
	}
}

func TestWriteMissesDeletesOrphansAndSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	expA := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	expB := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)

	// Initial population: two misses.
	if _, err := WriteMisses(dir, []MissedRun{
		{AgentID: "ping", ExpectedAt: expA},
		{AgentID: "deploy", ExpectedAt: expB},
	}); err != nil {
		t.Fatalf("first: %v", err)
	}

	// Snapshot mtime of the ping file so we can prove the second rebuild does
	// NOT rewrite it (unchanged content → no fs.watch noise).
	pingPath := filepath.Join(dir, MissFileName("ping", expA))
	beforeStat, err := os.Stat(pingPath)
	if err != nil {
		t.Fatalf("stat ping: %v", err)
	}

	// Sleep just long enough that a rewrite would produce a distinguishable
	// mtime on filesystems with second-resolution timestamps.
	time.Sleep(1100 * time.Millisecond)

	// Second rebuild: ping is still missed, deploy has been covered.
	res, err := WriteMisses(dir, []MissedRun{
		{AgentID: "ping", ExpectedAt: expA},
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res.Wrote != 0 {
		t.Fatalf("expected 0 rewrites for unchanged content, got %d", res.Wrote)
	}
	if res.Deleted != 1 {
		t.Fatalf("expected 1 orphan deletion, got %d", res.Deleted)
	}
	if names := listJSON(t, dir); len(names) != 1 || names[0] != MissFileName("ping", expA) {
		t.Fatalf("after rebuild: %v", names)
	}
	afterStat, err := os.Stat(pingPath)
	if err != nil {
		t.Fatalf("stat ping after: %v", err)
	}
	if !afterStat.ModTime().Equal(beforeStat.ModTime()) {
		t.Fatalf("unchanged file was rewritten: %v -> %v", beforeStat.ModTime(), afterStat.ModTime())
	}
}

func TestWriteMissesEmptySetClearsDir(t *testing.T) {
	dir := t.TempDir()
	exp := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	if _, err := WriteMisses(dir, []MissedRun{{AgentID: "ping", ExpectedAt: exp}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := WriteMisses(dir, nil)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if res.Wrote != 0 || res.Deleted != 1 {
		t.Fatalf("clear counts: %+v", res)
	}
	if names := listJSON(t, dir); len(names) != 0 {
		t.Fatalf("expected empty, got %v", names)
	}
}

func TestWriteMissesIgnoresNonJSONAndDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed text: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("seed subdir: %v", err)
	}
	if _, err := WriteMisses(dir, nil); err != nil {
		t.Fatalf("WriteMisses: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.txt")); err != nil {
		t.Fatalf("README.txt was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "subdir")); err != nil {
		t.Fatalf("subdir was removed: %v", err)
	}
}
