package scheduler

import (
	"sort"
	"time"
)

// MissedRun is one uncovered scheduled slot for one agent.
type MissedRun struct {
	AgentID    string
	ExpectedAt time.Time
}

type DetectOpts struct {
	Now      time.Time
	Jitter   time.Duration
	MaxTicks int
}

const (
	DefaultJitter   = 30 * time.Second
	DefaultMaxTicks = 500
	// lookbackBound caps how far back DetectMissed walks the schedule. The
	// longest period any ScheduleSpec can produce is once-per-week (daily
	// with a single weekday), so 8 days is enough to find the latest
	// expected slot.
	lookbackBound = 8 * 24 * time.Hour
)

// DetectMissed returns at most one MissedRun per agent: the most-recent
// expected slot <= now that no run covers. See the original (cron-era)
// algorithm comment in run_store.go.
func DetectMissed(agents []Agent, runs []Run, opts DetectOpts) []MissedRun {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	jitter := opts.Jitter
	if jitter == 0 {
		jitter = DefaultJitter
	}
	maxTicks := opts.MaxTicks
	if maxTicks == 0 {
		maxTicks = DefaultMaxTicks
	}
	lookbackFloor := now.Add(-lookbackBound)

	var out []MissedRun
	for _, a := range agents {
		if a.Meta.Schedule == nil {
			continue
		}

		earliest := lookbackFloor
		if a.Meta.ScheduledAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, a.Meta.ScheduledAt); err == nil {
				if t.After(earliest) {
					earliest = t
				}
			}
		}

		// Walk NextSlot forward from earliest, capturing the last slot <= now.
		// Evaluate in now.Location() — the OS scheduler interprets schedules
		// in local time.
		cursor := earliest.In(now.Location()).Add(-time.Second)
		var latest time.Time
		for i := 0; i < maxTicks; i++ {
			next := a.Meta.Schedule.NextSlot(cursor)
			if next.IsZero() || next.After(now) {
				break
			}
			latest = next
			cursor = next
		}
		if latest.IsZero() {
			continue
		}

		agentRuns := filterRuns(runs, a.ID)
		if isCovered(latest, agentRuns, jitter) {
			continue
		}
		out = append(out, MissedRun{AgentID: a.ID, ExpectedAt: latest})
	}
	// Newest first so callers showing a "latest miss" view don't re-sort.
	sort.Slice(out, func(i, j int) bool {
		return out[i].ExpectedAt.After(out[j].ExpectedAt)
	})
	return out
}

func filterRuns(runs []Run, agentID string) []Run {
	out := make([]Run, 0, len(runs))
	for _, r := range runs {
		if r.AgentID == agentID && !r.StartedAtTime.IsZero() {
			out = append(out, r)
		}
	}
	return out
}

func isCovered(E time.Time, runs []Run, jitter time.Duration) bool {
	earliest := E.Add(-jitter)
	latest := E.Add(jitter)
	for _, r := range runs {
		t := r.StartedAtTime
		// (a) run matches this slot within ±jitter — covers regardless of
		// status. A previously-recorded miss record at this slot lands here,
		// so detect+record is idempotent across consecutive ticks.
		if !t.Before(earliest) && !t.After(latest) {
			return true
		}
		// (b) terminal run at-or-after the slot — manual run or later fire.
		if (r.Status == StatusSuccess || r.Status == StatusError) && !t.Before(E) {
			return true
		}
		// (c) wrapper in-flight for this slot or a later one.
		if r.Status == StatusRunning && !t.Before(earliest) {
			return true
		}
	}
	return false
}
