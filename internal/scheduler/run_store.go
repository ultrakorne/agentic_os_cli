package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// RunTimestampFormat is the wire format every Go writer uses for run
// timestamps. Mirrors wrapper.sh's iso_now (Python isoformat,
// timespec="milliseconds", trailing "Z"). The previous mix of RFC3339,
// RFC3339Nano, and forced-millis formats made same-second records lex-invert
// — see the smoking-gun comment that used to live in runs.go.
const RunTimestampFormat = "2006-01-02T15:04:05.000Z"

// FormatRunTimestamp formats t as the canonical run timestamp (millisecond
// UTC). Use this anywhere the Go side stamps a Run.
func FormatRunTimestamp(t time.Time) string {
	return t.UTC().Format(RunTimestampFormat)
}

// Filter selects runs in List.
type Filter struct {
	AgentID string // empty = all agents
}

// SweepResult summarises a Sweep call: pairs seen, deleted, and individual
// files unlinked (1 or 2 per pair depending on whether the .out existed).
type SweepResult struct {
	Before  int
	Deleted int
	Files   int
}

// FileRunStore is the on-disk backing for <aos_home>/runs/. Read and write
// paths share this struct so the timestamp format, atomic-write dance, and
// id-pair pairing rules live in one place.
type FileRunStore struct {
	dir   string
	clock func() time.Time

	idOnce sync.Once
	idRand *rand.Rand
}

// NewFileRunStore constructs a store rooted at <aosHome>/runs. The directory
// is not created here. Write paths (ReplaceMissedWith) MkdirAll lazily;
// read paths (List, Get, Output, Load) tolerate a missing dir and return
// empty results. The fsnotify wiring in `aos start` is the one caller that
// needs the directory to exist before it runs (watcher.Add fails on a
// missing path), so it calls os.MkdirAll on store.Dir() itself.
func NewFileRunStore(aosHome string) *FileRunStore {
	return NewFileRunStoreFromDir(filepath.Join(aosHome, "runs"))
}

// NewFileRunStoreFromDir constructs a store that treats dir as the runs
// directory directly (skipping the <aosHome>/runs join). Use this when the
// caller already holds the runs path — e.g. the wait flow's tea model.
func NewFileRunStoreFromDir(dir string) *FileRunStore {
	return &FileRunStore{dir: dir, clock: time.Now}
}

// Dir returns the runs directory path. Useful for fsnotify wiring in the
// TUI, which still needs the raw path. Prefer not to use this for I/O —
// route reads/writes through the store methods.
func (s *FileRunStore) Dir() string { return s.dir }

// Load returns every parseable Run on disk in filesystem-defined order,
// without dropping records for missing fields. DetectMissed and
// RecordMissedRuns want the full slice — drop-on-empty filtering happens in
// List for the aos-runs read path. A missing dir is not an error.
func (s *FileRunStore) Load() ([]Run, error) {
	return loadRuns(s.dir)
}

// List reads every *.json under the runs dir, skips malformed/incomplete
// records, applies the filter, and sorts newest-first. Callers that want a
// capped slice (currently `aos runs --limit`) capture len(out) for "shown
// of total" reporting and then re-slice themselves.
func (s *FileRunStore) List(f Filter) ([]Run, error) {
	all, err := loadRuns(s.dir)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if r.ID == "" || r.AgentID == "" || r.StartedAt == "" {
			continue
		}
		if f.AgentID != "" && r.AgentID != f.AgentID {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAtTime.After(out[j].StartedAtTime)
	})
	return out, nil
}

// Get reads one run by id. Returns NotFoundError when the file is absent.
func (s *FileRunStore) Get(id string) (Run, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Run{}, NotFoundError{ID: id}
		}
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if run.ID == "" {
		run.ID = id
	}
	if run.StartedAt != "" {
		if t, perr := time.Parse(time.RFC3339Nano, run.StartedAt); perr == nil {
			run.StartedAtTime = t
		}
	}
	return run, nil
}

