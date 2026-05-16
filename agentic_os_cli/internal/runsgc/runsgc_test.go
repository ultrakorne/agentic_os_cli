package runsgc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func writeRun(t *testing.T, dir, id string, mtime time.Time, hasOut bool) {
	t.Helper()
	json := filepath.Join(dir, id+".json")
	if err := os.WriteFile(json, []byte(`{"id":"`+id+`"}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	if err := os.Chtimes(json, mtime, mtime); err != nil {
		t.Fatalf("chtimes json: %v", err)
	}
	if hasOut {
		out := filepath.Join(dir, id+".out")
		if err := os.WriteFile(out, []byte("hi"), 0o644); err != nil {
			t.Fatalf("write out: %v", err)
		}
		if err := os.Chtimes(out, mtime, mtime); err != nil {
			t.Fatalf("chtimes out: %v", err)
		}
	}
}

func listNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestSweep_MissingDirIsNoop(t *testing.T) {
	res, err := Sweep(filepath.Join(t.TempDir(), "absent"), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Before != 0 || res.Deleted != 0 || res.Files != 0 {
		t.Errorf("expected zero result on missing dir, got %+v", res)
	}
}

func TestSweep_NonPositiveCapIsNoop(t *testing.T) {
	dir := t.TempDir()
	writeRun(t, dir, "r1", time.Unix(1, 0), true)
	writeRun(t, dir, "r2", time.Unix(2, 0), false)

	res, err := Sweep(dir, 0)
	if err != nil {
		t.Fatalf("Sweep cap=0: %v", err)
	}
	if res.Deleted != 0 || res.Files != 0 {
		t.Errorf("expected no deletion when cap<=0, got %+v", res)
	}
	if got := listNames(t, dir); len(got) != 3 { // r1.json, r1.out, r2.json
		t.Errorf("files mutated: %v", got)
	}
}

func TestSweep_UnderCapIsNoop(t *testing.T) {
	dir := t.TempDir()
	writeRun(t, dir, "r1", time.Unix(1, 0), false)
	writeRun(t, dir, "r2", time.Unix(2, 0), false)

	res, err := Sweep(dir, 5)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Before != 2 || res.Deleted != 0 {
		t.Errorf("expected no-op under cap, got %+v", res)
	}
}

func TestSweep_DropsOldestPairsTogether(t *testing.T) {
	dir := t.TempDir()
	// Five runs; r1 is oldest, r5 newest. Cap=3 means r1+r2 must go,
	// and r1's paired .out must be unlinked along with the .json.
	writeRun(t, dir, "r1", time.Unix(100, 0), true)
	writeRun(t, dir, "r2", time.Unix(200, 0), false)
	writeRun(t, dir, "r3", time.Unix(300, 0), true)
	writeRun(t, dir, "r4", time.Unix(400, 0), false)
	writeRun(t, dir, "r5", time.Unix(500, 0), true)

	res, err := Sweep(dir, 3)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Before != 5 || res.Deleted != 2 {
		t.Errorf("got %+v, want Before=5 Deleted=2", res)
	}
	if res.Files != 3 {
		t.Errorf("files unlinked = %d, want 3 (r1.json+r1.out+r2.json)", res.Files)
	}

	got := listNames(t, dir)
	want := []string{"r3.json", "r3.out", "r4.json", "r5.json", "r5.out"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("after sweep got %v, want %v", got, want)
	}
}

func TestSweep_IgnoresUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	writeRun(t, dir, "r1", time.Unix(1, 0), false)
	writeRun(t, dir, "r2", time.Unix(2, 0), false)
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Sweep(dir, 1)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Before != 2 || res.Deleted != 1 {
		t.Errorf("got %+v, want Before=2 Deleted=1", res)
	}
	// stray.txt and subdir/ must still exist
	if _, err := os.Stat(filepath.Join(dir, "stray.txt")); err != nil {
		t.Errorf("stray.txt was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "subdir")); err != nil {
		t.Errorf("subdir was removed: %v", err)
	}
}
