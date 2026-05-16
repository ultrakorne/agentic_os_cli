package main

import (
	"strings"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// agentRecord builds the JSON payload for a single agent. Both `aos list`
// (array of records) and `aos describe` (single record) share this shape so
// clients can write one parser. `meta` is taken as a parameter so callers can
// pass an updated AgentMeta after a write without re-scanning.
func agentRecord(a scheduler.Agent, meta scheduler.AgentMeta) map[string]any {
	rec := map[string]any{
		"id":         a.ID,
		"section":    a.Section,
		"scriptPath": a.ScriptPath,
	}
	if meta.Schedule != nil {
		rec["schedule"] = meta.Schedule
		if cronExpr, err := scheduler.CompileToCron(*meta.Schedule); err == nil {
			rec["cron"] = cronExpr
		}
	}
	if meta.ScheduledAt != "" {
		rec["scheduledAt"] = meta.ScheduledAt
	}
	if meta.Title != "" {
		rec["title"] = meta.Title
	}
	if meta.Description != "" {
		rec["description"] = meta.Description
	}
	if len(a.Warnings) > 0 {
		rec["warnings"] = a.Warnings
	}
	return rec
}
