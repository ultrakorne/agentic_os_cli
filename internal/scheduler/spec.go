package scheduler

import (
	"encoding/json"
	"fmt"

	"github.com/ultrakorne/aos_cli/internal/scheduler/schedspec"
)

// Type aliases re-export the leaf schedspec package so existing callers
// (cmd/aos, tests) keep using `scheduler.ScheduleSpec` and `scheduler.Mon`.
type Weekday = schedspec.Weekday
type ScheduleSpec = schedspec.ScheduleSpec

const (
	Sun = schedspec.Sun
	Mon = schedspec.Mon
	Tue = schedspec.Tue
	Wed = schedspec.Wed
	Thu = schedspec.Thu
	Fri = schedspec.Fri
	Sat = schedspec.Sat
)

// AgentMeta mirrors the .meta.json sidecar shape.
type AgentMeta struct {
	Schedule    *ScheduleSpec `json:"schedule,omitempty"`
	ScheduledAt string        `json:"scheduledAt,omitempty"`
	Title       string        `json:"title,omitempty"`
	Description string        `json:"description,omitempty"`
}

// ParseMeta parses a .meta.json sidecar; non-object content degrades to empty.
func ParseMeta(data []byte) AgentMeta {
	if len(data) == 0 {
		return AgentMeta{}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return AgentMeta{}
	}
	var m AgentMeta
	_ = json.Unmarshal(data, &m)
	return m
}

// ValidateSchedule sanity-checks a ScheduleSpec for the same conditions
// CompileToCron used to enforce. Used by `aos schedule` so the CLI rejects
// nonsense before persisting it.
func ValidateSchedule(s ScheduleSpec) error {
	switch s.Kind {
	case "hourly":
		if s.EveryHours < 1 || s.EveryHours > 12 {
			return fmt.Errorf("hourly.everyHours must be 1..12, got %d", s.EveryHours)
		}
		if s.Minute < 0 || s.Minute > 59 {
			return fmt.Errorf("hourly.minute must be 0..59, got %d", s.Minute)
		}
	case "daily":
		if len(s.Days) == 0 {
			return fmt.Errorf("daily.days must include at least one weekday")
		}
		if s.Hour < 0 || s.Hour > 23 {
			return fmt.Errorf("daily.hour must be 0..23, got %d", s.Hour)
		}
		if s.Minute < 0 || s.Minute > 59 {
			return fmt.Errorf("daily.minute must be 0..59, got %d", s.Minute)
		}
		for _, d := range s.Days {
			if _, ok := schedspec.WeekdayToCron[d]; !ok {
				return fmt.Errorf("unknown weekday %q", d)
			}
		}
	default:
		return fmt.Errorf("unknown schedule kind %q", s.Kind)
	}
	return nil
}
