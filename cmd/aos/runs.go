package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
)

var runsCmd = &cobra.Command{
	Use:   "runs [run-id]",
	Short: "List recent runs, or show one by id",
	Long: `Without args: print recent runs sorted by start time, most recent first.

With one positional run id: print that run's record, including the captured
.out contents inline (also surfaced in the --json payload's "output" field).

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
	// Pull the full filtered set so the human view can report
	// "shown of total"; the JSON branch still honors --limit to keep the
	// existing contract with scripted consumers.
	all, err := scheduler.ReadRuns(runsDir, runsAgentID, 0)
	if err != nil {
		return fmt.Errorf("read runs: %w", err)
	}
	shown := all
	if runsLimit > 0 && len(shown) > runsLimit {
		shown = shown[:runsLimit]
	}
	if JSONOutput() {
		return printJSON(map[string]any{"runs": shown})
	}
	return printRunsHuman(shown, len(all))
}

func showOneRun(runsDir, runID string) error {
	run, err := scheduler.ReadRun(runsDir, runID)
	if err != nil {
		return err
	}
	// The wrapper writes the captured stdout/stderr to a sibling .out file,
	// not into the .json record. Merge it here so both human and JSON consumers
	// see the content without an extra flag.
	if out, err := scheduler.ReadRunOutput(runsDir, runID); err == nil && len(out) > 0 {
		run.Output = string(out)
	}
	if JSONOutput() {
		return printJSON(run)
	}
	return printOneRunHuman(run)
}

func printRunsHuman(runs []scheduler.JobRun, total int) error {
	if total == 0 {
		fmt.Println(styleMuted.Render("(no runs)"))
		return nil
	}
	fmt.Println(styleMuted.Render(runsCountSummary(len(runs), total, runsAgentID)))
	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, []string{
			r.ID, r.JobID, string(r.Status), r.Trigger,
			formatStartedAt(r.StartedAt), elapsedString(r),
		})
	}
	t := newTable(
		[]string{"RUN-ID", "AGENT", "STATUS", "TRIGGER", "STARTED", "ELAPSED"},
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

// runsCountSummary builds the muted "showing N of M runs" line that sits above
// the listing table, so the user can tell at a glance whether --limit is
// hiding records.
func runsCountSummary(shown, total int, agentID string) string {
	suffix := ""
	if agentID != "" {
		suffix = " for agent " + agentID
	}
	if shown < total {
		return fmt.Sprintf("showing %d of %d runs%s", shown, total, suffix)
	}
	noun := "runs"
	if total == 1 {
		noun = "run"
	}
	return fmt.Sprintf("showing %d %s%s", total, noun, suffix)
}

func printOneRunHuman(r scheduler.JobRun) error {
	banner("runs " + r.ID)
	statusS := statusStyle(string(r.Status))
	rows := []kvRow{
		{Key: "agent", Value: r.JobID},
		{Key: "status", Value: string(r.Status), Style: &statusS},
		{Key: "trigger", Value: r.Trigger},
		{Key: "startedAt", Value: formatStartedAt(r.StartedAt)},
	}
	if r.EndedAt != nil && *r.EndedAt != "" {
		rows = append(rows, kvRow{Key: "endedAt", Value: formatStartedAt(*r.EndedAt)})
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
	// Captured stdout/stderr lives in the .out file (loaded into r.Output by
	// showOneRun). Render it as a labeled block below the kv pairs so
	// multi-line content doesn't break the right-aligned key column.
	if r.Output != "" {
		fmt.Println()
		fmt.Println(styleLabel.Render("output"))
		fmt.Print(r.Output)
		if !strings.HasSuffix(r.Output, "\n") {
			fmt.Println()
		}
	}
	return nil
}

// elapsedString returns "..." while the run is still in flight (no endedAt)
// and a ms-precision duration once the wrapper has recorded the end. Missed
// runs never ran, so they render as an em-dash. Falls back to "-" if either
// timestamp won't parse — better than a panic.
func elapsedString(r scheduler.JobRun) string {
	if r.Status == scheduler.StatusMissed {
		return "—"
	}
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
	if r.Status == scheduler.StatusMissed {
		return "—"
	}
	if r.ExitCode == nil {
		return "-"
	}
	return fmt.Sprint(*r.ExitCode)
}

func init() {
	runsCmd.Flags().StringVar(&runsAgentID, "agent", "", "list: filter to runs of this agent id")
	runsCmd.Flags().IntVar(&runsLimit, "limit", 25, "list: cap result size (0 = no limit)")
	rootCmd.AddCommand(runsCmd)
}
