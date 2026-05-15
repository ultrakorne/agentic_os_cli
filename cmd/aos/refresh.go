package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/crontab"
	"github.com/ultrakorne/aos_cli/internal/logtrim"
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
		fmt.Println(s.OneLine())
	},
}

type RefreshSummary struct {
	Agents    int
	Scheduled int
	Issues    int
	Cron      string // wrote | unchanged | skipped:<reason>
	Wrapper   string // ok | missing
	Python3   string // ok | missing
	Daemon    string // ok | down | unknown
	AosPath   string // ok | warn
	Log       string // trimmed | untouched
}

func (s RefreshSummary) OneLine() string {
	return fmt.Sprintf(
		"aos refresh agents=%d scheduled=%d issues=%d cron=%s wrapper=%s python3=%s daemon=%s aos_on_cron_path=%s log=%s",
		s.Agents, s.Scheduled, s.Issues, s.Cron, s.Wrapper, s.Python3, s.Daemon, s.AosPath, s.Log,
	)
}

// RunRefresh executes the refresh pipeline in-process. init.go calls this
// directly after writing the config so we don't shell out to ourselves.
func RunRefresh() (RefreshSummary, error) {
	sum := RefreshSummary{Cron: "skipped:unknown", Wrapper: "missing", Python3: "missing", Daemon: "unknown", AosPath: "warn", Log: "untouched"}

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
	if runtime.AosOnCronPath() {
		sum.AosPath = "ok"
	}

	scan, err := scheduler.ScanAgents(filepath.Join(cfg.AosHome, "agents"))
	if err != nil {
		return sum, fmt.Errorf("scan agents: %w", err)
	}
	sum.Agents = len(scan.Agents)
	sum.Issues = len(scan.Issues)

	entries := make([]crontab.Entry, 0)
	for _, a := range scan.Agents {
		if a.Meta.Schedule == nil {
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
		tickCmd := crontab.BuildTickCommand(cfg.AosHome)
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

	return sum, nil
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
