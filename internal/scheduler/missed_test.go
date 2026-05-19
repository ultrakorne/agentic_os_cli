package scheduler

import (
	"strings"
	"testing"
	"time"
)

// hourlyAgent returns a scheduled hourly agent at the given minute, with
// scheduledAt anchoring the cron walk far enough back that DetectMissed will
// consider recent slots.
func hourlyAgent(id string, minute int, scheduledAt time.Time) Agent {
	return Agent{
		ID: id,
		Meta: AgentMeta{
			Schedule:    &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: minute},
			ScheduledAt: scheduledAt.UTC().Format(time.RFC3339Nano),
		},
	}
}

func TestDetectMissed_returnsOnlyTheLatestUncoveredSlotPerAgent(t *testing.T) {
	// An hourly agent that hasn't run for 3 hours. The naive "all slots in
	// window" algorithm would return three entries; the latest-only contract
	// requires exactly one — the most recent expected slot.
	now := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	scheduledAt := now.Add(-24 * time.Hour)
	a := hourlyAgent("ping", 0, scheduledAt)

	out := DetectMissed([]Agent{a}, nil, DetectOpts{Now: now})
	if len(out) != 1 {
		t.Fatalf("expected 1 miss, got %d (%+v)", len(out), out)
	}
	wantSlot := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	if !out[0].ExpectedAt.Equal(wantSlot) {
		t.Fatalf("expected slot %v, got %v", wantSlot, out[0].ExpectedAt)
	}
}

func TestDetectMissed_handlesWeeklyAcrossMultiDayOutage(t *testing.T) {
	// Daily-Monday-only at 09:00. Last fire 2 weeks ago, today is Thursday.
	// The most recent expected slot is "this past Monday at 09:00".
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC) // Thursday
	scheduledAt := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC) // Monday, 17 days back
	a := Agent{
		ID: "weekly",
		Meta: AgentMeta{
			Schedule:    &ScheduleSpec{Kind: "daily", Days: []Weekday{Mon}, Hour: 9, Minute: 0},
			ScheduledAt: scheduledAt.UTC().Format(time.RFC3339Nano),
		},
	}
	out := DetectMissed([]Agent{a}, nil, DetectOpts{Now: now})
	if len(out) != 1 {
		t.Fatalf("expected 1 miss, got %d (%+v)", len(out), out)
	}
	// This past Monday relative to Thu 2026-05-14 was 2026-05-11.
	want := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
	if !out[0].ExpectedAt.Equal(want) {
		t.Fatalf("expected %v got %v", want, out[0].ExpectedAt)
	}
}

func TestDetectMissed_coveredSlotProducesNothing(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	scheduledAt := now.Add(-2 * time.Hour)
	a := hourlyAgent("ping", 0, scheduledAt)

	// Successful run at the 12:00 slot — covers it via rule (a) (within jitter).
	runs := []Run{
		{
			ID:            "real-12",
			AgentID:         "ping",
			StartedAt:     "2026-05-16T12:00:00.500Z",
			Status:        StatusSuccess,
			StartedAtTime: time.Date(2026, 5, 16, 12, 0, 0, 500_000_000, time.UTC),
		},
	}
	out := DetectMissed([]Agent{a}, runs, DetectOpts{Now: now})
	if len(out) != 0 {
		t.Fatalf("expected 0 misses (slot covered), got %+v", out)
	}
}

func TestDetectMissed_recordedMissCoversItsSlot(t *testing.T) {
	// Steady-state idempotency: once a miss record is on disk, the next tick
	// at the same slot must not emit. Rule (a) covers regardless of status.
	now := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	scheduledAt := now.Add(-2 * time.Hour)
	a := hourlyAgent("ping", 0, scheduledAt)

	slot := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	runs := []Run{
		{
			ID:            MissedRunID("ping", slot),
			AgentID:         "ping",
			StartedAt:     slot.UTC().Format(time.RFC3339Nano),
			Status:        StatusMissed,
			StartedAtTime: slot,
		},
	}
	out := DetectMissed([]Agent{a}, runs, DetectOpts{Now: now})
	if len(out) != 0 {
		t.Fatalf("expected 0 misses (previously recorded), got %+v", out)
	}
}

