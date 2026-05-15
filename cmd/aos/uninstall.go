package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/crontab"
	"github.com/ultrakorne/aos_cli/internal/runtime"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the aos config, wrapper.sh, and managed cron section",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		s := runUninstall()
		fmt.Println(s.OneLine())
	},
}

type uninstallSummary struct {
	Wrapper string // removed | absent
	Cron    string // removed | unchanged | skipped:<reason>
	Config  string // removed | absent
	Kept    []string
}

func (s uninstallSummary) OneLine() string {
	kept := "[]"
	if len(s.Kept) > 0 {
		kept = "[" + strings.Join(s.Kept, ",") + "]"
	}
	return fmt.Sprintf("aos uninstall wrapper=%s cron=%s config=%s kept=%s", s.Wrapper, s.Cron, s.Config, kept)
}

func runUninstall() uninstallSummary {
	s := uninstallSummary{Wrapper: "absent", Cron: "unchanged", Config: "absent"}

	cfg, err := config.Load()
	if err != nil {
		// surface but continue — we still want to try removing the crontab block.
		fmt.Fprintf(os.Stderr, "warn: read config: %v\n", err)
	}

	if cfg != nil && cfg.AosHome != "" {
		wrapperPath := filepath.Join(cfg.AosHome, "wrapper.sh")
		if err := os.Remove(wrapperPath); err == nil {
			s.Wrapper = "removed"
		} else if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "warn: remove %s: %v\n", wrapperPath, err)
		}

		// Try empty-only removal of agents/, runs/, and aos_home itself.
		for _, sub := range []string{"agents", "runs"} {
			p := filepath.Join(cfg.AosHome, sub)
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				s.Kept = append(s.Kept, p)
			}
		}
		if err := os.Remove(cfg.AosHome); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.Kept = append(s.Kept, cfg.AosHome)
		}
	}

	if runtime.HasBin("crontab") {
		result, err := crontab.RemoveManaged()
		switch {
		case err != nil:
			s.Cron = "skipped:" + sanitize(err.Error())
		case result.Wrote:
			s.Cron = "removed"
		default:
			s.Cron = "unchanged"
		}
	} else {
		s.Cron = "skipped:no-crontab-bin"
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
