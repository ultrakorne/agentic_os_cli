package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
//   - updated: the post-write []JobRun. Same shape as a fresh LoadRuns
//     would produce — stale miss records removed, new ones appended with
//     StartedAtTime populated — so the caller can chain into a follow-up
//     pass (DetectCatchups in aos tick) without re-reading the directory.
func RecordMissedRuns(aosHome string, agents []Agent, now time.Time) ([]MissedRun, []JobRun, error) {
	runsDir := filepath.Join(aosHome, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("mkdir runs: %w", err)
	}

	// Surface a LoadRuns error rather than swallowing — if the runs dir is
	// unreadable (permission denied, etc.) we'd otherwise treat it as empty
	// and re-record misses every tick. LoadRuns already handles ErrNotExist
	// internally, so a clean install with no runs/ yet still returns (nil, nil).
	runs, err := LoadRuns(runsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("load runs: %w", err)
	}
	detected := DetectMissed(agents, runs, DetectOpts{Now: now})
	if len(detected) == 0 {
		return nil, runs, nil
	}

	// Index existing miss records by agent so we can identify both "already
	// recorded this exact slot" (skip) and "stale miss for a different slot"
	// (delete before writing the new one).
	existingByAgent := map[string][]string{}
	for _, r := range runs {
		if r.Status != StatusMissed {
			continue
		}
		existingByAgent[r.JobID] = append(existingByAgent[r.JobID], r.ID)
	}

	var written []MissedRun
	for _, m := range detected {
		newID := MissedRunID(m.AgentID, m.ExpectedAt)
		if slices.Contains(existingByAgent[m.AgentID], newID) {
			continue
		}
		for _, id := range existingByAgent[m.AgentID] {
			if err := os.Remove(filepath.Join(runsDir, id+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
				return written, runs, fmt.Errorf("remove stale miss %s: %w", id, err)
			}
			runs = removeRunByID(runs, id)
		}
		newRun, err := writeMissedRun(runsDir, newID, m)
		if err != nil {
			return written, runs, err
		}
		runs = append(runs, newRun)
		written = append(written, m)
	}
	return written, runs, nil
}

// removeRunByID returns runs with any entry matching id stripped. Order is
// not preserved (callers re-sort as needed; DetectCatchups doesn't care).
func removeRunByID(runs []JobRun, id string) []JobRun {
	out := runs[:0]
	for _, r := range runs {
		if r.ID == id {
			continue
		}
		out = append(out, r)
	}
	return out
}

// writeMissedRun marshals a JobRun{Status: StatusMissed, ...} for the given
// slot, writes it atomically (temp+rename) into runsDir, and returns the
// in-memory record (with StartedAtTime populated) so callers can stitch it
// into their working runs slice without re-reading the file.
func writeMissedRun(runsDir, id string, m MissedRun) (JobRun, error) {
	expectedUTC := m.ExpectedAt.UTC()
	run := JobRun{
		ID:            id,
		JobID:         m.AgentID,
		ScheduleID:    nil,
		Trigger:       "schedule",
		StartedAt:     expectedUTC.Format(time.RFC3339Nano),
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
		return JobRun{}, fmt.Errorf("marshal miss %s: %w", id, err)
	}
	full := filepath.Join(runsDir, id+".json")
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return JobRun{}, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return JobRun{}, fmt.Errorf("rename %s: %w", full, err)
	}
	return run, nil
}
