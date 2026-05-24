package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StaleRunningThreshold is how long after StartedAt a `running` run is
// considered abandoned and rewritten as error. The backend reload kill window
// is single-digit minutes; an hour is well past anything legitimate.
const StaleRunningThreshold = time.Hour

// SweepStaleRunning rewrites runs that have been status="running" for
// longer than maxAge as status="error" with a fixed message. Returns the
// number of records mutated.
func SweepStaleRunning(store *FileRunStore, runs []Run, now time.Time, maxAge time.Duration) (int, error) {
	count := 0
	for _, r := range runs {
		if r.Status != StatusRunning {
			continue
		}
		if r.StartedAtTime.IsZero() {
			continue
		}
		if now.Sub(r.StartedAtTime) < maxAge {
			continue
		}
		if err := rewriteStaleRunning(store.Dir(), r, now); err != nil {
			return count, fmt.Errorf("rewrite %s: %w", r.ID, err)
		}
		count++
	}
	return count, nil
}

func rewriteStaleRunning(runsDir string, r Run, now time.Time) error {
	endedAt := FormatRunTimestamp(now)
	errMsg := "no completion record"
	exit := 1
	r.Status = StatusError
	r.EndedAt = &endedAt
	r.Error = &errMsg
	r.ExitCode = &exit

	buf, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	full := filepath.Join(runsDir, r.ID+".json")
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
