package main

import (
	"encoding/json"
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
the wrapper writes.`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

func runRun(cmd *cobra.Command, args []string) error {
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
	startedAt := isoMillisUTC(time.Now())

	if err := spawnWrapperDetached(wrapperPath, cfg.AosHome, agent.ID, agent.ScriptPath, runID); err != nil {
		return fmt.Errorf("spawn wrapper: %w", err)
	}

	stub := jobRunStub(runID, agent.ID, startedAt)
	if JSONOutput() {
		return printRunJSON(stub)
	}
	return printRunHuman(stub)
}

// jobRunStub mirrors the JobRun JSON shape the renderer expects
// (src/shared/scheduler.ts). The wrapper overwrites the on-disk record once it
// finishes; this stub only documents what the caller can poll for.
func jobRunStub(runID, agentID, startedAt string) map[string]any {
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
	}
}

func printRunJSON(stub map[string]any) error {
	buf, err := json.MarshalIndent(stub, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(buf))
	return nil
}

func printRunHuman(stub map[string]any) error {
	fmt.Printf("aos run id=%s run=%s status=%s startedAt=%s\n",
		stub["jobId"], stub["id"], stub["status"], stub["startedAt"])
	return nil
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
	rootCmd.AddCommand(runCmd)
}
