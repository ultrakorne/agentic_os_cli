package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var (
	runsAgentID string
	runsLimit   int
	runsShowOut bool
)

var runsCmd = &cobra.Command{
	Use:   "runs [run-id]",
	Short: "List recent runs, or show one by id",
	Long: `Without args: print recent runs sorted by start time, most recent first.

With one positional run id: print that run's record (use --json for the full
JobRun shape). Add --output to dump the .out file's contents instead of the
record, so you can pipe it: ` + "`aos runs <run-id> --output | less`" + `.

Runs are read from <aos_home>/runs/<run-id>.{json,out}; the wrapper writes
them, this command only reads.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRunsCmd,
}

func runRunsCmd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return errors.New("aos not initialized — run `aos init <path>` first")
	}
	runsDir := filepath.Join(cfg.AosHome, "runs")

	if len(args) == 1 {
		return showOneRun(runsDir, args[0])
	}
	return listAllRuns(runsDir)
}

func listAllRuns(runsDir string) error {
	runs, err := scheduler.ReadRuns(runsDir, runsAgentID, runsLimit)
	if err != nil {
		return fmt.Errorf("read runs: %w", err)
	}
	if JSONOutput() {
		buf, err := json.MarshalIndent(map[string]any{"runs": runs}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(buf))
		return nil
	}
	return printRunsHuman(runs)
}

func showOneRun(runsDir, runID string) error {
	if runsShowOut {
		data, err := scheduler.ReadRunOutput(runsDir, runID)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}
	run, err := scheduler.ReadRun(runsDir, runID)
	if err != nil {
		return err
	}
	if JSONOutput() {
		buf, err := json.MarshalIndent(run, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(buf))
		return nil
	}
	return printOneRunHuman(run)
}

func printRunsHuman(runs []scheduler.JobRun) error {
	if len(runs) == 0 {
		fmt.Println("(no runs)")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RUN-ID\tAGENT\tSTATUS\tTRIGGER\tSTARTED\tELAPSED\tEXIT")
	for _, r := range runs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.JobID, r.Status, r.Trigger,
			r.StartedAt, elapsedString(r), exitString(r))
	}
	return w.Flush()
}

func printOneRunHuman(r scheduler.JobRun) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	row := func(k string, v any) {
		fmt.Fprintf(w, "%s\t%v\n", k, v)
	}
	row("id", r.ID)
	row("agent", r.JobID)
	row("status", r.Status)
	row("trigger", r.Trigger)
	row("startedAt", r.StartedAt)
	if r.EndedAt != nil && *r.EndedAt != "" {
		row("endedAt", *r.EndedAt)
	}
	row("elapsed", elapsedString(r))
	row("exit", exitString(r))
	if r.OutputPath != nil && *r.OutputPath != "" {
		row("outputPath", *r.OutputPath)
	}
	if r.Error != nil && *r.Error != "" {
		row("error", *r.Error)
	}
	return w.Flush()
}

// elapsedString returns "..." while the run is still in flight (no endedAt)
// and a ms-precision duration once the wrapper has recorded the end. Falls
// back to "-" if either timestamp won't parse — better than a panic.
func elapsedString(r scheduler.JobRun) string {
	if r.EndedAt == nil || *r.EndedAt == "" {
		return "..."
	}
	start, err1 := time.Parse(time.RFC3339Nano, r.StartedAt)
	end, err2 := time.Parse(time.RFC3339Nano, *r.EndedAt)
	if err1 != nil || err2 != nil {
		return "-"
	}
	return end.Sub(start).Truncate(time.Millisecond).String()
}

func exitString(r scheduler.JobRun) string {
	if r.ExitCode == nil {
		return "-"
	}
	return fmt.Sprint(*r.ExitCode)
}

func init() {
	runsCmd.Flags().StringVar(&runsAgentID, "agent", "", "list: filter to runs of this agent id")
	runsCmd.Flags().IntVar(&runsLimit, "limit", 100, "list: cap result size (0 = no limit)")
	runsCmd.Flags().BoolVar(&runsShowOut, "output", false, "single-run: dump the .out file contents instead of the record")
	rootCmd.AddCommand(runsCmd)
}
