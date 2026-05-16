package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/crontab"
	"github.com/ultrakorne/aos_cli/internal/logtrim"
	"github.com/ultrakorne/aos_cli/internal/runsgc"
	"github.com/ultrakorne/aos_cli/internal/runtime"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Reconcile agent schedules and runtime into the user's crontab",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := RunRefresh()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if JSONOutput() {
			if jerr := printJSON(s); jerr != nil {
				fmt.Fprintln(os.Stderr, jerr)
				os.Exit(1)
			}
			return
		}
		banner("refresh")
		printRefreshHuman(s)
	},
}

// printRefreshHuman renders the refresh summary as a key/value block, color
// coding the runtime-health fields so a degraded install (missing wrapper,
// cron daemon down, etc.) stands out at a glance.
func printRefreshHuman(s RefreshSummary) {
	healthStyle := func(v string) *lipgloss.Style {
		switch {
		case v == "ok" || v == "wrote" || v == "unchanged" || v == "trimmed" || v == "untouched":
			st := lipgloss.NewStyle().Foreground(colorSuccess)
			return &st
		case strings.HasPrefix(v, "swept:"):
			st := lipgloss.NewStyle().Foreground(colorSuccess)
			return &st
		case strings.HasPrefix(v, "skipped"):
			st := styleWarn
			return &st
		case v == "missing" || v == "down":
			st := styleErr
			return &st
		default:
			return nil
		}
	}
	printKV([]kvRow{
		{Key: "agents", Value: fmt.Sprintf("%d", s.Agents)},
		{Key: "scheduled", Value: fmt.Sprintf("%d", s.Scheduled)},
		{Key: "issues", Value: fmt.Sprintf("%d", s.Issues), Style: issueStyle(s.Issues)},
		{Key: "cron", Value: s.Cron, Style: healthStyle(s.Cron)},
		{Key: "wrapper", Value: s.Wrapper, Style: healthStyle(s.Wrapper)},
		{Key: "python3", Value: s.Python3, Style: healthStyle(s.Python3)},
		{Key: "daemon", Value: s.Daemon, Style: healthStyle(s.Daemon)},
		{Key: "log", Value: s.Log, Style: healthStyle(s.Log)},
		{Key: "runs", Value: s.Runs, Style: healthStyle(s.Runs)},
	})
}

func issueStyle(n int) *lipgloss.Style {
	if n == 0 {
		return nil
	}
	st := styleWarn
	return &st
}

type RefreshSummary struct {
	Agents    int    `json:"agents"`
	Scheduled int    `json:"scheduled"`
	Issues    int    `json:"issues"`
	Cron      string `json:"cron"`    // wrote | unchanged | skipped:<reason>
	Wrapper   string `json:"wrapper"` // ok | missing
	Python3   string `json:"python3"` // ok | missing
	Daemon    string `json:"daemon"`  // ok | down | unknown
	Log       string `json:"log"`     // trimmed | untouched
	Runs      string `json:"runs"`    // untouched | swept:<n> | skipped:<reason>
}

func (s RefreshSummary) OneLine() string {
	return fmt.Sprintf(
		"aos refresh agents=%d scheduled=%d issues=%d cron=%s wrapper=%s python3=%s daemon=%s log=%s runs=%s",
		s.Agents, s.Scheduled, s.Issues, s.Cron, s.Wrapper, s.Python3, s.Daemon, s.Log, s.Runs,
	)
}

