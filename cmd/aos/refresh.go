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
	Short: "Reconcile agent schedules and runtime into the platform scheduler",
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

func runRefresh() (scheduler.RefreshOutcome, error) {
	cfg, err := config.Load()
	if err != nil {
		return scheduler.RefreshOutcome{}, fmt.Errorf("load config: %w", err)
	}
	return scheduler.Refresh(scheduler.RefreshDeps{Cfg: cfg, Now: time.Now()})
}

func emitWarnings(ws []string) {
	for _, w := range ws {
		fmt.Fprintf(os.Stderr, "warn: %s\n", w)
	}
}

func printRefreshHuman(s scheduler.RefreshOutcome) {
	rows := []kvRow{
		{Key: "agents", Value: fmt.Sprintf("%d", s.Agents)},
		{Key: "scheduled", Value: fmt.Sprintf("%d", s.Scheduled)},
		{Key: "issues", Value: fmt.Sprintf("%d", s.Issues), Style: issueStyle(s.Issues)},
		{Key: "backend", Value: backendDisplay(s.Backend), Style: backendStyle(s.Backend)},
		{Key: "wrapper", Value: string(s.Wrapper), Style: healthStyle(string(s.Wrapper))},
		{Key: "python3", Value: string(s.Python3), Style: healthStyle(string(s.Python3))},
		{Key: "backendHealth", Value: string(s.BackendHealth), Style: healthStyle(string(s.BackendHealth))},
	}
	if s.LingerState != "" {
		rows = append(rows, kvRow{Key: "linger", Value: string(s.LingerState), Style: healthStyle(string(s.LingerState))})
	}
	rows = append(rows,
		kvRow{Key: "log", Value: logDisplay(s.Log), Style: logStyle(s.Log)},
		kvRow{Key: "runs", Value: runsDisplay(s.Runs), Style: runsStyle(s.Runs)},
	)
	printKV(rows)
}

func backendDisplay(b scheduler.BackendSyncOutcome) string {
	parts := []string{b.State}
	if len(b.Reasons) > 0 {
		parts = append(parts, strings.Join(b.Reasons, ","))
	}
	counts := []string{}
	if b.Wrote > 0 {
		counts = append(counts, fmt.Sprintf("wrote=%d", b.Wrote))
	}
	if b.Removed > 0 {
		counts = append(counts, fmt.Sprintf("removed=%d", b.Removed))
	}
	if b.Unchanged > 0 {
		counts = append(counts, fmt.Sprintf("unchanged=%d", b.Unchanged))
	}
	if len(counts) > 0 {
		parts = append(parts, "("+strings.Join(counts, " ")+")")
	}
	return strings.Join(parts, ":")
}

func backendStyle(b scheduler.BackendSyncOutcome) *lipgloss.Style {
	switch b.State {
	case "managed", "empty":
		st := lipgloss.NewStyle().Foreground(colorSuccess)
		return &st
	case "drift":
		st := styleWarn
		return &st
	case "skipped":
		st := styleWarn
		return &st
	}
	if len(b.Failed) > 0 {
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
	case "disabled":
		st := styleWarn
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
