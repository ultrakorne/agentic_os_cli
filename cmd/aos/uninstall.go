package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler/backend"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the aos config, wrapper.sh, tick log, and platform scheduler entries",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		s := runUninstall()
		if JSONOutput() {
			if err := printJSON(s); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
		banner("uninstall")
		printUninstallHuman(s)
	},
}

type uninstallSummary struct {
	Wrapper string `json:"wrapper"` // removed | absent
	TickLog string `json:"tickLog"` // removed | absent
	Backend string `json:"backend"` // removed | skipped:<reason>
	Config  string `json:"config"`  // removed | absent
}

func printUninstallHuman(s uninstallSummary) {
	stateStyle := func(v string) *lipgloss.Style {
		switch {
		case v == "removed":
			st := lipgloss.NewStyle().Foreground(colorSuccess)
			return &st
		case strings.HasPrefix(v, "skipped"):
			st := styleWarn
			return &st
		default:
			return nil
		}
	}
	rows := []kvRow{
		{Key: "wrapper", Value: s.Wrapper, Style: stateStyle(s.Wrapper)},
		{Key: "tickLog", Value: s.TickLog, Style: stateStyle(s.TickLog)},
		{Key: "backend", Value: s.Backend, Style: stateStyle(s.Backend)},
		{Key: "config", Value: s.Config, Style: stateStyle(s.Config)},
	}
	printKV(rows)
}

func runUninstall() uninstallSummary {
	s := uninstallSummary{Wrapper: "absent", TickLog: "absent", Backend: "unchanged", Config: "absent"}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: read config: %v\n", err)
	}

	if cfg != nil && cfg.AosHome != "" {
		wrapperPath := filepath.Join(cfg.AosHome, "wrapper.sh")
		if err := os.Remove(wrapperPath); err == nil {
			s.Wrapper = "removed"
		} else if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "warn: remove %s: %v\n", wrapperPath, err)
		}
		tickLog := filepath.Join(cfg.AosHome, "tick.log")
		if err := os.Remove(tickLog); err == nil {
			s.TickLog = "removed"
		} else if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "warn: remove %s: %v\n", tickLog, err)
		}
	}

	if cfg != nil && cfg.AosHome != "" {
		if be, err := backend.Select(cfg.AosHome); err != nil {
			s.Backend = "skipped:" + sanitize(err.Error())
		} else if err := be.Remove(); err != nil {
			s.Backend = "skipped:" + sanitize(err.Error())
		} else {
			s.Backend = "removed"
		}
	} else {
		s.Backend = "skipped:no-config"
	}

	configRemoved, _, err := config.Remove()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: remove config: %v\n", err)
	}
	if configRemoved {
		s.Config = "removed"
	}

	return s
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
