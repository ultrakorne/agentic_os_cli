package scheduler

import (
	"sort"
	"time"

	"github.com/robfig/cron/v3"
)

type MissedRun struct {
	AgentID    string
	ExpectedAt time.Time
}

type DetectOpts struct {
	Now      time.Time
	Window   time.Duration
	Jitter   time.Duration
	MaxTicks int
}

const (
	DefaultWindow   = 24 * time.Hour
	DefaultJitter   = 30 * time.Second
	DefaultMaxTicks = 500
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// DetectMissed flags expected fire times in [now-window, now] for which no
// wrapper run was recorded. The wrapper writes status="running" before
// `exec`, so any run within ±jitter of an expected slot covers it, regardless
// of final status. Terminal runs at or after a slot also cover it (manual
// catch-up). A running record at-or-after E-jitter covers E (in-flight).
func DetectMissed(agents []Agent, runs []JobRun, opts DetectOpts) []MissedRun {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	window := opts.Window
	if window == 0 {
		window = DefaultWindow
	}
	jitter := opts.Jitter
	if jitter == 0 {
		jitter = DefaultJitter
	}
	maxTicks := opts.MaxTicks
	if maxTicks == 0 {
		maxTicks = DefaultMaxTicks
	}
	cutoff := now.Add(-window)

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

		earliest := cutoff
		if a.Meta.ScheduledAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, a.Meta.ScheduledAt); err == nil {
				if t.After(earliest) {
					earliest = t
				}
			}
		}

		var expected []time.Time
		cursor := earliest.Add(-time.Second)
		for i := 0; i < maxTicks; i++ {
			next := sched.Next(cursor)
			if next.IsZero() || next.After(now) {
				break
			}
			expected = append(expected, next)
			cursor = next
		}
		if len(expected) == 0 {
			continue
		}

		agentRuns := filterRuns(runs, a.ID)
		sort.Slice(agentRuns, func(i, j int) bool {
			return agentRuns[i].StartedAtTime.Before(agentRuns[j].StartedAtTime)
		})

		for _, E := range expected {
			if isCovered(E, agentRuns, jitter) {
				continue
			}
			out = append(out, MissedRun{AgentID: a.ID, ExpectedAt: E})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ExpectedAt.After(out[j].ExpectedAt)
	})
	return out
}

func filterRuns(runs []JobRun, agentID string) []JobRun {
	out := make([]JobRun, 0, len(runs))
	for _, r := range runs {
		if r.JobID == agentID && !r.StartedAtTime.IsZero() {
			out = append(out, r)
		}
	}
	return out
}

func isCovered(E time.Time, runs []JobRun, jitter time.Duration) bool {
	earliest := E.Add(-jitter)
	latest := E.Add(jitter)
	for _, r := range runs {
		t := r.StartedAtTime
		// (a) run matches this slot within ±jitter — covers regardless of status.
		if !t.Before(earliest) && !t.After(latest) {
			return true
		}
		// (b) terminal run at-or-after the slot — manual catch-up.
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
