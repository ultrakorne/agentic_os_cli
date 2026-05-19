package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/build"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the aos version, commit, and build date",
	Long: "Prints the version of aos, the git commit it was built from, and\n" +
		"the build date. Local (non-release) builds report version \"dev\".\n" +
		"With --json prints {\"version\", \"commit\", \"date\", \"go\", \"os\", \"arch\"}.",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if JSONOutput() {
			payload := map[string]any{
				"version": build.Version,
				"commit":  build.Commit,
				"date":    build.Date,
				"go":      runtime.Version(),
				"os":      runtime.GOOS,
				"arch":    runtime.GOARCH,
			}
			if err := printJSON(payload); err != nil {
				fmt.Fprintf(os.Stderr, "aos version: %v\n", err)
				os.Exit(1)
			}
			return
		}
		banner("version")
		rows := []kvRow{
			{Key: "version", Value: build.Version},
			{Key: "commit", Value: build.Commit},
			{Key: "date", Value: build.Date},
			{Key: "go", Value: runtime.Version()},
			{Key: "platform", Value: runtime.GOOS + "/" + runtime.GOARCH},
		}
		printKV(rows)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
