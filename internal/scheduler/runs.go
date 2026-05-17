package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RunStatus string

const (
	StatusRunning RunStatus = "running"
	StatusSuccess RunStatus = "success"
	StatusError   RunStatus = "error"
	// StatusMissed marks a scheduled slot the wrapper never fired. Recorded
	// by `aos tick` / `aos refresh`, with startedAt = the expected slot and
	// endedAt = nil. Only the latest uncovered slot per agent is persisted;
	// see internal/scheduler/missed_record.go.
	StatusMissed RunStatus = "missed"
)

// JobRun mirrors the on-disk wrapper output and the renderer's shared/JobRun
// type (src/shared/scheduler.ts). Optional fields use pointers so JSON
// round-trips as `null` (matching what the wrapper writes and what the
// renderer expects) instead of dropping or zero-defaulting them.
type JobRun struct {
	ID         string    `json:"id"`
	JobID      string    `json:"jobId"`
	ScheduleID *string   `json:"scheduleId"`
	Trigger    string    `json:"trigger"`
	StartedAt  string    `json:"startedAt"`
	EndedAt    *string   `json:"endedAt"`
	Status     RunStatus `json:"status"`
	Output     string    `json:"output"`
	Error      *string   `json:"error"`
	ExitCode   *int      `json:"exitCode"`
	OutputPath *string   `json:"outputPath"`

	// derived
	StartedAtTime time.Time `json:"-"`
}

// LoadRuns reads every <runsDir>/*.json into a JobRun slice. Malformed files
// are silently skipped. Order is filesystem-defined (caller sorts as needed).
// missed.go consumes this directly; aos runs goes through ReadRuns.
func LoadRuns(runsDir string) ([]JobRun, error) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]JobRun, 0, len(entries))
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
		var r JobRun
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

// ReadRuns is the aos runs read path: drops malformed records (missing
// id/jobId/startedAt), optionally filters by agentID, sorts by StartedAt
// descending, and caps at limit. Pass limit=0 for "no limit".
//
// Sorting compares StartedAtTime (parsed from the JSON by LoadRuns), not the
// raw string. ISO-8601 *does* sort chronologically as a string when the
// subsecond format is uniform — but our writers aren't uniform. wrapper.sh
// carries ms (".123Z"); aos run forces 3-digit ms (".000Z"); a miss record
// formats with time.RFC3339Nano on a zero-subsecond expected slot, which
// strips the fraction entirely ("Z"). ASCII '.' (46) < 'Z' (90), so a
// same-second mixed-format collision lex-inverts.
func ReadRuns(runsDir, agentID string, limit int) ([]JobRun, error) {
	all, err := LoadRuns(runsDir)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if r.ID == "" || r.JobID == "" || r.StartedAt == "" {
			continue
		}
		if agentID != "" && r.JobID != agentID {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAtTime.After(out[j].StartedAtTime)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// EstimateRunDuration averages the elapsed time of the newest completed runs
// for agentID, capped at limit. Runs without a parseable endedAt are ignored.
func EstimateRunDuration(runsDir, agentID string, limit int) (time.Duration, bool, error) {
	runs, err := ReadRuns(runsDir, agentID, 0)
	if err != nil {
		return 0, false, err
	}
	var total time.Duration
	count := 0
	for _, r := range runs {
		if limit > 0 && count >= limit {
			break
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

// ReadRun reads one run by id. Returns NotFoundError if the file is absent.
func ReadRun(runsDir, runID string) (JobRun, error) {
	path := filepath.Join(runsDir, runID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return JobRun{}, NotFoundError{ID: runID}
		}
		return JobRun{}, err
	}
	var run JobRun
	if err := json.Unmarshal(data, &run); err != nil {
		return JobRun{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if run.ID == "" {
		run.ID = runID
	}
	if run.StartedAt != "" {
		if t, perr := time.Parse(time.RFC3339Nano, run.StartedAt); perr == nil {
			run.StartedAtTime = t
		}
	}
	return run, nil
}

// ReadRunOutput reads the .out file for runID. Resolves the actual filename
// from the run's OutputPath when set, falling back to "<runID>.out". Returns
// (nil, nil) when the run exists but the .out file is absent — running runs
// and runs that produced no output both legitimately lack a .out file.
func ReadRunOutput(runsDir, runID string) ([]byte, error) {
	run, err := ReadRun(runsDir, runID)
	if err != nil {
		return nil, err
	}
	name := runID + ".out"
	if run.OutputPath != nil && *run.OutputPath != "" {
		name = *run.OutputPath
	}
	data, err := os.ReadFile(filepath.Join(runsDir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}
