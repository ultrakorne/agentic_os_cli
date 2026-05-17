package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// newFileStore builds a FileRunStore rooted under a freshly-created TempDir
// "home/runs". Returns the store plus the runs dir for setups that need to
// pre-populate files directly.
func newFileStore(t *testing.T) (*FileRunStore, string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	return NewFileRunStore(home), dir
}

func TestFormatRunTimestamp_isMillisecondUTC(t *testing.T) {
	// Pin the format: ms precision, trailing Z, UTC. wrapper.sh writes the
	// same shape via Python isoformat(timespec="milliseconds").
	got := FormatRunTimestamp(time.Date(2026, 5, 16, 13, 9, 37, int(72*time.Millisecond), time.UTC))
	want := "2026-05-16T13:09:37.072Z"
	if got != want {
		t.Errorf("FormatRunTimestamp = %q, want %q", got, want)
	}
	// Zero-subsecond inputs must NOT drop the fractional component — that
	// was the bug the canonical format is here to prevent.
	if got := FormatRunTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); !strings.HasSuffix(got, ".000Z") {
		t.Errorf("zero-subsecond timestamp = %q, want .000Z suffix", got)
	}
}

func TestFileRunStore_ListEmpty(t *testing.T) {
	s, _ := newFileStore(t)
	runs, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected zero, got %d", len(runs))
	}
}

func TestFileRunStore_ListMissingDirIsNoError(t *testing.T) {
	s := NewFileRunStore(filepath.Join(t.TempDir(), "absent-home"))
	runs, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("List on missing dir: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected empty, got %v", runs)
	}
}

func TestFileRunStore_ListSortsDescAndFiltersAndLimits(t *testing.T) {
	s, dir := newFileStore(t)
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "success")
	writeRunMeta(t, dir, "r2", "pong", "2026-05-16T13:05:00.000Z", "success")
	writeRunMeta(t, dir, "r3", "ping", "2026-05-16T13:10:00.000Z", "running")
	// non-json sibling and malformed file are skipped silently.
	if err := os.WriteFile(filepath.Join(dir, "r1.out"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not"), 0o644); err != nil {
		t.Fatal(err)
	}

	all, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	for i, w := range []string{"r3", "r2", "r1"} {
		if all[i].ID != w {
			t.Errorf("order[%d] = %s, want %s", i, all[i].ID, w)
		}
	}

	ping, err := s.List(Filter{AgentID: "ping"})
	if err != nil {
		t.Fatalf("List ping: %v", err)
	}
	if len(ping) != 2 || ping[0].ID != "r3" || ping[1].ID != "r1" {
		t.Errorf("filter result wrong: %+v", ping)
	}
}

