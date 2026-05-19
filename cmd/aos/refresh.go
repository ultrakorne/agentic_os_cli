package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Reconcile agent schedules and runtime into the user's crontab",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runRefresh()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		emitWarnings(out.Warnings)
		if JSONOutput() {
			if jerr := printJSON(out); jerr != nil {
				fmt.Fprintln(os.Stderr, jerr)
				os.Exit(1)
			}
			return
		}
		banner("refresh")
		printRefreshHuman(out)
	},
}

// runRefresh loads config and delegates to scheduler.Refresh. The in-process
// callers (init, schedule, the TUI popup) call this same helper so flag/env
// resolution stays in one place.
func runRefresh() (scheduler.RefreshOutcome, error) {
	cfg, err := config.Load()
	if err != nil {
		return scheduler.RefreshOutcome{}, fmt.Errorf("load config: %w", err)
	}
	return scheduler.Refresh(scheduler.RefreshDeps{Cfg: cfg, Now: time.Now()})
}

// emitWarnings prints each non-fatal step error to stderr so operators see
// them next to whatever else cron is logging. The structured outcome carries
// them in JSON too — this is just the human channel.
func emitWarnings(ws []string) {
	for _, w := range ws {
		fmt.Fprintf(os.Stderr, "warn: %s\n", w)
	}
}

// printRefreshHuman renders the refresh outcome as a key/value block, color
// coding the runtime-health fields so a degraded install (missing wrapper,
// cron daemon down, etc.) stands out at a glance.
func printRefreshHuman(s scheduler.RefreshOutcome) {
	printKV([]kvRow{
		{Key: "agents", Value: fmt.Sprintf("%d", s.Agents)},
		{Key: "scheduled", Value: fmt.Sprintf("%d", s.Scheduled)},
		{Key: "issues", Value: fmt.Sprintf("%d", s.Issues), Style: issueStyle(s.Issues)},
		{Key: "cron", Value: cronDisplay(s.Cron), Style: cronStyle(s.Cron)},
		{Key: "wrapper", Value: string(s.Wrapper), Style: healthStyle(string(s.Wrapper))},
		{Key: "python3", Value: string(s.Python3), Style: healthStyle(string(s.Python3))},
		{Key: "daemon", Value: string(s.Daemon), Style: healthStyle(string(s.Daemon))},
		{Key: "log", Value: logDisplay(s.Log), Style: logStyle(s.Log)},
		{Key: "runs", Value: runsDisplay(s.Runs), Style: runsStyle(s.Runs)},
	})
}

func cronDisplay(c scheduler.CronSyncOutcome) string {
	if len(c.Reasons) == 0 {
		return string(c.State)
	}
	return string(c.State) + ":" + strings.Join(c.Reasons, ",")
}

func cronStyle(c scheduler.CronSyncOutcome) *lipgloss.Style {
	switch c.State {
	case scheduler.CronWrote, scheduler.CronUnchanged:
		st := lipgloss.NewStyle().Foreground(colorSuccess)
		return &st
	case scheduler.CronSkipped:
		st := styleWarn
		return &st
	case scheduler.CronConflict:
		st := styleErr
		return &st
	}
	return nil
}

func logDisplay(l scheduler.LogTrimOutcome) string {
	if l.Trimmed {
		return "trimmed"
	}
	return "untouched"
}

func logStyle(l scheduler.LogTrimOutcome) *lipgloss.Style {
	st := lipgloss.NewStyle().Foreground(colorSuccess)
	_ = l
	return &st
}

func runsDisplay(r scheduler.RunsSweepOutcome) string {
	if r.Skipped != "" {
		return "skipped:" + r.Skipped
	}
	if r.Deleted > 0 {
		return fmt.Sprintf("swept:%d", r.Deleted)
	}
	return "untouched"
}

func runsStyle(r scheduler.RunsSweepOutcome) *lipgloss.Style {
	if r.Skipped != "" {
		st := styleWarn
		return &st
	}
	st := lipgloss.NewStyle().Foreground(colorSuccess)
	return &st
}

func healthStyle(v string) *lipgloss.Style {
	switch v {
	case "ok":
		st := lipgloss.NewStyle().Foreground(colorSuccess)
		return &st
	case "missing", "down":
		st := styleErr
		return &st
	case "unknown":
		st := styleWarn
		return &st
	}
	return nil
}

func issueStyle(n int) *lipgloss.Style {
	if n == 0 {
		return nil
	}
	st := styleWarn
	return &st
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
