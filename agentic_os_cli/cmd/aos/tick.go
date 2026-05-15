package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/crontab"
	"github.com/ultrakorne/aos_cli/internal/runtime"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var tickCmd = &cobra.Command{
	Use:   "tick",
	Short: "Run one scheduler tick: scan, detect missed runs, log a summary",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runTick(); err != nil {
			fmt.Fprintf(os.Stderr, "[tick] failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func runTick() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return fmt.Errorf("aos not initialized")
	}

	scan, err := scheduler.ScanAgents(filepath.Join(cfg.AosHome, "agents"))
	if err != nil {
		return fmt.Errorf("scan agents: %w", err)
	}

	scheduled := 0
	for _, a := range scan.Agents {
		if a.Meta.Schedule != nil {
			scheduled++
		}
	}

	runs, err := scheduler.LoadRuns(filepath.Join(cfg.AosHome, "runs"))
	if err != nil {
		return fmt.Errorf("load runs: %w", err)
	}
	missed := scheduler.DetectMissed(scan.Agents, runs, scheduler.DetectOpts{Now: time.Now()})

	state := crontabState(cfg.AosHome, scan.Agents)

	line := fmt.Sprintf("[tick] %s scripts=%d scheduled=%d missed=%d crontab=%s\n",
		time.Now().UTC().Format(time.RFC3339), len(scan.Agents), scheduled, len(missed), state)
	if err := appendLog(filepath.Join(cfg.AosHome, "tick.log"), line); err != nil {
		return fmt.Errorf("write tick.log: %w", err)
	}
	// Mirror tick.ts's stdout summary so the cron tail makes sense to a human.
	fmt.Print(line)
	return nil
}

// crontabState returns one of: managed | empty | conflict | drift | error(<msg>).
// "drift" means: a managed block exists, but rebuilding it from the live
// agents would produce a different block. The user should run `aos refresh`.
func crontabState(dataDir string, agents []scheduler.Agent) string {
	if !runtime.HasBin("crontab") {
		return "error(no-crontab-bin)"
	}
	text, err := crontab.ReadCrontab()
	if err != nil {
		return "error(" + sanitize(err.Error()) + ")"
	}
	ex := crontab.ExtractManaged(text)
	if ex.Conflict {
		return "conflict"
	}
	if !ex.HasMarker {
		if len(scheduledOnly(agents)) == 0 {
			return "empty"
		}
		return "drift"
	}

	wrapperPath := filepath.Join(dataDir, "wrapper.sh")
	entries := make([]crontab.Entry, 0)
	for _, a := range scheduledOnly(agents) {
		expr, err := scheduler.CompileToCron(*a.Meta.Schedule)
		if err != nil {
			continue
		}
		entries = append(entries, crontab.Entry{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			Expression: expr,
		})
	}
	aosBin, err := runtime.AosBinaryPath()
	if err != nil {
		return "error(" + sanitize(err.Error()) + ")"
	}
	expectedBlock := crontab.BuildManagedBlock(entries, wrapperPath, dataDir, crontab.BuildTickCommand(aosBin, dataDir))
	actualBlock := crontab.BeginMarker + "\n" + strings.Join(ex.Managed, "\n") + "\n" + crontab.EndMarker
	if actualBlock == expectedBlock {
		return "managed"
	}
	return "drift"
}

func scheduledOnly(agents []scheduler.Agent) []scheduler.Agent {
	out := make([]scheduler.Agent, 0)
	for _, a := range agents {
		if a.Meta.Schedule != nil {
			out = append(out, a)
		}
	}
	return out
}

func appendLog(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func init() {
	rootCmd.AddCommand(tickCmd)
}
