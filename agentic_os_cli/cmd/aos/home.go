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
		"Exits non-zero if aos has not been initialized.",
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
		fmt.Println(cfg.AosHome)
	},
}

func init() {
	rootCmd.AddCommand(homeCmd)
}