// TestFileRunStore_ListSortsMixedSubsecondPrecision exercises the
// regression the canonical timestamp format prevents. Pre-store, three
// writers produced timestamps with different subsecond shapes within the
// same second; lex-sorting them inverted the chronological order. After
// the store normalizes its own writes the same-second collision still
// shows up when reading historical records, so the sort path (which
// parses StartedAtTime) must order correctly regardless.
func TestFileRunStore_ListSortsMixedSubsecondPrecision(t *testing.T) {
	s, dir := newFileStore(t)
	// Same wall second; three historical shapes.
	writeRunMeta(t, dir, "wrapper", "ping", "2026-05-16T13:00:00.123Z", "success") // wrapper.sh ms
	writeRunMeta(t, dir, "aosrun", "ping", "2026-05-16T13:00:00.000Z", "success")  // aos run isoMillisUTC
	writeRunMeta(t, dir, "miss", "ping", "2026-05-16T13:00:00Z", "missed")         // old RFC3339Nano on zero-subsecond

	all, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Newest first: 13:00:00.123 > 13:00:00.000 == 13:00:00 (tie between
	// the latter two). Critically the .123 record must NOT sort below the
	// zero-subsecond ones — the bug pre-fix was the inverse.
	if all[0].ID != "wrapper" {
		t.Errorf("first = %s, want wrapper (.123Z is the newest)", all[0].ID)
	}
}

func TestFileRunStore_GetReturnsNotFound(t *testing.T) {
	s, _ := newFileStore(t)
	_, err := s.Get("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var nf NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestFileRunStore_GetParsesStartedAtTime(t *testing.T) {
	s, dir := newFileStore(t)
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "success")
	r, err := s.Get("r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if r.StartedAtTime.IsZero() {
		t.Error("StartedAtTime not populated")
	}
}

func TestFileRunStore_Output(t *testing.T) {
	s, dir := newFileStore(t)
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "success")
	if err := os.WriteFile(filepath.Join(dir, "r1.out"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.Output("r1")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if string(got) != "hello\n" {
		t.Errorf("got %q, want hello", got)
	}
}

func TestFileRunStore_OutputMissingFileIsNilNotError(t *testing.T) {
	s, dir := newFileStore(t)
	writeRunMeta(t, dir, "r1", "ping", "2026-05-16T13:00:00.000Z", "running")
	got, err := s.Output("r1")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing .out, got %q", got)
	}
}

func TestFileRunStore_OutputMissingRunIsNotFound(t *testing.T) {
	s, _ := newFileStore(t)
	_, err := s.Output("missing")
	var nf NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestFileRunStore_EstimateDuration(t *testing.T) {
	s, dir := newFileStore(t)
	for i := 1; i <= 11; i++ {
		started := fmt.Sprintf("2026-05-16T13:%02d:00.000Z", i)
		ended := fmt.Sprintf("2026-05-16T13:%02d:%02d.000Z", i, i)
		writeFinishedRunMeta(t, dir, fmt.Sprintf("r%d", i), "ping", started, ended)
	}
	// Other-agent record must not pull the average around.
	writeFinishedRunMeta(t, dir, "other", "pong", "2026-05-16T14:00:00.000Z", "2026-05-16T14:00:30.000Z")
	writeRunMeta(t, dir, "running", "ping", "2026-05-16T14:01:00.000Z", "running")

	got, ok, err := s.EstimateDuration("ping", 10)
	if err != nil {
		t.Fatalf("EstimateDuration: %v", err)
	}
	if !ok {
		t.Fatal("expected estimate")
	}
	if got != 6500*time.Millisecond {
		t.Errorf("estimate = %s, want 6.5s", got)
	}
}

func TestFileRunStore_EstimateDurationNoSamples(t *testing.T) {
	s, dir := newFileStore(t)
	writeRunMeta(t, dir, "running", "ping", "2026-05-16T14:01:00.000Z", "running")
	got, ok, err := s.EstimateDuration("ping", 10)
	if err != nil {
		t.Fatalf("EstimateDuration: %v", err)
	}
	if ok || got != 0 {
		t.Errorf("estimate = %s, %v; want 0, false", got, ok)
	}
}

func TestFileRunStore_NewIDShape(t *testing.T) {
	s, _ := newFileStore(t)
	id := s.NewID()
	// <unix-millis>-<rand4>: 13-digit unix millis followed by '-' and four
	// hex chars. The exact prefix shifts with time, so we just shape-check.
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		t.Fatalf("id %q does not split on '-' into 2 parts", id)
	}
	if len(parts[0]) < 12 {
		t.Errorf("id %q millis prefix too short", id)
	}
	if len(parts[1]) != 4 {
		t.Errorf("id %q random suffix should be 4 hex chars", id)
	}
}

func TestFileRunStore_ReplaceMissedFirstWrite(t *testing.T) {
	s, dir := newFileStore(t)
	at := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	run, wrote, err := s.ReplaceMissed("ping", at)
	if err != nil {
		t.Fatalf("ReplaceMissed: %v", err)
	}
	if !wrote {
		t.Error("wrote=false on first write")
	}
	if run.Status != StatusMissed || run.AgentID != "ping" {
		t.Errorf("returned run wrong: %+v", run)
	}
	// File on disk uses the canonical millisecond timestamp.
	got, err := s.Get(run.ID)
	if err != nil {
		t.Fatalf("Get(%s): %v", run.ID, err)
	}
	if !strings.HasSuffix(got.StartedAt, ".000Z") {
		t.Errorf("startedAt = %q, want ms-precision .000Z suffix", got.StartedAt)
	}
	// One file on disk.
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestFileRunStore_ReplaceMissedIsIdempotent(t *testing.T) {
	s, _ := newFileStore(t)
	at := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	if _, wrote, err := s.ReplaceMissed("ping", at); err != nil || !wrote {
		t.Fatalf("first: wrote=%v err=%v", wrote, err)
	}
	if _, wrote, err := s.ReplaceMissed("ping", at); err != nil {
		t.Fatalf("second: %v", err)
	} else if wrote {
		t.Error("second ReplaceMissed wrote=true; want idempotent skip")
	}
}

func TestFileRunStore_ReplaceMissedReplacesPriorSlot(t *testing.T) {
	s, dir := newFileStore(t)
	first := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	second := time.Date(2026, 5, 16, 13, 0, 0, 0, time.UTC)

	if _, _, err := s.ReplaceMissed("ping", first); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, wrote, err := s.ReplaceMissed("ping", second); err != nil || !wrote {
		t.Fatalf("second: wrote=%v err=%v", wrote, err)
	}
	// Only one file remains.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file after replacement, got %d", len(entries))
	}
	if entries[0].Name() != MissedRunID("ping", second)+".json" {
		t.Errorf("remaining file %q, want %q", entries[0].Name(), MissedRunID("ping", second)+".json")
	}
}

func TestFileRunStore_ReplaceMissedWithBatchIndex(t *testing.T) {
	// ReplaceMissedWith uses the externally-supplied index instead of
	// loading the runs dir each call. The map must be updated on a
	// successful write so a sequence of calls sees its own writes.
	s, _ := newFileStore(t)
	idx := map[string][]string{}
	slots := []time.Time{
		time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 16, 13, 0, 0, 0, time.UTC),
	}
	for _, slot := range slots {
		if _, wrote, err := s.ReplaceMissedWith(idx, "ping", slot); err != nil || !wrote {
			t.Fatalf("slot %s: wrote=%v err=%v", slot, wrote, err)
		}
	}
	// After two writes the map should reflect only the latest miss id.
	if got := idx["ping"]; len(got) != 1 || got[0] != MissedRunID("ping", slots[1]) {
		t.Errorf("idx after batch = %v, want [%s]", got, MissedRunID("ping", slots[1]))
	}
}

