package scheduler

import (
	"sort"
	"time"

	"github.com/robfig/cron/v3"
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
	// lookbackBound caps how far back DetectMissed walks the cron schedule.
	// The longest period any ScheduleSpec can produce is once-per-week
	// (daily with a single weekday), so 8 days is enough to find the latest
	// expected slot for any schedule we support.
	lookbackBound = 8 * 24 * time.Hour
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// DetectMissed returns at most one MissedRun per agent: the most-recent
// expected slot <= now that no run covers. By design we don't surface every
// missed slot in a multi-slot outage — once the latest is recorded as a
// Run{status:"missed"} on disk, rule (a) below covers it on subsequent
// ticks. Replacement (one miss file per agent at a time) happens in
// RecordMissedRuns.
//
// Coverage rules (isCovered):
//
//	(a) any run within ±jitter of the slot — regardless of status, so a
//	    previously-recorded miss record at this slot acts as an ack and
//	    prevents re-emission on the next tick.
//	(b) any terminal run (success|error) at-or-after the slot — manual
//	    catch-up or a later scheduled fire.
//	(c) any running record at-or-after slot - jitter — wrapper in flight.
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
		expr, err := CompileToCron(*a.Meta.Schedule)
		if err != nil {
			continue
		}
		sched, err := cronParser.Parse(expr)
		if err != nil {
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

		// Walk forward and keep only the last slot <= now. cron/v3 has no
		// Prev(), so we step through Next() and overwrite. The lookback
		// floor + maxTicks bound the loop tightly.
		var latest time.Time
		cursor := earliest.Add(-time.Second)
		for i := 0; i < maxTicks; i++ {
			next := sched.Next(cursor)
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
	// There's only one entry per agent so this just orders agents.
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
		// (b) terminal run at-or-after the slot — manual catch-up or later fire.
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
