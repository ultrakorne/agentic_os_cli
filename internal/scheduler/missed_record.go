package scheduler

import (
	"fmt"
	"strings"
	"time"
)

// MissedRunID returns a stable run id for (agentID, expectedAt). The "miss-"
// prefix distinguishes these records from wrapper-generated IDs
// (`<unix>-<pid>-<rand>`) at a glance. Colons in the RFC3339 timestamp are
// replaced with '-' so the resulting filename is portable across filesystems
// that disallow ':'.
func MissedRunID(agentID string, expectedAt time.Time) string {
	ts := expectedAt.UTC().Format(time.RFC3339)
	ts = strings.ReplaceAll(ts, ":", "-")
	return "miss-" + agentID + "-" + ts
}

// RecordMissedRuns persists the latest uncovered slot per agent as a
// runs/<id>.json with status="missed". At most one miss record per agent
// exists on disk at any time: when a newer slot is detected, the previous
// miss record for that agent is deleted and replaced. The deliberate
// granularity loss (multi-slot outages collapse to one entry) is what lets
// the dashboard show "agents currently behind" as a one-row-per-agent
// banner that auto-resolves on the next real run.
//
// Returns:
//   - written: misses actually written this call (zero when every detected
//     miss already has a matching file on disk). aos tick / aos refresh
//     surface the count as their "newly recorded this tick" summary.
//   - updated: the post-write []Run. Same shape as a fresh store.Load
//     would produce — stale miss records removed, new ones appended with
//     StartedAtTime populated — so the caller can chain into a follow-up
//     pass (the stale-running sweep in aos tick) without re-reading the
//     directory.
func RecordMissedRuns(aosHome string, agents []Agent, now time.Time) ([]MissedRun, []Run, error) {
	store := NewFileRunStore(aosHome)

	// Surface a Load error rather than swallowing — if the runs dir is
	// unreadable (permission denied, etc.) we'd otherwise treat it as empty
	// and re-record misses every tick. Load handles ErrNotExist internally,
	// so a clean install with no runs/ yet still returns (nil, nil).
	runs, err := store.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load runs: %w", err)
	}
	detected := DetectMissed(agents, runs, DetectOpts{Now: now})
	if len(detected) == 0 {
		return nil, runs, nil
	}

	// Build the existing-miss-by-agent index from the in-memory slice once,
	// then drive replacement through the store's batch-friendly API. This
	// keeps the directory walk to a single pass even when many agents need
	// replacement on the same tick.
	idx := store.IndexMissedFromRuns(runs)

	var written []MissedRun
	for _, m := range detected {
		stale := append([]string(nil), idx[m.AgentID]...)
		newRun, wrote, err := store.ReplaceMissedWith(idx, m.AgentID, m.ExpectedAt)
		if err != nil {
			return written, runs, err
		}
		if !wrote {
			continue
		}
		// Mirror the disk delete in the in-memory slice so the returned
		// view reflects what a fresh Load would produce — tick's stale-running
		// sweep chains off this slice.
		for _, id := range stale {
			runs = removeRunByID(runs, id)
		}
		runs = append(runs, newRun)
		written = append(written, m)
	}
	return written, runs, nil
}

// removeRunByID returns runs with any entry matching id stripped. Order is
// not preserved (callers re-sort as needed).
func removeRunByID(runs []Run, id string) []Run {
	out := runs[:0]
	for _, r := range runs {
		if r.ID == id {
			continue
		}
		out = append(out, r)
	}
	return out
}
