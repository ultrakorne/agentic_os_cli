package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
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
process. Prints a JobRun stub (id, jobId, startedAt, status="running", ...) so
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

	runID := newRunID()
	now := time.Now()
	startedAt := isoMillisUTC(now)
	estimateDur := time.Duration(-1)
	estimateMillis := int64(-1)
	if estimate, ok, err := scheduler.EstimateRunDuration(filepath.Join(cfg.AosHome, "runs"), agent.ID, 10); err != nil {
		return fmt.Errorf("estimate run duration: %w", err)
	} else if ok {
		estimateDur = estimate
		estimateMillis = estimate.Truncate(time.Millisecond).Milliseconds()
	}

	if err := spawnWrapperDetached(wrapperPath, cfg.AosHome, agent.ID, agent.ScriptPath, runID); err != nil {
		return fmt.Errorf("spawn wrapper: %w", err)
	}

	stub := jobRunStub(runID, agent.ID, startedAt, estimateMillis)
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
	return waitFlow(filepath.Join(cfg.AosHome, "runs"), runID, agent.ID, now, estimateDur)
}

// jobRunStub mirrors the JobRun JSON shape the renderer expects, plus an
// estimate field for the launcher response. The wrapper overwrites the on-disk
// record once it finishes; this stub only documents what the caller can poll
// for.
func jobRunStub(runID, agentID, startedAt string, estimateMillis ...int64) map[string]any {
	estimate := int64(-1)
	if len(estimateMillis) > 0 {
		estimate = estimateMillis[0]
	}
	return map[string]any{
		"id":         runID,
		"jobId":      agentID,
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
	banner("run " + fmt.Sprint(stub["jobId"]))
	statusS := statusStyle(fmt.Sprint(stub["status"]))
	printKV([]kvRow{
		{Key: "run", Value: fmt.Sprint(stub["id"])},
		{Key: "status", Value: fmt.Sprint(stub["status"]), Style: &statusS},
		{Key: "estimate", Value: estimateString(stub["estimate"])},
		{Key: "startedAt", Value: fmt.Sprint(stub["startedAt"])},
	})
	return nil
}

func estimateString(v any) string {
	ms, ok := v.(int64)
	if !ok || ms < 0 {
		return "none"
	}
	return (time.Duration(ms) * time.Millisecond).String()
}

// spawnWrapperDetached starts wrapper.sh in a new session so it survives this
// process exiting. stdout/stderr/stdin default to /dev/null in os/exec when
// left nil, decoupling the wrapper from whatever shell `aos` was launched in.
func spawnWrapperDetached(wrapperPath, aosHome, agentID, scriptPath, runID string) error {
	cmd := exec.Command(wrapperPath, aosHome, "", agentID, scriptPath, runID)
	cmd.Env = append(os.Environ(), "AGENTIC_OS_TRIGGER=manual")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// isoMillisUTC mirrors wrapper.sh's iso_now: millisecond-precision UTC.
func isoMillisUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

var (
	runIDOnce sync.Once
	runIDRand *rand.Rand
)

// newRunID is the engine-side analogue of wrapper.sh's fallback id format
// (`<unix>-<pid>-<rand><rand>`). Threading a pre-generated id through the
// wrapper's 5th argv lets the stub returned to the renderer match the file
// name the wrapper writes.
func newRunID() string {
	runIDOnce.Do(func() {
		runIDRand = rand.New(rand.NewSource(time.Now().UnixNano()))
	})
	return fmt.Sprintf("%d-%d-%d%d",
		time.Now().Unix(), os.Getpid(),
		runIDRand.Int31(), runIDRand.Int31())
}

func init() {
	runCmd.Flags().BoolVar(&runWaitFlag, "wait", false, "block until the run finishes, then print its .out to stdout")
	rootCmd.AddCommand(runCmd)
}
