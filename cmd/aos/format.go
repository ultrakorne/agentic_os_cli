package main

import (
	"strings"
	"time"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// formatStartedAt renders an ISO/RFC3339 timestamp as local "HH:MM:SS" for
// human output. JSON consumers still get the raw ISO string from the stub;
// this is purely a presentation helper for printKV / table rows. Falls back to
// the input on parse failure so we never swallow data.
func formatStartedAt(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.Local().Format("15:04:05")
}

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
