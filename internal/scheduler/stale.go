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

// staleClockSkewBound caps how far `now - startedAt` may exceed maxAge
// before we treat it as clock skew rather than a stale run. Without it, an
// NTP forward-jump after wake-from-sleep can falsely classify live runs as
// stale. 24h past the threshold is the floor we trust.
const staleClockSkewBound = 24 * time.Hour

// SweepStaleRunning rewrites runs that have been status="running" for
// longer than maxAge as status="error" with a fixed message. Returns the
// number of records mutated.
//
// Records whose computed age is negative (wall-clock skew backwards) or
// implausibly large (`maxAge + staleClockSkewBound` ahead) are skipped —
// the run's StartedAtTime has no monotonic clock so Sub() falls back to
// wall-clock subtraction and a system-clock jump can produce nonsense.
func SweepStaleRunning(store *FileRunStore, runs []Run, now time.Time, maxAge time.Duration) (int, error) {
	count := 0
	for _, r := range runs {
		if r.Status != StatusRunning {
			continue
		}
		if r.StartedAtTime.IsZero() {
			continue
		}
		age := now.Sub(r.StartedAtTime)
		if age < maxAge {
			continue
		}
		if age > maxAge+staleClockSkewBound {
			continue
		}
		wrote, err := rewriteStaleRunning(store.Dir(), r, now)
		if err != nil {
			return count, fmt.Errorf("rewrite %s: %w", r.ID, err)
		}
		if wrote {
			count++
		}
	}
	return count, nil
}

// rewriteStaleRunning re-reads the on-disk record just before writing; if
// the wrapper finished naturally between Sweep's load and now, the record
// is no longer status=running and we leave it alone. Narrows (but does not
// close) the race against wrapper.sh's terminal-state write.
func rewriteStaleRunning(runsDir string, r Run, now time.Time) (bool, error) {
	full := filepath.Join(runsDir, r.ID+".json")
	current, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var cur Run
	if err := json.Unmarshal(current, &cur); err == nil && cur.Status != StatusRunning {
		return false, nil
	}

	endedAt := FormatRunTimestamp(now)
	errMsg := "no completion record"
	exit := 1
	r.Status = StatusError
	r.EndedAt = &endedAt
	r.Error = &errMsg
	r.ExitCode = &exit

	buf, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return false, err
	}
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
}
