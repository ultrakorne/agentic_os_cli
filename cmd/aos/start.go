package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Open the interactive dashboard",
	Long: `Launch the terminal UI: agents grouped by section, with vim-style
navigation, in-list filter (/), and one-key manual runs (x). Refreshes live
from the filesystem as the wrapper writes new run records.

` + "`aos` with no arguments is an alias for `aos start`. Press Ctrl+C to exit.",
	Args: cobra.NoArgs,
	RunE: runStart,
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return errors.New("aos not initialized — run `aos init <path>` first")
	}

	agentsDir := filepath.Join(cfg.AosHome, "agents")
	store := scheduler.NewFileRunStore(cfg.AosHome)

	scan, err := scheduler.ScanAgents(agentsDir)
	if err != nil {
		return fmt.Errorf("scan agents: %w", err)
	}
	// Ensure runs/ exists so fsnotify.Add doesn't fail on a fresh init.
	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		return fmt.Errorf("mkdir runs: %w", err)
	}
	runs, err := store.List(scheduler.Filter{})
	if err != nil {
		return fmt.Errorf("read runs: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	if err := watcher.Add(store.Dir()); err != nil {
		watcher.Close()
		return fmt.Errorf("watch %s: %w", store.Dir(), err)
	}
	defer watcher.Close()

	model := newStartModel(cfg.AosHome, store, scan, runs, watcher.Events, watcher.Errors)
	_, err = tea.NewProgram(&model).Run()
	return err
}

func init() {
	rootCmd.AddCommand(startCmd)
	// Bare `aos` with no subcommand falls through to `aos start`. Cobra
	// still routes `aos help`, `aos -h`, and known verbs to their dedicated
	// commands; unknown verbs (`aos foo`) get cobra's built-in "unknown
	// command" error before reaching this RunE.
	rootCmd.RunE = runStart
}
