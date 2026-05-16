package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "aos",
	Short: "Agentic Os cli",
	Long:  "Agentic Os (aos) is the cli to schedule and observe agents",
}

// jsonOutput is set by the persistent `--json` flag on rootCmd. Subcommands
// read it via JSONOutput() to pick between a human-readable one-line summary
// and a structured JSON payload.
var jsonOutput bool

func JSONOutput() bool { return jsonOutput }

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON instead of the human summary")
	// Cobra prints "Error: <msg>" itself; we don't want to re-print on Execute
	// failure, and the full usage dump after a real error just buries the
	// signal. Subcommands opt into showing usage explicitly when they want it.
	rootCmd.SilenceUsage = true
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