// Output reads the captured-output bytes for runID. Resolves the actual
// filename from the run's OutputPath when set, falling back to "<id>.out".
// (nil, nil) when the run exists but the .out file is absent — running and
// silent-success runs both legitimately lack one.
func (s *FileRunStore) Output(id string) ([]byte, error) {
	run, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	name := id + ".out"
	if run.OutputPath != nil && *run.OutputPath != "" {
		name = *run.OutputPath
	}
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// EstimateDuration averages elapsed time of the newest successful runs for
// agentID, capped at sample. Only StatusSuccess records contribute — error,
// running, and missed runs are skipped so a fast-failing script doesn't drag
// the ETA below the typical successful runtime.
func (s *FileRunStore) EstimateDuration(agentID string, sample int) (time.Duration, bool, error) {
	runs, err := s.List(Filter{AgentID: agentID})
	if err != nil {
		return 0, false, err
	}
	var total time.Duration
	count := 0
	for _, r := range runs {
		if sample > 0 && count >= sample {
			break
		}
		if r.Status != StatusSuccess {
			continue
		}
		if r.EndedAt == nil || *r.EndedAt == "" {
			continue
		}
		start, err1 := time.Parse(time.RFC3339Nano, r.StartedAt)
		end, err2 := time.Parse(time.RFC3339Nano, *r.EndedAt)
		if err1 != nil || err2 != nil {
			continue
		}
		elapsed := end.Sub(start)
		if elapsed < 0 {
			continue
		}
		total += elapsed
		count++
	}
	if count == 0 {
		return 0, false, nil
	}
	return total / time.Duration(count), true, nil
}

// NewID mints a wrapper-shaped run id. Mirrors wrapper.sh's fallback format
// (<unix-millis>-<rand4>) so the spawn-time stub returned by aos run/tick
// matches the file the wrapper writes when callers pre-generate the id.
func (s *FileRunStore) NewID() string {
	s.idOnce.Do(func() {
		s.idRand = rand.New(rand.NewSource(s.clock().UnixNano()))
	})
	return fmt.Sprintf("%d-%04x", s.clock().UnixMilli(), s.idRand.Int31()&0xffff)
}

// ReplaceMissed implements the "at most one miss record per agent"
// invariant. If a previous miss record for agentID exists pointing at a
// different slot, it is unlinked before the new record is written. Atomic:
// the new record is written via temp+rename.
//
// Returns (run, wrote, err). wrote == false means the exact slot already
// had a matching file on disk — no I/O happened.
//
// Walks the runs directory to find prior miss records. For batched callers
// that already have a runs slice in hand, see ReplaceMissedWith — it
// accepts the existing-by-agent index so N replacements run in one walk
// instead of N+1.
func (s *FileRunStore) ReplaceMissed(agentID string, expectedAt time.Time) (Run, bool, error) {
	existing, err := s.IndexMissed()
	if err != nil {
		return Run{}, false, err
	}
	return s.ReplaceMissedWith(existing, agentID, expectedAt)
}

// IndexMissed returns the set of miss-record ids on disk, keyed by agent.
// Callers that perform a batch of ReplaceMissedWith calls build this once
// and pass it through to avoid re-walking the runs dir per call.
func (s *FileRunStore) IndexMissed() (map[string][]string, error) {
	return s.indexMissedFromRuns(nil)
}

// IndexMissedFromRuns is the in-memory variant — when the caller already has
// a fresh runs slice (RecordMissedRuns does), reuse it instead of touching
// disk.
func (s *FileRunStore) IndexMissedFromRuns(runs []Run) map[string][]string {
	out, _ := s.indexMissedFromRuns(runs)
	return out
}

func (s *FileRunStore) indexMissedFromRuns(runs []Run) (map[string][]string, error) {
	if runs == nil {
		all, err := loadRuns(s.dir)
		if err != nil {
			return nil, err
		}
		runs = all
	}
	out := map[string][]string{}
	for _, r := range runs {
		if r.Status == StatusMissed {
			out[r.AgentID] = append(out[r.AgentID], r.ID)
		}
	}
	return out, nil
}

// ReplaceMissedWith is the batch-friendly form of ReplaceMissed. existing
// is the agent → previous-miss-id index returned by IndexMissed /
// IndexMissedFromRuns; on a successful write the new id replaces stale
// entries for agentID inside the map so a follow-up call sees the new state.
func (s *FileRunStore) ReplaceMissedWith(existing map[string][]string, agentID string, expectedAt time.Time) (Run, bool, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Run{}, false, fmt.Errorf("mkdir runs: %w", err)
	}
	newID := MissedRunID(agentID, expectedAt)
	prior := existing[agentID]
	if slices.Contains(prior, newID) {
		return Run{}, false, nil // already on disk, idempotent
	}
	for _, id := range prior {
		if err := os.Remove(filepath.Join(s.dir, id+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Run{}, false, fmt.Errorf("remove stale miss %s: %w", id, err)
		}
	}
	run, err := s.writeMissedRun(newID, agentID, expectedAt)
	if err != nil {
		return Run{}, false, err
	}
	existing[agentID] = []string{newID}
	return run, true, nil
}

func (s *FileRunStore) writeMissedRun(id, agentID string, expectedAt time.Time) (Run, error) {
	expectedUTC := expectedAt.UTC()
	startedAt := FormatRunTimestamp(expectedUTC)
	run := Run{
		ID:            id,
		AgentID:       agentID,
		ScheduleID:    nil,
		Trigger:       "schedule",
		StartedAt:     startedAt,
		StartedAtTime: expectedUTC,
		EndedAt:       nil,
		Status:        StatusMissed,
		Output:        "",
		Error:         nil,
		ExitCode:      nil,
		OutputPath:    nil,
	}
	buf, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return Run{}, fmt.Errorf("marshal miss %s: %w", id, err)
	}
	full := filepath.Join(s.dir, id+".json")
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return Run{}, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return Run{}, fmt.Errorf("rename %s: %w", full, err)
	}
	return run, nil
}

