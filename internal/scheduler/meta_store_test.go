package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteSchedule_setsScheduledAtOnFirstWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo.meta.json")
	now := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)

	got, err := WriteSchedule(path, &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}, now)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if got.ScheduledAt == "" {
		t.Fatalf("scheduledAt not set on first write")
	}
	disk, err := ReadMeta(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if disk.ScheduledAt != got.ScheduledAt {
		t.Fatalf("disk scheduledAt %q != returned %q", disk.ScheduledAt, got.ScheduledAt)
	}
}

func TestWriteSchedule_preservesScheduledAtWhenSpecUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo.meta.json")
	spec := &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}
	first, err := WriteSchedule(path, spec, time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := WriteSchedule(path, spec, time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ScheduledAt != first.ScheduledAt {
		t.Fatalf("scheduledAt bumped on unchanged spec: %q -> %q", first.ScheduledAt, second.ScheduledAt)
	}
}

func TestWriteSchedule_bumpsScheduledAtOnSpecChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo.meta.json")
	first, err := WriteSchedule(path, &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}, time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := WriteSchedule(path, &ScheduleSpec{Kind: "hourly", EveryHours: 3, Minute: 0}, time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ScheduledAt == first.ScheduledAt {
		t.Fatalf("scheduledAt unchanged after spec change")
	}
}

func TestWriteSchedule_clearRemovesFileWhenNoOtherFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo.meta.json")
	if _, err := WriteSchedule(path, &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}, time.Now()); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file after set: %v", err)
	}
	if _, err := WriteSchedule(path, nil, time.Now()); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err=%v", err)
	}
}

func TestWriteSchedule_clearKeepsFileWhenDescriptionRemains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo.meta.json")
	if _, err := WriteDescription(path, "hi"); err != nil {
		t.Fatalf("desc: %v", err)
	}
	if _, err := WriteSchedule(path, &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}, time.Now()); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := WriteSchedule(path, nil, time.Now()); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err := ReadMeta(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Description != "hi" {
		t.Fatalf("description lost: %q", got.Description)
	}
	if got.Schedule != nil || got.ScheduledAt != "" {
		t.Fatalf("schedule/scheduledAt not cleared: %+v", got)
	}
}

func TestWriteDescription_emptyClearsAndRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo.meta.json")
	if _, err := WriteDescription(path, "hello"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := WriteDescription(path, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err=%v", err)
	}
}

func TestWriteDescription_emptyKeepsScheduleFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo.meta.json")
	if _, err := WriteSchedule(path, &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}, time.Now()); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if _, err := WriteDescription(path, "hello"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := WriteDescription(path, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err := ReadMeta(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Schedule == nil {
		t.Fatalf("schedule lost when clearing description")
	}
	if got.Description != "" {
		t.Fatalf("description not cleared: %q", got.Description)
	}
}

func TestWriteSchedule_doesNotLeaveTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.meta.json")
	if _, err := WriteSchedule(path, &ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}, time.Now()); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp file leaked: %s", e.Name())
		}
	}
}

func TestSpecsEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b *ScheduleSpec
		want bool
	}{
		{"both nil", nil, nil, true},
		{"one nil", nil, &ScheduleSpec{Kind: "hourly", EveryHours: 1}, false},
		{"different kinds", &ScheduleSpec{Kind: "hourly"}, &ScheduleSpec{Kind: "daily"}, false},
		{
			"hourly same",
			&ScheduleSpec{Kind: "hourly", EveryHours: 3, Minute: 0},
			&ScheduleSpec{Kind: "hourly", EveryHours: 3, Minute: 0},
			true,
		},
		{
			"hourly different everyHours",
			&ScheduleSpec{Kind: "hourly", EveryHours: 3, Minute: 0},
			&ScheduleSpec{Kind: "hourly", EveryHours: 6, Minute: 0},
			false,
		},
		{
			"daily reordered days",
			&ScheduleSpec{Kind: "daily", Days: []Weekday{Mon, Tue, Wed}, Hour: 9, Minute: 0},
			&ScheduleSpec{Kind: "daily", Days: []Weekday{Wed, Mon, Tue}, Hour: 9, Minute: 0},
			true,
		},
		{
			"daily different days",
			&ScheduleSpec{Kind: "daily", Days: []Weekday{Mon, Tue, Wed}, Hour: 9, Minute: 0},
			&ScheduleSpec{Kind: "daily", Days: []Weekday{Mon, Tue, Thu}, Hour: 9, Minute: 0},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SpecsEqual(tc.a, tc.b); got != tc.want {
				t.Fatalf("SpecsEqual(%+v, %+v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
