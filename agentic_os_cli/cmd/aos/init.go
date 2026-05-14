package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Initialize aos with a home path",
	Args:  cobra.ExactArgs(1),
	Run:   initFunc,
}

func initFunc(cmd *cobra.Command, args []string) {
	targetPath := args[0]
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	configDir := filepath.Join(homeDir, ".config", "aos")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		fmt.Printf("Error creating config directory: %v\n", err)
		os.Exit(1)
	}
	configPath := filepath.Join(configDir, "config.toml")

	content := fmt.Sprintf("aos_home = \"%s\"\n", targetPath)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		fmt.Printf("Error writing config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("aos init confiig at -> %s\n", configPath)
}

func init() {
	rootCmd.AddCommand(initCmd)
}