func TestDetectMissed_terminalRunAfterSlotCoversIt(t *testing.T) {
	// Manual catch-up: user runs the agent after a slot was missed. Even
	// though the run is far outside the ±jitter of the slot, rule (b) covers.
	now := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	scheduledAt := now.Add(-2 * time.Hour)
	a := hourlyAgent("ping", 0, scheduledAt)

	manualRunAt := time.Date(2026, 5, 16, 12, 20, 0, 0, time.UTC)
	runs := []Run{
		{
			ID:            "manual-run",
			AgentID:         "ping",
			StartedAt:     manualRunAt.UTC().Format(time.RFC3339Nano),
			Status:        StatusSuccess,
			StartedAtTime: manualRunAt,
		},
	}
	out := DetectMissed([]Agent{a}, runs, DetectOpts{Now: now})
	if len(out) != 0 {
		t.Fatalf("expected 0 misses (manual catch-up covers), got %+v", out)
	}
}

func TestDetectMissed_ignoresSlotsBeforeScheduledAt(t *testing.T) {
	// Regression: changing an agent's schedule must not retroactively flag
	// slots that the *new* spec would have put in the past. WriteSchedule
	// bumps scheduledAt on every spec change; DetectMissed anchors its
	// cron-walk at scheduledAt, so any slot ≤ scheduledAt is invisible to
	// missed-detection. Without this, editing a schedule at 12:15 would
	// immediately surface a phantom 12:00 miss for the new spec.
	now := time.Date(2026, 5, 16, 12, 30, 0, 0, time.UTC)
	editedAt := time.Date(2026, 5, 16, 12, 15, 0, 0, time.UTC)
	// Hourly at :00 — the 12:00 slot is 30 min in the past relative to now,
	// but 15 min *before* the schedule was set. Must not be flagged.
	a := hourlyAgent("ping", 0, editedAt)

	out := DetectMissed([]Agent{a}, nil, DetectOpts{Now: now})
	if len(out) != 0 {
		t.Fatalf("expected 0 misses (12:00 slot predates scheduledAt 12:15), got %+v", out)
	}
}

func TestDetectMissed_cronWalkUsesLocalTime(t *testing.T) {
	// The OS crontab daemon interprets cron entries in local time. DetectMissed
	// must do the same — otherwise miss detection silently disagrees with the
	// real schedule on every non-UTC host.
	//
	// Setup: "daily at 09:00, Mon–Fri" on a host whose local time is UTC+2.
	// At 09:30 local (= 07:30 UTC) the 09:00 local slot has just passed and
	// nothing covers it. A UTC interpretation would compute the latest slot
	// as yesterday 09:00 UTC and find it "covered" by some later run, missing
	// today's real outage.
	loc := time.FixedZone("test+02", 2*60*60)
	now := time.Date(2026, 5, 19, 9, 30, 0, 0, loc) // Tue 09:30 local
	// scheduledAt is in UTC (RFC3339 with 'Z'), like every real meta file.
	scheduledAt := time.Date(2026, 5, 18, 12, 21, 54, 0, time.UTC)
	a := Agent{
		ID: "daily_planner",
		Meta: AgentMeta{
			Schedule: &ScheduleSpec{
				Kind:  "daily",
				Days:  []Weekday{Mon, Tue, Wed, Thu, Fri},
				Hour:  9,
				Minute: 0,
			},
			ScheduledAt: scheduledAt.Format(time.RFC3339Nano),
		},
	}

	// Yesterday's 09:00 local slot was covered by a manual run at 14:22 local,
	// matching the real-world repro on samir's box.
	monManualLocal := time.Date(2026, 5, 18, 14, 22, 29, 0, loc)
	runs := []Run{
		{
			ID:            "manual-mon",
			AgentID:       "daily_planner",
			StartedAt:     monManualLocal.UTC().Format(time.RFC3339Nano),
			Status:        StatusSuccess,
			StartedAtTime: monManualLocal,
		},
	}

	out := DetectMissed([]Agent{a}, runs, DetectOpts{Now: now})
	if len(out) != 1 {
		t.Fatalf("expected 1 miss for today 09:00 local, got %d (%+v)", len(out), out)
	}
	wantSlot := time.Date(2026, 5, 19, 9, 0, 0, 0, loc)
	if !out[0].ExpectedAt.Equal(wantSlot) {
		t.Fatalf("expected slot %v, got %v", wantSlot, out[0].ExpectedAt)
	}
}

func TestMissedRunID_isStableAndPortable(t *testing.T) {
	exp := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	got := MissedRunID("ping", exp)
	want := "miss-ping-2026-05-15T09-00-00Z"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if strings.Contains(got, ":") {
		t.Fatalf("id contains ':', not portable: %q", got)
	}
	again := MissedRunID("ping", exp)
	if again != got {
		t.Fatalf("non-deterministic: %q vs %q", got, again)
	}
}
