package scheduler

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// WriteSchedule mutates the sidecar at metaPath to set or clear the schedule.
// When schedule is nil, both schedule and scheduledAt are cleared. When set,
// scheduledAt is bumped only if the spec actually changes (or if no
// scheduledAt is recorded yet), matching the contract from
// src/main/scheduler/agent-meta-store.ts. A meta that becomes fully empty is
// removed from disk rather than left as a `{}` stub.
func WriteSchedule(metaPath string, schedule *ScheduleSpec, now time.Time) (AgentMeta, error) {
	return mutateMeta(metaPath, func(cur AgentMeta) AgentMeta {
		if schedule == nil {
			cur.Schedule = nil
			cur.ScheduledAt = ""
			return cur
		}
		if !SpecsEqual(cur.Schedule, schedule) || cur.ScheduledAt == "" {
			cur.ScheduledAt = now.UTC().Format(time.RFC3339Nano)
		}
		cur.Schedule = schedule
		return cur
	})
}

// WriteDescription mutates the sidecar at metaPath to set the description.
// An empty string clears the field; if no other fields remain, the file is
// removed.
func WriteDescription(metaPath, description string) (AgentMeta, error) {
	return mutateMeta(metaPath, func(cur AgentMeta) AgentMeta {
		cur.Description = description
		return cur
	})
}

// ReadMeta loads the sidecar at path. A missing or unparseable file degrades
// to an empty AgentMeta with no error — the caller treats "no sidecar" and
// "empty sidecar" identically.
func ReadMeta(path string) (AgentMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AgentMeta{}, nil
		}
		return AgentMeta{}, err
	}
	return ParseMeta(data), nil
}

// SpecsEqual reports whether two schedule specs encode the same trigger. Day
// lists are compared as sets so reordering doesn't count as a change.
func SpecsEqual(a, b *ScheduleSpec) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case "hourly":
		return a.EveryHours == b.EveryHours && a.Minute == b.Minute
	case "daily":
		if a.Hour != b.Hour || a.Minute != b.Minute {
			return false
		}
		if len(a.Days) != len(b.Days) {
			return false
		}
		seen := map[Weekday]struct{}{}
		for _, d := range a.Days {
			seen[d] = struct{}{}
		}
		for _, d := range b.Days {
			if _, ok := seen[d]; !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func mutateMeta(path string, fn func(AgentMeta) AgentMeta) (AgentMeta, error) {
	cur, err := ReadMeta(path)
	if err != nil {
		return AgentMeta{}, err
	}
	next := fn(cur)
	if err := writeMetaJSON(path, next); err != nil {
		return AgentMeta{}, err
	}
	return next, nil
}

func writeMetaJSON(path string, meta AgentMeta) error {
	if isEmptyMeta(meta) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func isEmptyMeta(m AgentMeta) bool {
	return m.Schedule == nil && m.ScheduledAt == "" && m.Title == "" && m.Description == ""
}