// Sweep enforces a hard cap on the runs directory. A non-positive maxPairs
// is treated as "no cap" — Sweep is a no-op. A missing dir is not an error.
//
// Each run owns a <id>.json and optionally a paired <id>.out; the sweeper
// groups by stem so the two files are always deleted together.
func (s *FileRunStore) Sweep(maxPairs int) (SweepResult, error) {
	var res SweepResult
	if maxPairs <= 0 {
		return res, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return res, nil
		}
		return res, err
	}

	type pair struct {
		stem  string
		files []string
		mtime int64
	}
	pairs := make(map[string]*pair)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		if ext != ".json" && ext != ".out" {
			continue
		}
		stem := strings.TrimSuffix(name, ext)
		p, ok := pairs[stem]
		if !ok {
			p = &pair{stem: stem}
			pairs[stem] = p
		}
		p.files = append(p.files, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if m := info.ModTime().UnixNano(); m > p.mtime {
			p.mtime = m
		}
	}
	res.Before = len(pairs)
	if res.Before <= maxPairs {
		return res, nil
	}

	list := make([]*pair, 0, len(pairs))
	for _, p := range pairs {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].mtime != list[j].mtime {
			return list[i].mtime < list[j].mtime
		}
		// Deterministic tiebreak when the filesystem reports identical
		// mtimes (common on fast test runs and coarse-mtime filesystems).
		return list[i].stem < list[j].stem
	})
	drop := list[:len(list)-maxPairs]
	for _, p := range drop {
		for _, f := range p.files {
			if err := os.Remove(filepath.Join(s.dir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return res, err
			}
			res.Files++
		}
		res.Deleted++
	}
	return res, nil
}

// loadRuns reads every <runsDir>/*.json into a Run slice. Malformed files
// are silently skipped. Order is filesystem-defined (callers sort).
func loadRuns(runsDir string) ([]Run, error) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Run, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(runsDir, e.Name()))
		if err != nil {
			continue
		}
		var r Run
		if err := json.Unmarshal(data, &r); err != nil {
			continue
		}
		if r.StartedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, r.StartedAt); err == nil {
				r.StartedAtTime = t
			}
		}
		out = append(out, r)
	}
	return out, nil
}
