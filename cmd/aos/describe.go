package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var describeCmd = &cobra.Command{
	Use:   "describe <id> [text]",
	Short: "Show one agent (and optionally rewrite its description)",
	Long: `Print the full record for a single agent: section, script path,
schedule (compiled cron expression and structured spec), scheduledAt, and
description. The JSON form matches the per-agent shape of ` + "`aos list --json`" + `
so a client can use a single parser for both.

With a second positional argument, the description is written before the
record is printed. An empty string clears it.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return errors.New("aos not initialized — run `aos init <path>` first")
	}

	id := args[0]
	agentsDir := filepath.Join(cfg.AosHome, "agents")
	agent, _, err := scheduler.FindAgentByID(agentsDir, id)
	if err != nil {
		return err
	}

	meta := agent.Meta
	if len(args) == 2 {
		updated, err := scheduler.WriteDescription(agent.MetaPath, args[1])
		if err != nil {
			return fmt.Errorf("write meta: %w", err)
		}
		meta = updated
	}

	rec := agentRecord(agent, meta)
	if JSONOutput() {
		buf, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(buf))
		return nil
	}
	return printAgentHuman(rec)
}

// printAgentHuman renders the record as a small key/value block. Schedule
// fields collapse to one line; the description goes last so it isn't
// truncated and the user can grep/copy it cleanly.
func printAgentHuman(rec map[string]any) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	row := func(k string, v any) {
		fmt.Fprintf(w, "%s\t%v\n", k, v)
	}
	row("id", rec["id"])
	row("section", rec["section"])
	row("script", rec["scriptPath"])
	if sched, ok := rec["schedule"].(*scheduler.ScheduleSpec); ok && sched != nil {
		row("schedule", summarizeSchedule(sched))
		if cronExpr, ok := rec["cron"].(string); ok {
			row("cron", cronExpr)
		}
		if ts, ok := rec["scheduledAt"].(string); ok {
			row("scheduledAt", ts)
		}
	} else {
		row("schedule", "-")
	}
	desc, _ := rec["description"].(string)
	if desc == "" {
		desc = "-"
	}
	row("description", desc)
	return w.Flush()
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
