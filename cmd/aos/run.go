package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/runtime"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var runWaitFlag bool

var runCmd = &cobra.Command{
	Use:   "run <id>",
	Short: "Spawn a manual run of an agent in the background",
	Long: `Start a manual run of <id> by spawning wrapper.sh detached from this
process. Prints a Run stub (id, agentId, startedAt, status="running", ...) so
callers can record the new run id without waiting for the wrapper to finish;
the wrapper writes the final status to <aos_home>/runs/<run-id>.json.

The trigger is set to "manual" via AGENTIC_OS_TRIGGER so the on-disk record is
distinguishable from cron-driven runs. The run id is pre-generated here and
threaded as the wrapper's 5th argv, so the printed stub's id matches the file
the wrapper writes.

Pass --wait to block until the wrapper finishes; the stub still prints first,
then a progress/spinner shows on stderr while polling, then the .out bytes
print to stdout. With --json --wait the stub JSON prints first, then output.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRun,
}

func runRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	id := args[0]
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return errors.New("aos not initialized — run `aos init <path>` first")
	}

	agent, _, err := scheduler.FindAgentByID(filepath.Join(cfg.AosHome, "agents"), id)
	if err != nil {
		return err
	}

	wrapperPath := filepath.Join(cfg.AosHome, "wrapper.sh")
	if !runtime.FileExists(wrapperPath) {
		return fmt.Errorf("%s missing — run `aos init <path>`", wrapperPath)
	}
	if !runtime.IsExecutable(wrapperPath) {
		return fmt.Errorf("%s is not executable", wrapperPath)
	}

	store := scheduler.NewFileRunStore(cfg.AosHome)
	runID := store.NewID()
	now := time.Now()
	startedAt := scheduler.FormatRunTimestamp(now)
	estimateDur := time.Duration(-1)
	estimateMillis := int64(-1)
	if estimate, ok, err := store.EstimateDuration(agent.ID, 10); err != nil {
		return fmt.Errorf("estimate run duration: %w", err)
	} else if ok {
		estimateDur = estimate
		estimateMillis = estimate.Truncate(time.Millisecond).Milliseconds()
	}

	if err := scheduler.SpawnWrapperDetached(wrapperPath, scheduler.SpawnOpts{
		AosHome:    cfg.AosHome,
		AgentID:    agent.ID,
		ScriptPath: agent.ScriptPath,
		RunID:      runID,
		Trigger:    "manual",
	}); err != nil {
		return fmt.Errorf("spawn wrapper: %w", err)
	}

	stub := runStub(runID, agent.ID, startedAt, estimateMillis)
	if JSONOutput() {
		if err := printJSON(stub); err != nil {
			return err
		}
	} else if err := printRunHuman(stub); err != nil {
		return err
	}

	if !runWaitFlag {
		return nil
	}
	return waitFlow(store, runID, agent.ID, now, estimateDur)
}

// runStub mirrors the Run JSON shape the renderer expects, plus an
// estimate field for the launcher response. The wrapper overwrites the on-disk
// record once it finishes; this stub only documents what the caller can poll
// for.
func runStub(runID, agentID, startedAt string, estimateMillis ...int64) map[string]any {
	estimate := int64(-1)
	if len(estimateMillis) > 0 {
		estimate = estimateMillis[0]
	}
	return map[string]any{
		"id":         runID,
		"agentId":      agentID,
		"scheduleId": nil,
		"trigger":    "manual",
		"startedAt":  startedAt,
		"endedAt":    nil,
		"status":     "running",
		"output":     "",
		"error":      nil,
		"exitCode":   nil,
		"outputPath": runID + ".out",
		"estimate":   estimate,
	}
}

func printRunHuman(stub map[string]any) error {
	banner("run " + fmt.Sprint(stub["agentId"]))
	statusS := statusStyle(fmt.Sprint(stub["status"]))
	printKV([]kvRow{
		{Key: "run", Value: fmt.Sprint(stub["id"])},
		{Key: "status", Value: fmt.Sprint(stub["status"]), Style: &statusS},
		{Key: "estimate", Value: estimateString(stub["estimate"])},
		{Key: "startedAt", Value: formatStartedAt(fmt.Sprint(stub["startedAt"]))},
	})
	return nil
}

func estimateString(v any) string {
	ms, ok := v.(int64)
	if !ok || ms < 0 {
		return "none"
	}
	// Round to 100 ms (~1 decimal of a second) so the stub prints clean values
	// like "1.2s" or "1m23.5s" instead of full ns-precision output.
	return (time.Duration(ms) * time.Millisecond).Round(100 * time.Millisecond).String()
}

func init() {
	runCmd.Flags().BoolVar(&runWaitFlag, "wait", false, "block until the run finishes, then print its .out to stdout")
	rootCmd.AddCommand(runCmd)
}
