// Package schedspec holds the agent schedule type. Lives in its own leaf
// package so both the scheduler core and the platform-native backends can
// import it without cycling.
package schedspec

import "time"

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

// WeekdayToCron maps Weekday values to their crontab/launchd integer.
// 0=Sun..6=Sat — matches launchd's `Weekday` field.
var WeekdayToCron = map[Weekday]int{
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

// NextSlot returns the earliest scheduled instant strictly after `after`,
// evaluated in after.Location(). Returns zero time if the schedule is
// invalid.
func (s ScheduleSpec) NextSlot(after time.Time) time.Time {
	loc := after.Location()
	switch s.Kind {
	case "hourly":
		if s.EveryHours < 1 || s.EveryHours > 12 || s.Minute < 0 || s.Minute > 59 {
			return time.Time{}
		}
		cur := time.Date(after.Year(), after.Month(), after.Day(), after.Hour(), s.Minute, 0, 0, loc)
		for i := 0; i < 48; i++ {
			if isHourAligned(cur.Hour(), s.EveryHours) && cur.After(after) {
				return cur
			}
			cur = cur.Add(time.Hour)
		}
		return time.Time{}
	case "daily":
		if len(s.Days) == 0 || s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 {
			return time.Time{}
		}
		allowed := map[time.Weekday]bool{}
		for _, d := range s.Days {
			n, ok := WeekdayToCron[d]
			if !ok {
				return time.Time{}
			}
			allowed[time.Weekday(n)] = true
		}
		day := time.Date(after.Year(), after.Month(), after.Day(), s.Hour, s.Minute, 0, 0, loc)
		for i := 0; i < 8; i++ {
			candidate := day.AddDate(0, 0, i)
			if !allowed[candidate.Weekday()] {
				continue
			}
			if candidate.After(after) {
				return candidate
			}
		}
		return time.Time{}
	default:
		return time.Time{}
	}
}

func isHourAligned(h, every int) bool {
	if every <= 1 {
		return true
	}
	return h%every == 0
}
