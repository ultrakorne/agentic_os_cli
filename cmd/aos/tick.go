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

// TickSummary is the structured form of one scheduler tick. The cron-tail
// line in tick.log keeps its historical "[tick] ..." shape (existing log
// readers depend on the prefix), but stdout switches between this struct and
// a styled block depending on --json.
//
// Missed counts miss records *newly written this tick*, not currently
// outstanding. Most ticks emit 0; the count goes positive only when a new
// uncovered slot is detected for an agent that didn't already have a record
// for it.
//
// Catchups counts catch-up wrappers *spawned this tick* — one per agent
// whose latest run was status="missed". The trigger condition is strictly
// "latest is missed", so once the catch-up writes its running/success/error
// record the agent is no longer a candidate. Off by default when
// catchup_enabled=false in config.toml.
type TickSummary struct {
	Timestamp string `json:"timestamp"`
	Scripts   int    `json:"scripts"`
	Scheduled int    `json:"scheduled"`
	Missed    int    `json:"missed"`
	Catchups  int    `json:"catchups"`
	Crontab   string `json:"crontab"`
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

	now := time.Now()
	missed, runs, missesErr := scheduler.RecordMissedRuns(cfg.AosHome, scan.Agents, now)
	if missesErr != nil {
		// Don't fail the tick — the runs/cron side of the world is still
		// authoritative even if a miss record didn't land this round.
		fmt.Fprintf(os.Stderr, "[tick] record missed runs: %v\n", missesErr)
	}

	catchups := 0
	if cfg.EffectiveCatchupEnabled() {
		// Reuse the runs slice RecordMissedRuns returned — it already
		// reflects this tick's writes, so fireCatchups doesn't need a
		// second LoadRuns of the (potentially 2000-entry) runs/ dir.
		fired, err := fireCatchups(cfg.AosHome, scan.Agents, runs)
		if err != nil {
			// Spawn failures don't fail the tick — same posture as miss
			// recording. Operators see them on stderr; the next tick retries.
			fmt.Fprintf(os.Stderr, "[tick] fire catch-ups: %v\n", err)
		}
		catchups = fired
	}

	state := crontabState(cfg.AosHome, scan.Agents)
	summary := TickSummary{
		Timestamp: now.UTC().Format(time.RFC3339),
		Scripts:   len(scan.Agents),
		Scheduled: scheduled,
		Missed:    len(missed),
		Catchups:  catchups,
		Crontab:   state,
	}

	// The on-disk log keeps its historical bracketed shape; tail consumers
	// (the dashboard's tick.log viewer, ad-hoc grep) parse that line.
	logLine := fmt.Sprintf("[tick] %s scripts=%d scheduled=%d missed=%d catchups=%d crontab=%s\n",
		summary.Timestamp, summary.Scripts, summary.Scheduled, summary.Missed, summary.Catchups, summary.Crontab)
	if err := appendLog(filepath.Join(cfg.AosHome, "tick.log"), logLine); err != nil {
		return fmt.Errorf("write tick.log: %w", err)
	}

	if JSONOutput() {
		return printJSON(summary)
	}
	// Match the log's terse single-line shape — when cron tails this verb,
	// the operator wants the same string they'd grep for in tick.log.
	fmt.Print(logLine)
	return nil
}

// fireCatchups inspects per-agent latest run state and spawns wrapper.sh with
// AGENTIC_OS_TRIGGER=catch-up for every agent whose latest run is missed.
// Returns the number of wrappers actually spawned. Each spawn failure is
// logged to stderr but does not short-circuit the loop — one broken agent
// shouldn't block catch-ups for siblings.
//
// `runs` is the post-RecordMissedRuns view of the runs/ directory — pass it
// through from tick so a 2000-file directory isn't walked twice per tick.
func fireCatchups(aosHome string, agents []scheduler.Agent, runs []scheduler.JobRun) (int, error) {
	wrapperPath := filepath.Join(aosHome, "wrapper.sh")
	if !runtime.FileExists(wrapperPath) || !runtime.IsExecutable(wrapperPath) {
		// Mirrors aos run's posture: without a usable wrapper we can't spawn
		// anything. Treat as a soft error so the tick's other work still lands.
		return 0, fmt.Errorf("%s missing or not executable", wrapperPath)
	}
	candidates := scheduler.DetectCatchups(agents, runs)
	fired := 0
	for _, c := range candidates {
		err := scheduler.SpawnWrapperDetached(wrapperPath, scheduler.SpawnOpts{
			AosHome:    aosHome,
			ScheduleID: c.MissedSlot,
			AgentID:    c.AgentID,
			ScriptPath: c.ScriptPath,
			RunID:      scheduler.NewRunID(),
			Trigger:    "catch-up",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[tick] catch-up %s: %v\n", c.AgentID, err)
			continue
		}
		fired++
	}
	return fired, nil
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
