package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var tickCmd = &cobra.Command{
	Use:   "tick",
	Short: "Run one scheduler tick: scan, detect missed runs, log a summary",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runTick()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[tick] failed: %v\n", err)
			os.Exit(1)
		}
		emitWarnings(out.Warnings)
		if JSONOutput() {
			if jerr := printJSON(out); jerr != nil {
				fmt.Fprintln(os.Stderr, jerr)
				os.Exit(1)
			}
			return
		}
		// Match the on-disk log's terse single-line shape — when cron tails
		// this verb, the operator wants the same string they'd grep for in
		// tick.log.
		fmt.Print(out.LogLine())
	},
}

func runTick() (scheduler.TickOutcome, error) {
	cfg, err := config.Load()
	if err != nil {
		return scheduler.TickOutcome{}, fmt.Errorf("load config: %w", err)
	}
	return scheduler.Tick(scheduler.TickDeps{Cfg: cfg, Now: time.Now()})
}

func init() {
	rootCmd.AddCommand(tickCmd)
}
