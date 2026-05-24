package scheduler

import (
	"testing"
	"time"
)

func TestScheduleSpec_NextSlot_hourly(t *testing.T) {
	utc := time.UTC
	cases := []struct {
		name  string
		spec  ScheduleSpec
		after time.Time
		want  time.Time
	}{
		{
			name:  "every hour at :15, before :15 in same hour",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 15},
			after: time.Date(2026, 5, 17, 10, 0, 0, 0, utc),
			want:  time.Date(2026, 5, 17, 10, 15, 0, 0, utc),
		},
		{
			name:  "every hour at :15, after :15 in same hour rolls to next",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 15},
			after: time.Date(2026, 5, 17, 10, 20, 0, 0, utc),
			want:  time.Date(2026, 5, 17, 11, 15, 0, 0, utc),
		},
		{
			name:  "every hour at :15, exact match rolls to next (strictly after)",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 15},
			after: time.Date(2026, 5, 17, 10, 15, 0, 0, utc),
			want:  time.Date(2026, 5, 17, 11, 15, 0, 0, utc),
		},
		{
			name:  "every 4h at :05, before first aligned hour",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 4, Minute: 5},
			after: time.Date(2026, 5, 17, 1, 0, 0, 0, utc),
			want:  time.Date(2026, 5, 17, 4, 5, 0, 0, utc),
		},
		{
			name:  "every 4h at :05, on aligned hour but past minute → next aligned",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 4, Minute: 5},
			after: time.Date(2026, 5, 17, 4, 6, 0, 0, utc),
			want:  time.Date(2026, 5, 17, 8, 5, 0, 0, utc),
		},
		{
			name:  "every 4h at :05, last aligned hour of day rolls to next day 00:05",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 4, Minute: 5},
			after: time.Date(2026, 5, 17, 22, 0, 0, 0, utc),
			want:  time.Date(2026, 5, 18, 0, 5, 0, 0, utc),
		},
		{
			name:  "every 12h at :00",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 12, Minute: 0},
			after: time.Date(2026, 5, 17, 1, 0, 0, 0, utc),
			want:  time.Date(2026, 5, 17, 12, 0, 0, 0, utc),
		},
		{
			name:  "every 12h at :00 just after noon",
			spec:  ScheduleSpec{Kind: "hourly", EveryHours: 12, Minute: 0},
			after: time.Date(2026, 5, 17, 12, 0, 1, 0, utc),
			want:  time.Date(2026, 5, 18, 0, 0, 0, 0, utc),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.spec.NextSlot(tc.after)
			if !got.Equal(tc.want) {
				t.Errorf("NextSlot(%v) = %v, want %v", tc.after, got, tc.want)
			}
		})
	}
}

func TestScheduleSpec_NextSlot_daily(t *testing.T) {
	utc := time.UTC
	cases := []struct {
		name  string
		spec  ScheduleSpec
		after time.Time
		want  time.Time
	}{
		{
			name:  "weekdays 09:00 - Sunday late evening rolls to Monday morning",
			spec:  ScheduleSpec{Kind: "daily", Days: []Weekday{Mon, Tue, Wed, Thu, Fri}, Hour: 9, Minute: 0},
			after: time.Date(2026, 5, 17, 22, 0, 0, 0, utc), // Sunday
			want:  time.Date(2026, 5, 18, 9, 0, 0, 0, utc),  // Monday
		},
		{
			name:  "Mon-only - Tuesday afternoon → next Monday",
			spec:  ScheduleSpec{Kind: "daily", Days: []Weekday{Mon}, Hour: 9, Minute: 0},
			after: time.Date(2026, 5, 19, 15, 0, 0, 0, utc), // Tue
			want:  time.Date(2026, 5, 25, 9, 0, 0, 0, utc),  // next Mon
		},
		{
			name:  "Mon-only - Monday before slot",
			spec:  ScheduleSpec{Kind: "daily", Days: []Weekday{Mon}, Hour: 9, Minute: 0},
			after: time.Date(2026, 5, 18, 8, 59, 59, 0, utc),
			want:  time.Date(2026, 5, 18, 9, 0, 0, 0, utc),
		},
		{
			name:  "Mon-only - exactly at slot rolls to next week (strictly after)",
			spec:  ScheduleSpec{Kind: "daily", Days: []Weekday{Mon}, Hour: 9, Minute: 0},
			after: time.Date(2026, 5, 18, 9, 0, 0, 0, utc),
			want:  time.Date(2026, 5, 25, 9, 0, 0, 0, utc),
		},
		{
			name:  "Sat+Sun at 12:00 - Friday evening → Saturday",
			spec:  ScheduleSpec{Kind: "daily", Days: []Weekday{Sat, Sun}, Hour: 12, Minute: 0},
			after: time.Date(2026, 5, 22, 20, 0, 0, 0, utc), // Friday
			want:  time.Date(2026, 5, 23, 12, 0, 0, 0, utc), // Saturday
		},
		{
			name:  "Sat+Sun at 12:00 - Sunday afternoon → next Saturday (wrap)",
			spec:  ScheduleSpec{Kind: "daily", Days: []Weekday{Sat, Sun}, Hour: 12, Minute: 0},
			after: time.Date(2026, 5, 24, 13, 0, 0, 0, utc), // Sun after slot
			want:  time.Date(2026, 5, 30, 12, 0, 0, 0, utc), // next Sat
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.spec.NextSlot(tc.after)
			if !got.Equal(tc.want) {
				t.Errorf("NextSlot(%v) = %v, want %v", tc.after, got, tc.want)
			}
		})
	}
}

