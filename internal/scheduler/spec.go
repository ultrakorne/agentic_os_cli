package scheduler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Weekday string

const (
	Sun Weekday = "sun"
	Mon Weekday = "mon"
	Tue Weekday = "tue"
	Wed Weekday = "wed"
	Thu Weekday = "thu"
	Fri Weekday = "fri"
	Sat Weekday = "sat"
)

var weekdayToCron = map[Weekday]int{
	Sun: 0, Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6,
}

// ScheduleSpec is a tagged union: kind=hourly|daily.
type ScheduleSpec struct {
	Kind       string    `json:"kind"`
	EveryHours int       `json:"everyHours,omitempty"`
	Days       []Weekday `json:"days,omitempty"`
	Hour       int       `json:"hour,omitempty"`
	Minute     int       `json:"minute"`
}

// AgentMeta mirrors the .meta.json sidecar shape (only fields we care about).
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

func CompileToCron(s ScheduleSpec) (string, error) {
	switch s.Kind {
	case "hourly":
		if s.EveryHours < 1 || s.EveryHours > 12 {
			return "", fmt.Errorf("hourly.everyHours must be 1..12, got %d", s.EveryHours)
		}
		if s.Minute < 0 || s.Minute > 59 {
			return "", fmt.Errorf("hourly.minute must be 0..59, got %d", s.Minute)
		}
		hourField := "*"
		if s.EveryHours != 1 {
			hourField = fmt.Sprintf("*/%d", s.EveryHours)
		}
		return fmt.Sprintf("%d %s * * *", s.Minute, hourField), nil
	case "daily":
		if len(s.Days) == 0 {
			return "", fmt.Errorf("daily.days must include at least one weekday")
		}
		if s.Hour < 0 || s.Hour > 23 {
			return "", fmt.Errorf("daily.hour must be 0..23, got %d", s.Hour)
		}
		if s.Minute < 0 || s.Minute > 59 {
			return "", fmt.Errorf("daily.minute must be 0..59, got %d", s.Minute)
		}
		seen := map[int]struct{}{}
		nums := make([]int, 0, len(s.Days))
		for _, d := range s.Days {
			n, ok := weekdayToCron[d]
			if !ok {
				return "", fmt.Errorf("unknown weekday %q", d)
			}
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			nums = append(nums, n)
		}
		sort.Ints(nums)
		parts := make([]string, len(nums))
		for i, n := range nums {
			parts[i] = fmt.Sprintf("%d", n)
		}
		return fmt.Sprintf("%d %d * * %s", s.Minute, s.Hour, strings.Join(parts, ",")), nil
	default:
		return "", fmt.Errorf("unknown schedule kind %q", s.Kind)
	}
}
