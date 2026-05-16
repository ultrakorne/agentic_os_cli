package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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
		return printJSON(map[string]any{"runs": runs})
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
		return printJSON(run)
	}
	return printOneRunHuman(run)
}

func printRunsHuman(runs []scheduler.JobRun) error {
	if len(runs) == 0 {
		fmt.Println(styleMuted.Render("(no runs)"))
		return nil
	}
	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, []string{
			r.ID, r.JobID, string(r.Status), r.Trigger,
			r.StartedAt, elapsedString(r), exitString(r),
		})
	}
	t := newTable(
		[]string{"RUN-ID", "AGENT", "STATUS", "TRIGGER", "STARTED", "ELAPSED", "EXIT"},
		rows,
	).StyleFunc(func(row, col int) lipgloss.Style {
		if row == table.HeaderRow {
			return tableHeaderStyle
		}
		// Column 2 is STATUS — colorize per running/success/error so a long
		// list scans visually at a glance.
		if col == 2 {
			return statusStyle(rows[row][2]).Padding(0, 1)
		}
		return tableCellStyle
	})
	fmt.Println(t)
	return nil
}

func printOneRunHuman(r scheduler.JobRun) error {
	banner("runs " + r.ID)
	statusS := statusStyle(string(r.Status))
	rows := []kvRow{
		{Key: "agent", Value: r.JobID},
		{Key: "status", Value: string(r.Status), Style: &statusS},
		{Key: "trigger", Value: r.Trigger},
		{Key: "startedAt", Value: r.StartedAt},
	}
	if r.EndedAt != nil && *r.EndedAt != "" {
		rows = append(rows, kvRow{Key: "endedAt", Value: *r.EndedAt})
	}
	rows = append(rows,
		kvRow{Key: "elapsed", Value: elapsedString(r)},
		kvRow{Key: "exit", Value: exitString(r)},
	)
	if r.OutputPath != nil && *r.OutputPath != "" {
		rows = append(rows, kvRow{Key: "outputPath", Value: *r.OutputPath})
	}
	if r.Error != nil && *r.Error != "" {
		errS := styleErr
		rows = append(rows, kvRow{Key: "error", Value: *r.Error, Style: &errS})
	}
	printKV(rows)
	return nil
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
