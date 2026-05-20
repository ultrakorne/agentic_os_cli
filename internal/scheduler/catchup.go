package scheduler

import (
	"sort"
	"time"
)

// MinCatchupSlotAge is the quiet window after a missed slot during which
// DetectCatchups will not yet fire a catch-up. The race it guards against:
// when the laptop is alive at the scheduled minute, cron fires the agent
// wrapper at the slot start, but the wrapper's "running" marker can take
// several hundred ms to land on disk (wrapper.sh shells out to python3 for
// the ISO timestamp). The */10 tick that fires in the same minute reads
// the runs/ dir before that marker exists, sees the slot as missed, and
// races a duplicate catch-up wrapper.
//
// Waiting 60s defers the catch-up decision to the next tick, by which point
// either the cron-spawned wrapper has registered (rule (b)/(c) in
// DetectMissed covers the slot, no catch-up) or it really never fired
// (catch-up proceeds normally). The trade-off is up to one tick's worth of
// extra catch-up latency for genuinely-missed slots — fine, since the tick
// cadence already dominates that timeline.
const MinCatchupSlotAge = 60 * time.Second

// CatchupCandidate identifies one agent + missed slot that aos tick should
// auto-fire a catch-up wrapper for. MissedSlot is the missed record's
// startedAt (RFC3339 of the expected slot) and gets passed through as the
// wrapper's scheduleId argv so the catch-up record links back to the slot
// it covers.
type CatchupCandidate struct {
	AgentID    string
	ScriptPath string
	MissedSlot string
}

// DetectCatchups returns one candidate per agent whose latest run (by
// startedAt) has status="missed" AND whose slot is older than
// MinCatchupSlotAge (see that constant for the race it guards). The "latest
// is missed" rule is the only trigger condition — a completed scheduled run,
// a manual run, a running wrapper, or a previously-fired catch-up that
// succeeded or failed all supersede the missed record and short-circuit
// auto-fire. Agents without a schedule are skipped: clearing an agent's
// schedule also clears it from the catch-up loop.
//
// Order is stable by AgentID so the caller's log output is deterministic.
//
// "Latest" is compared via StartedAtTime (parsed by FileRunStore.Load) rather
// than the raw StartedAt string. FileRunStore writes StartedAt via
// FormatRunTimestamp so every Go-side writer uses the same millisecond UTC
// shape wrapper.sh's iso_now produces, but historical records on disk may
// still mix shapes — the time.Time comparison keeps the sort honest either
// way.
func DetectCatchups(agents []Agent, runs []Run, now time.Time) []CatchupCandidate {
	latestByAgent := map[string]Run{}
	for _, r := range runs {
		if r.AgentID == "" || r.StartedAtTime.IsZero() {
			continue
		}
		if prev, ok := latestByAgent[r.AgentID]; !ok || r.StartedAtTime.After(prev.StartedAtTime) {
			latestByAgent[r.AgentID] = r
		}
	}

	var out []CatchupCandidate
	for _, a := range agents {
		if a.Meta.Schedule == nil {
			continue
		}
		latest, ok := latestByAgent[a.ID]
		if !ok || latest.Status != StatusMissed {
			continue
		}
		if !now.IsZero() && now.Sub(latest.StartedAtTime) < MinCatchupSlotAge {
			continue
		}
		out = append(out, CatchupCandidate{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			MissedSlot: latest.StartedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out
}