// RunRefresh executes the refresh pipeline in-process. init.go calls this
// directly after writing the config so we don't shell out to ourselves.
func RunRefresh() (RefreshSummary, error) {
	sum := RefreshSummary{Cron: "skipped:unknown", Wrapper: "missing", Python3: "missing", Daemon: "unknown", Log: "untouched", Runs: "untouched"}

	cfg, err := config.Load()
	if err != nil {
		return sum, fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return sum, fmt.Errorf("aos not initialized — run `aos init <path>` first")
	}
	if st, err := os.Stat(cfg.AosHome); err != nil || !st.IsDir() {
		return sum, fmt.Errorf("aos_home %q does not exist or is not a directory", cfg.AosHome)
	}

	wrapperPath := filepath.Join(cfg.AosHome, "wrapper.sh")
	if runtime.FileExists(wrapperPath) && runtime.IsExecutable(wrapperPath) {
		sum.Wrapper = "ok"
	}
	if runtime.HasBin("python3") {
		sum.Python3 = "ok"
	}
	hasCrontab := runtime.HasBin("crontab")
	if running, err := runtime.CronDaemonRunning(); err == nil {
		if running {
			sum.Daemon = "ok"
		} else {
			sum.Daemon = "down"
		}
	} else {
		sum.Daemon = "unknown"
	}
	scan, err := scheduler.ScanAgents(filepath.Join(cfg.AosHome, "agents"))
	if err != nil {
		return sum, fmt.Errorf("scan agents: %w", err)
	}
	sum.Agents = len(scan.Agents)
	sum.Issues = len(scan.Issues)

	// Rebuild <aos_home>/misses/ so the dashboard's view reflects the agents
	// it just saw. Failure is non-fatal — the cron block is still the more
	// important thing to reconcile.
	if _, err := scheduler.SyncMissesDir(cfg.AosHome, scan.Agents, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "warn: %v\n", err)
	}

	entries := make([]crontab.Entry, 0)
	for _, a := range scan.Agents {
		if a.Meta.Schedule == nil {
			continue
		}
		if len(a.Warnings) > 0 {
			// A warned agent (e.g. not-executable) shouldn't enter the
			// managed crontab block — cron would fire a script that can't
			// run. Surface the count so a human reading the summary can see
			// why a scheduled agent isn't showing up under cron.
			sum.Issues++
			continue
		}
		expr, err := scheduler.CompileToCron(*a.Meta.Schedule)
		if err != nil {
			sum.Issues++
			continue
		}
		entries = append(entries, crontab.Entry{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			Expression: expr,
		})
	}
	sum.Scheduled = len(entries)

	if hasCrontab && sum.Wrapper == "ok" && sum.Python3 == "ok" {
		aosBin, err := runtime.AosBinaryPath()
		if err != nil {
			sum.Cron = "skipped:" + sanitize(err.Error())
			return sum, nil
		}
		tickCmd := crontab.BuildTickCommand(aosBin, cfg.AosHome)
		result, err := crontab.SyncCrontab(crontab.SyncArgs{
			Entries:     entries,
			WrapperPath: wrapperPath,
			DataDir:     cfg.AosHome,
			TickCommand: tickCmd,
		})
		if err != nil {
			sum.Cron = "skipped:" + sanitize(err.Error())
		} else if result.Conflict {
			sum.Cron = "skipped:conflict"
		} else if result.Wrote {
			sum.Cron = "wrote"
		} else {
			sum.Cron = "unchanged"
		}
	} else {
		reason := []string{}
		if !hasCrontab {
			reason = append(reason, "no-crontab-bin")
		}
		if sum.Wrapper != "ok" {
			reason = append(reason, "no-wrapper")
		}
		if sum.Python3 != "ok" {
			reason = append(reason, "no-python3")
		}
		sum.Cron = "skipped:" + strings.Join(reason, ",")
	}

	trimmed, err := logtrim.Trim(filepath.Join(cfg.AosHome, "tick.log"), logtrim.DefaultMaxBytes, logtrim.DefaultKeepBytes)
	if err == nil && trimmed {
		sum.Log = "trimmed"
	}

	runsRes, err := runsgc.Sweep(filepath.Join(cfg.AosHome, "runs"), cfg.EffectiveRunsHardCap())
	if err != nil {
		sum.Runs = "skipped:" + sanitize(err.Error())
	} else if runsRes.Deleted > 0 {
		sum.Runs = fmt.Sprintf("swept:%d", runsRes.Deleted)
	}

	return sum, nil
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
