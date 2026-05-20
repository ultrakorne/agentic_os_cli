package scheduler

import (
	"testing"
	"time"
)

// run is a tiny helper so each test reads as a one-line "agent X had run with
// status S at time T" rather than a multi-line struct literal. Mirrors what
// LoadRuns produces in production: both StartedAt (string, on disk) and
// StartedAtTime (parsed, used for comparisons) are populated.
func run(agentID string, status RunStatus, at time.Time) Run {
	utc := at.UTC()
	return Run{
		ID:            agentID + "-" + utc.Format(time.RFC3339),
		AgentID:         agentID,
		StartedAt:     utc.Format(time.RFC3339Nano),
		StartedAtTime: utc,
		Status:        status,
	}
}

func TestDetectCatchups_firesWhenLatestIsMissed(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))
	runs := []Run{
		run("ping", StatusSuccess, now.Add(-2*time.Hour)),
		run("ping", StatusMissed, now.Add(-30*time.Minute)),
	}

	out := DetectCatchups([]Agent{a}, runs, now)
	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d (%+v)", len(out), out)
	}
	if out[0].AgentID != "ping" {
		t.Errorf("AgentID = %q, want %q", out[0].AgentID, "ping")
	}
	wantSlot := now.Add(-30 * time.Minute).UTC().Format(time.RFC3339Nano)
	if out[0].MissedSlot != wantSlot {
		t.Errorf("MissedSlot = %q, want %q", out[0].MissedSlot, wantSlot)
	}
}

func TestDetectCatchups_skipsLatestNonMissed(t *testing.T) {
	// The rule is "only if latest is missed". Anything else short-circuits.
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))

	cases := []struct {
		name   string
		status RunStatus
	}{
		{"latest is success (manual or scheduled, completed)", StatusSuccess},
		{"latest is error (script failed; do not retry)", StatusError},
		{"latest is running (wrapper in flight)", StatusRunning},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runs := []Run{
				run("ping", StatusMissed, now.Add(-2*time.Hour)),
				run("ping", tc.status, now.Add(-30*time.Minute)),
			}
			if got := DetectCatchups([]Agent{a}, runs, now); len(got) != 0 {
				t.Errorf("expected 0 candidates, got %+v", got)
			}
		})
	}
}

func TestDetectCatchups_skipsAgentWithNoRuns(t *testing.T) {
	// A scheduled agent with no runs yet is *not* a catch-up target — the
	// missed-run sweep is what surfaces never-fired slots, and detection here
	// only kicks in after a missed record exists.
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))
	if got := DetectCatchups([]Agent{a}, nil, now); len(got) != 0 {
		t.Errorf("expected 0 candidates, got %+v", got)
	}
}

func TestDetectCatchups_skipsUnscheduledAgent(t *testing.T) {
	// User cleared the schedule but the historical missed record is still on
	// disk. We must not fire a catch-up after the user opted out of scheduling.
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := Agent{ID: "ping"} // no schedule
	runs := []Run{run("ping", StatusMissed, now.Add(-30*time.Minute))}
	if got := DetectCatchups([]Agent{a}, runs, now); len(got) != 0 {
		t.Errorf("expected 0 candidates, got %+v", got)
	}
}

func TestDetectCatchups_idempotentOnceCatchupIsRunning(t *testing.T) {
	// First tick fires the catch-up; the wrapper writes a "running" record.
	// Next tick must see latest=running and not double-fire.
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))
	runs := []Run{
		run("ping", StatusMissed, now.Add(-30*time.Minute)),
		run("ping", StatusRunning, now.Add(-1*time.Minute)),
	}
	if got := DetectCatchups([]Agent{a}, runs, now); len(got) != 0 {
		t.Errorf("expected 0 candidates (catch-up already running), got %+v", got)
	}
}

func TestDetectCatchups_doesNotRetryFailedCatchup(t *testing.T) {
	// Catch-up itself errored. Latest = error, not missed, so no further
	// auto-rerun — the only retry condition is "latest is missed".
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))
	runs := []Run{
		run("ping", StatusMissed, now.Add(-30*time.Minute)),
		run("ping", StatusError, now.Add(-1*time.Minute)),
	}
	if got := DetectCatchups([]Agent{a}, runs, now); len(got) != 0 {
		t.Errorf("expected 0 candidates (catch-up failed; do not retry), got %+v", got)
	}
}

