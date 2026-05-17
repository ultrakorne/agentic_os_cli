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
// "Latest" is compared via StartedAtTime (parsed by LoadRuns) rather than the
// raw StartedAt string. Writers emit subseconds inconsistently — wrapper.sh
// always carries ms (".123Z"), aos run forces 3-digit ms (".000Z"), but
// time.RFC3339Nano strips trailing zeros so a zero-subsecond miss record
// emits just "Z". ASCII '.' (46) < 'Z' (90), so a same-second wrapper run
// would lex-compare *before* its covering miss — flipping the order and
// triggering a phantom catch-up.
func DetectCatchups(agents []Agent, runs []JobRun) []CatchupCandidate {
	latestByAgent := map[string]JobRun{}
	for _, r := range runs {
		if r.JobID == "" || r.StartedAtTime.IsZero() {
			continue
		}
		if prev, ok := latestByAgent[r.JobID]; !ok || r.StartedAtTime.After(prev.StartedAtTime) {
			latestByAgent[r.JobID] = r
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
