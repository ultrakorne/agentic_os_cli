package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
)

var homeCmd = &cobra.Command{
	Use:   "home",
	Short: "Print the configured aos_home directory",
	Long: "Prints the absolute path of the aos_home directory on stdout.\n" +
		"Exits non-zero if aos has not been initialized.\n" +
		"With --json prints {\"home\": \"<path>\"} so machine consumers get the\n" +
		"same shape every other verb uses.",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "aos home: read config: %v\n", err)
			os.Exit(1)
		}
		if cfg == nil || cfg.AosHome == "" {
			fmt.Fprintln(os.Stderr, "aos home: not initialized — run `aos init <path>` first")
			os.Exit(1)
		}
		if JSONOutput() {
			if err := printJSON(map[string]any{"home": cfg.AosHome}); err != nil {
				fmt.Fprintf(os.Stderr, "aos home: %v\n", err)
				os.Exit(1)
			}
			return
		}
		// Plain path (no styling) so existing `$(aos home)/runs` patterns keep
		// working. Lipgloss is reserved for the verbs that produce structured
		// human output — this one stays scriptable by design.
		fmt.Println(cfg.AosHome)
	},
}

func init() {
	rootCmd.AddCommand(homeCmd)
}
