package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents with their schedule and description",
	Args:  cobra.NoArgs,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return errors.New("aos not initialized — run `aos init <path>` first")
	}

	res, err := scheduler.ScanAgents(filepath.Join(cfg.AosHome, "agents"))
	if err != nil {
		return fmt.Errorf("scan agents: %w", err)
	}

	if JSONOutput() {
		return printListJSON(res)
	}
	return printListHuman(res)
}

func printListJSON(res scheduler.ScanResult) error {
	items := make([]map[string]any, 0, len(res.Agents))
	for _, a := range res.Agents {
		items = append(items, agentRecord(a, a.Meta))
	}
	issues := make([]map[string]string, 0, len(res.Issues))
	for _, iss := range res.Issues {
		issues = append(issues, map[string]string{
			"kind": iss.Kind,
			"path": iss.Path,
			"note": iss.Note,
		})
	}
	buf, err := json.MarshalIndent(map[string]any{
		"agents": items,
		"issues": issues,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(buf))
	return nil
}

func printListHuman(res scheduler.ScanResult) error {
	if len(res.Agents) == 0 {
		fmt.Println("(no agents)")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSECTION\tSCHEDULE\tWARNINGS\tDESCRIPTION")
		for _, a := range res.Agents {
			warn := "-"
			if len(a.Warnings) > 0 {
				warn = strings.Join(a.Warnings, ",")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				a.ID, a.Section, summarizeSchedule(a.Meta.Schedule), warn, summarizeDescription(a.Meta.Description))
		}
		w.Flush()
	}
	for _, iss := range res.Issues {
		fmt.Fprintf(os.Stderr, "issue: %s %s — %s\n", iss.Kind, iss.Path, iss.Note)
	}
	return nil
}

func summarizeSchedule(s *scheduler.ScheduleSpec) string {
	if s == nil {
		return "-"
	}
	switch s.Kind {
	case "hourly":
		if s.EveryHours == 1 {
			return fmt.Sprintf("hourly :%02d", s.Minute)
		}
		return fmt.Sprintf("every %dh :%02d", s.EveryHours, s.Minute)
	case "daily":
		return fmt.Sprintf("%s %02d:%02d", joinDays(s.Days), s.Hour, s.Minute)
	}
	return s.Kind
}

func summarizeDescription(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return "-"
	}
	if i := strings.IndexByte(d, '\n'); i >= 0 {
		d = d[:i] + "…"
	}
	if len(d) > 60 {
		d = d[:57] + "…"
	}
	return d
}

func init() {
	rootCmd.AddCommand(listCmd)
}
