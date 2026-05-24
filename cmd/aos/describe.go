package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var describeCmd = &cobra.Command{
	Use:   "describe <id> [text]",
	Short: "Show one agent (and optionally rewrite its description)",
	Long: `Print the full record for a single agent: section, script path,
schedule (structured spec), scheduledAt, and description. The JSON form
matches the per-agent shape of ` + "`aos list --json`" + ` so a client can use a
single parser for both.

With a second positional argument, the description is written before the
record is printed. An empty string clears it.`,
	Args: cobra.MaximumNArgs(2),
	RunE: runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
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
		return printJSON(rec)
	}
	return printAgentHuman(rec)
}

// printAgentHuman renders the record as a styled key/value block. Schedule
// fields collapse to one line; the description goes last so it isn't
// truncated and the user can grep/copy it cleanly.
func printAgentHuman(rec map[string]any) error {
	banner("describe " + asString(rec["id"]))
	rows := []kvRow{
		{Key: "section", Value: asString(rec["section"])},
		{Key: "script", Value: asString(rec["scriptPath"])},
	}
	if warns, ok := rec["warnings"].([]string); ok && len(warns) > 0 {
		warnS := styleWarn
		rows = append(rows, kvRow{Key: "warnings", Value: strings.Join(warns, ","), Style: &warnS})
	}
	if sched, ok := rec["schedule"].(*scheduler.ScheduleSpec); ok && sched != nil {
		rows = append(rows, kvRow{Key: "schedule", Value: summarizeSchedule(sched)})
		if ts, ok := rec["scheduledAt"].(string); ok {
			rows = append(rows, kvRow{Key: "scheduledAt", Value: ts})
		}
	} else {
		rows = append(rows, kvRow{Key: "schedule", Value: "-"})
	}
	desc, _ := rec["description"].(string)
	rows = append(rows, kvRow{Key: "description", Value: desc})
	printKV(rows)
	return nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