func TestFileRunStore_SweepMissingDirIsNoop(t *testing.T) {
	s := NewFileRunStore(filepath.Join(t.TempDir(), "absent"))
	res, err := s.Sweep(100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Before != 0 || res.Deleted != 0 || res.Files != 0 {
		t.Errorf("expected zero result, got %+v", res)
	}
}

func TestFileRunStore_SweepNonPositiveCapIsNoop(t *testing.T) {
	s, dir := newFileStore(t)
	writeRunWithMtime(t, dir, "r1", time.Unix(1, 0), true)
	writeRunWithMtime(t, dir, "r2", time.Unix(2, 0), false)

	res, err := s.Sweep(0)
	if err != nil {
		t.Fatalf("Sweep cap=0: %v", err)
	}
	if res.Deleted != 0 {
		t.Errorf("expected no deletion, got %+v", res)
	}
}

func TestFileRunStore_SweepDropsOldestPairs(t *testing.T) {
	s, dir := newFileStore(t)
	// r1 oldest, r5 newest; cap=3 drops r1+r2 with their .out siblings.
	writeRunWithMtime(t, dir, "r1", time.Unix(100, 0), true)
	writeRunWithMtime(t, dir, "r2", time.Unix(200, 0), false)
	writeRunWithMtime(t, dir, "r3", time.Unix(300, 0), true)
	writeRunWithMtime(t, dir, "r4", time.Unix(400, 0), false)
	writeRunWithMtime(t, dir, "r5", time.Unix(500, 0), true)

	res, err := s.Sweep(3)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Before != 5 || res.Deleted != 2 || res.Files != 3 {
		t.Errorf("res = %+v, want {5 2 3}", res)
	}
	got := listNames(t, dir)
	want := []string{"r3.json", "r3.out", "r4.json", "r5.json", "r5.out"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("after sweep got %v, want %v", got, want)
	}
}

func TestFileRunStore_SweepIgnoresUnrelatedFiles(t *testing.T) {
	s, dir := newFileStore(t)
	writeRunWithMtime(t, dir, "r1", time.Unix(1, 0), false)
	writeRunWithMtime(t, dir, "r2", time.Unix(2, 0), false)
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := s.Sweep(1)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Before != 2 || res.Deleted != 1 {
		t.Errorf("res = %+v, want {2 1 _}", res)
	}
	for _, name := range []string{"stray.txt", "subdir"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
}

// writeRunWithMtime is a Sweep-test helper — same shape as writeRunMeta but
// stamps the file's mtime so the oldest-first ordering is deterministic.
func writeRunWithMtime(t *testing.T, dir, id string, mtime time.Time, hasOut bool) {
	t.Helper()
	jsonPath := filepath.Join(dir, id+".json")
	if err := os.WriteFile(jsonPath, []byte(`{"id":"`+id+`"}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	if err := os.Chtimes(jsonPath, mtime, mtime); err != nil {
		t.Fatalf("chtimes json: %v", err)
	}
	if hasOut {
		outPath := filepath.Join(dir, id+".out")
		if err := os.WriteFile(outPath, []byte("hi"), 0o644); err != nil {
			t.Fatalf("write out: %v", err)
		}
		if err := os.Chtimes(outPath, mtime, mtime); err != nil {
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