func TestScheduleSpec_NextSlot_localTime(t *testing.T) {
	// Same wall-clock instant must evaluate identically in any zone — the OS
	// scheduler interprets entries in local time.
	loc := time.FixedZone("test+02", 2*60*60)
	spec := ScheduleSpec{Kind: "daily", Days: []Weekday{Mon, Tue, Wed, Thu, Fri}, Hour: 9, Minute: 0}
	// 08:30 local on Monday → next slot is 09:00 local same day.
	after := time.Date(2026, 5, 18, 8, 30, 0, 0, loc)
	got := spec.NextSlot(after)
	want := time.Date(2026, 5, 18, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("NextSlot in fixed +02 = %v, want %v", got, want)
	}
	if got.Location().String() != loc.String() {
		t.Errorf("NextSlot location = %v, want %v", got.Location(), loc)
	}
}

func TestScheduleSpec_NextSlot_dstSpringForwardSkipsGap(t *testing.T) {
	// US Eastern: 2026-03-08 02:00 local → 03:00 local (spring forward).
	// "Daily Sun at 02:30" must skip the spring-forward Sunday entirely,
	// not roll the normalized 03:30 EDT into a fake slot. launchd's
	// StartCalendarInterval and systemd's OnCalendar both refuse to fire
	// on a non-existent local time; aos must agree to avoid emitting a
	// phantom miss every spring-forward Sunday.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata available")
	}
	spec := ScheduleSpec{Kind: "daily", Days: []Weekday{Sun}, Hour: 2, Minute: 30}
	after := time.Date(2026, 3, 8, 1, 0, 0, 0, loc)
	got := spec.NextSlot(after)
	want := time.Date(2026, 3, 15, 2, 30, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("NextSlot across spring-forward gap = %v, want %v", got, want)
	}
}

func TestScheduleSpec_NextSlot_dstFallBackPicksValidHour(t *testing.T) {
	// US Eastern: 2026-11-01 02:00 EDT → 01:00 EST (fall back; 01:30 happens
	// twice). Daily Sun at 01:30 must still produce a real slot. time.Date
	// returns the standard-convention instance, which is well-defined.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata available")
	}
	spec := ScheduleSpec{Kind: "daily", Days: []Weekday{Sun}, Hour: 1, Minute: 30}
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	got := spec.NextSlot(after)
	if got.IsZero() {
		t.Fatal("expected a slot on fall-back Sunday, got zero")
	}
	if got.Hour() != 1 || got.Minute() != 30 {
		t.Errorf("slot wall-clock = %02d:%02d, want 01:30", got.Hour(), got.Minute())
	}
}

func TestScheduleSpec_NextSlot_invalid(t *testing.T) {
	cases := []struct {
		name string
		spec ScheduleSpec
	}{
		{"unknown kind", ScheduleSpec{Kind: "monthly"}},
		{"hourly out of range", ScheduleSpec{Kind: "hourly", EveryHours: 0, Minute: 0}},
		{"hourly minute out of range", ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 60}},
		{"daily no days", ScheduleSpec{Kind: "daily", Hour: 9, Minute: 0}},
		{"daily hour out of range", ScheduleSpec{Kind: "daily", Days: []Weekday{Mon}, Hour: 24, Minute: 0}},
		{"daily bad weekday", ScheduleSpec{Kind: "daily", Days: []Weekday{"funday"}, Hour: 9, Minute: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.spec.NextSlot(time.Now())
			if !got.IsZero() {
				t.Errorf("expected zero time for invalid spec, got %v", got)
			}
		})
	}
}
