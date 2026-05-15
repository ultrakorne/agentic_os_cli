package scheduler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// missesDirName is the on-disk directory (under aos_home) that holds the
// current set of outstanding missed-run records — one JSON file per miss.
const missesDirName = "misses"

// MissesDir returns <aosHome>/misses.
func MissesDir(aosHome string) string {
	return filepath.Join(aosHome, missesDirName)
}

// MissRecord is the wire shape persisted to each missed-run file. Kept small
// on purpose — the dashboard re-derives anything else (title, schedule) from
// the agent's meta sidecar.
type MissRecord struct {
	AgentID    string `json:"agentId"`
	ExpectedAt string `json:"expectedAt"`
}

// MissFileName returns a stable filename for (agentID, expectedAt). Colons in
// the RFC3339 timestamp are replaced with '-' so the name is portable across
// filesystems that disallow ':'. Idempotency depends on this function being
// deterministic — never include observedAt/timezone here.
func MissFileName(agentID string, expectedAt time.Time) string {
	ts := expectedAt.UTC().Format(time.RFC3339)
	ts = strings.ReplaceAll(ts, ":", "-")
	return fmt.Sprintf("%s__%s.json", agentID, ts)
}

// WriteMissesResult counts what changed during the rebuild. Useful for the
// tick.log line and for tests.
type WriteMissesResult struct {
	Wrote   int
	Deleted int
}

// WriteMisses rebuilds missesDir so it contains exactly one JSON file per
// entry in `misses` and nothing else. Each write is atomic (temp+rename).
// Files whose contents already match are left untouched so the dashboard's
// fs.watch only fires on actual changes.
func WriteMisses(missesDir string, misses []MissedRun) (WriteMissesResult, error) {
	res := WriteMissesResult{}
	if err := os.MkdirAll(missesDir, 0o755); err != nil {
		return res, err
	}

	desired := make(map[string]MissRecord, len(misses))
	for _, m := range misses {
		name := MissFileName(m.AgentID, m.ExpectedAt)
		desired[name] = MissRecord{
			AgentID:    m.AgentID,
			ExpectedAt: m.ExpectedAt.UTC().Format(time.RFC3339),
		}
	}

	entries, err := os.ReadDir(missesDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return res, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if _, keep := desired[e.Name()]; keep {
			continue
		}
		if rmErr := os.Remove(filepath.Join(missesDir, e.Name())); rmErr == nil {
			res.Deleted++
		} else if !errors.Is(rmErr, os.ErrNotExist) {
			return res, rmErr
		}
	}

	names := make([]string, 0, len(desired))
	for k := range desired {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		full := filepath.Join(missesDir, name)
		body, mErr := json.Marshal(desired[name])
		if mErr != nil {
			return res, mErr
		}
		if existing, rErr := os.ReadFile(full); rErr == nil && bytes.Equal(existing, body) {
			continue
		}
		tmp := full + ".tmp"
		if wErr := os.WriteFile(tmp, body, 0o644); wErr != nil {
			return res, wErr
		}
		if rErr := os.Rename(tmp, full); rErr != nil {
			_ = os.Remove(tmp)
			return res, rErr
		}
		res.Wrote++
	}
	return res, nil
}

// SyncMissesDir is the convenience helper used by both `aos tick` and
// `aos refresh`: load runs from <aosHome>/runs, detect misses against the
// given agents, and rebuild <aosHome>/misses to match. Returns the list of
// detected misses (so callers can include the count in their summary line).
//
// Failure to load runs degrades to "no runs" — a missing/empty runs dir is a
// fresh install, not an error. Write failures are returned.
func SyncMissesDir(aosHome string, agents []Agent, now time.Time) ([]MissedRun, error) {
	runs, _ := LoadRuns(filepath.Join(aosHome, "runs"))
	missed := DetectMissed(agents, runs, DetectOpts{Now: now})
	if _, err := WriteMisses(MissesDir(aosHome), missed); err != nil {
		return missed, fmt.Errorf("write misses: %w", err)
	}
	return missed, nil
}