func TestDetectCatchups_quietWindowDefersFreshSlot(t *testing.T) {
	// Race scenario: the laptop is alive at the scheduled minute, cron fires
	// the wrapper at the slot start, but the */10 tick also fires in the same
	// second and reads runs/ before wrapper.sh has written its "running"
	// marker (wrapper's iso_now shells out to python3 — hundreds of ms). The
	// tick sees the slot as missed and would otherwise race a duplicate
	// catch-up. The quiet window defers that decision to the next tick, by
	// which time the cron-spawned wrapper will have registered.
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))
	freshMiss := run("ping", StatusMissed, now.Add(-1*time.Second))

	if got := DetectCatchups([]Agent{a}, []Run{freshMiss}, now); len(got) != 0 {
		t.Errorf("expected 0 candidates within quiet window, got %+v", got)
	}

	// Sanity check the boundary: a miss older than MinCatchupSlotAge still
	// fires. The previous case must be exercising the window, not some other
	// short-circuit.
	oldMiss := run("ping", StatusMissed, now.Add(-MinCatchupSlotAge-time.Second))
	if got := DetectCatchups([]Agent{a}, []Run{oldMiss}, now); len(got) != 1 {
		t.Errorf("expected 1 candidate past quiet window, got %+v", got)
	}
}

func TestDetectCatchups_zeroNowDisablesQuietWindow(t *testing.T) {
	// Callers that don't care about wall-clock pacing (older test paths, ad
	// hoc tooling) can pass time.Time{} to skip the quiet-window check and
	// get the original "latest is missed → candidate" behavior.
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))
	freshMiss := run("ping", StatusMissed, now.Add(-1*time.Second))

	if got := DetectCatchups([]Agent{a}, []Run{freshMiss}, time.Time{}); len(got) != 1 {
		t.Errorf("expected 1 candidate with zero now, got %+v", got)
	}
}

// TestDetectCatchups_sameSecondMixedTimestampFormat pins the fix for a bug
// where string-compare on StartedAt mis-ordered same-second records when one
// side carried subseconds and the other didn't.
//
// Miss records are written via time.RFC3339Nano on a zero-subsecond expected
// slot, which strips the fraction: "2026-05-17T11:00:00Z". A wrapper that
// fires within the same second emits "2026-05-17T11:00:00.500Z" (wrapper.sh
// always carries ms; aos run forces 3-digit ms). ASCII '.' (46) < 'Z' (90),
// so the wrapper string lex-compares *before* the miss — a string-only
// "latest" pick would wrongly select the miss as the latest and trigger a
// phantom catch-up. Comparing parsed times instead avoids this entirely.
func TestDetectCatchups_sameSecondMixedTimestampFormat(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	a := hourlyAgent("ping", 0, now.Add(-48*time.Hour))
	slot := time.Date(2026, 5, 17, 11, 0, 0, 0, time.UTC)
	wrapperStart := slot.Add(500 * time.Millisecond)

	miss := Run{
		ID:            "miss-ping",
		AgentID:         "ping",
		StartedAt:     slot.Format(time.RFC3339Nano), // "2026-05-17T11:00:00Z"
		StartedAtTime: slot,
		Status:        StatusMissed,
	}
	real := Run{
		ID:            "real-success",
		AgentID:         "ping",
		StartedAt:     wrapperStart.Format("2006-01-02T15:04:05.000Z"), // "...:00.500Z"
		StartedAtTime: wrapperStart,
		Status:        StatusSuccess,
	}

	// Sanity check the premise: the strings really do lex-invert.
	if !(real.StartedAt < miss.StartedAt) {
		t.Fatalf("test premise broken: expected %q < %q lex", real.StartedAt, miss.StartedAt)
	}

	if got := DetectCatchups([]Agent{a}, []Run{miss, real}, now); len(got) != 0 {
		t.Errorf("expected 0 candidates (real wrapper later in real time despite lex inversion), got %+v", got)
	}
}

func TestDetectCatchups_multipleAgentsStableOrder(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	zebra := hourlyAgent("zebra", 0, now.Add(-48*time.Hour))
	apple := hourlyAgent("apple", 0, now.Add(-48*time.Hour))
	mango := hourlyAgent("mango", 0, now.Add(-48*time.Hour)) // not behind
	runs := []Run{
		run("zebra", StatusMissed, now.Add(-30*time.Minute)),
		run("apple", StatusMissed, now.Add(-31*time.Minute)),
		run("mango", StatusSuccess, now.Add(-29*time.Minute)),
	}
	out := DetectCatchups([]Agent{zebra, apple, mango}, runs, now)
	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d (%+v)", len(out), out)
	}
	if out[0].AgentID != "apple" || out[1].AgentID != "zebra" {
		t.Errorf("order = [%s, %s], want [apple, zebra]", out[0].AgentID, out[1].AgentID)
	}
}
