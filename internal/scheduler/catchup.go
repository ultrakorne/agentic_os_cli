package scheduler

import "sort"

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
// startedAt) has status="missed". The "latest is missed" rule is the only
// trigger condition — a completed scheduled run, a manual run, a running
// wrapper, or a previously-fired catch-up that succeeded or failed all
// supersede the missed record and short-circuit auto-fire. Agents without a
// schedule are skipped: clearing an agent's schedule also clears it from the
// catch-up loop.
//
// Order is stable by AgentID so the caller's log output is deterministic.
//
// "Latest" is compared via StartedAtTime (parsed by FileRunStore.Load) rather
// than the raw StartedAt string. FileRunStore writes StartedAt via
// FormatRunTimestamp so every Go-side writer uses the same millisecond UTC
// shape wrapper.sh's iso_now produces, but historical records on disk may
// still mix shapes — the time.Time comparison keeps the sort honest either
// way.
func DetectCatchups(agents []Agent, runs []Run) []CatchupCandidate {
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
		out = append(out, CatchupCandidate{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			MissedSlot: latest.StartedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out
}
