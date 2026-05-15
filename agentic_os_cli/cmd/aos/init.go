package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/resources"
)

var initCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Initialize aos with a home path",
	Args:  cobra.ExactArgs(1),
	Run:   initFunc,
}

func initFunc(cmd *cobra.Command, args []string) {
	target, err := expandAbs(args[0])
	if err != nil {
		fail("resolve target path: %v", err)
	}

	existing, err := config.Load()
	if err != nil {
		fail("read existing config: %v", err)
	}

	mode := "fresh"
	if existing != nil && existing.AosHome != "" {
		oldHome, err := expandAbs(existing.AosHome)
		if err != nil {
			fail("resolve existing aos_home: %v", err)
		}
		switch {
		case oldHome == target:
			mode = "same"
		default:
			moved, err := relocateHome(oldHome, target)
			if err != nil {
				fail("%v", err)
			}
			if moved {
				mode = "moved"
			} else {
				mode = "repointed"
			}
		}
	}

	wrapperState, err := ensureHome(target)
	if err != nil {
		fail("%v", err)
	}

	if err := config.Save(&config.Config{AosHome: target}); err != nil {
		fail("write config: %v", err)
	}

	refresh, err := RunRefresh()
	if err != nil {
		fail("refresh: %v", err)
	}

	fmt.Printf("aos init mode=%s home=%s wrapper=%s | %s\n", mode, target, wrapperState, refresh.OneLine())
}

func expandAbs(p string) (string, error) {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			p = home
		} else if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func relocateHome(oldHome, newHome string) (bool, error) {
	oldInfo, err := os.Stat(oldHome)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat old home %s: %w", oldHome, err)
	}
	if !oldInfo.IsDir() {
		return false, fmt.Errorf("old aos_home %s is not a directory", oldHome)
	}

	newInfo, err := os.Stat(newHome)
	switch {
	case err != nil && errors.Is(err, os.ErrNotExist):
		// destination clear; mkdir parent
		if err := os.MkdirAll(filepath.Dir(newHome), 0o755); err != nil {
			return false, fmt.Errorf("mkdir parent of %s: %w", newHome, err)
		}
	case err != nil:
		return false, fmt.Errorf("stat new home %s: %w", newHome, err)
	default:
		if !newInfo.IsDir() {
			return false, fmt.Errorf("relocation target %s exists and is not a directory", newHome)
		}
		empty, err := dirEmpty(newHome)
		if err != nil {
			return false, fmt.Errorf("inspect %s: %w", newHome, err)
		}
		if !empty {
			return false, fmt.Errorf("relocation target %s already exists and is not empty — resolve manually", newHome)
		}
		// remove the empty target so Rename can land on it
		if err := os.Remove(newHome); err != nil {
			return false, fmt.Errorf("remove empty target %s: %w", newHome, err)
		}
	}

	if err := os.Rename(oldHome, newHome); err != nil {
		var linkErr *os.LinkError
		if errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV) {
			if err := copyTree(oldHome, newHome); err != nil {
				return false, fmt.Errorf("copy %s -> %s: %w", oldHome, newHome, err)
			}
			if err := os.RemoveAll(oldHome); err != nil {
				return false, fmt.Errorf("remove old home %s: %w", oldHome, err)
			}
			return true, nil
		}
		return false, fmt.Errorf("rename %s -> %s: %w", oldHome, newHome, err)
	}
	return true, nil
}

// ensureHome creates the home dir, agents/, runs/, and writes wrapper.sh from
// the embedded copy. Returns wrapperState=wrote|same.
func ensureHome(home string) (string, error) {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", home, err)
	}
	for _, sub := range []string{"agents", "runs"} {
		if err := os.MkdirAll(filepath.Join(home, sub), 0o755); err != nil {
			return "", fmt.Errorf("mkdir %s: %w", sub, err)
		}
	}
	wrapperPath := filepath.Join(home, "wrapper.sh")
	state := "wrote"
	if existing, err := os.ReadFile(wrapperPath); err == nil {
		if bytes.Equal(existing, resources.WrapperSh) {
			state = "same"
		}
	}
	if state != "same" {
		if err := os.WriteFile(wrapperPath, resources.WrapperSh, 0o755); err != nil {
			return "", fmt.Errorf("write %s: %w", wrapperPath, err)
		}
		// WriteFile honors umask; force exact mode.
		if err := os.Chmod(wrapperPath, 0o755); err != nil {
			return "", fmt.Errorf("chmod %s: %w", wrapperPath, err)
		}
	}
	return state, nil
}

func dirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target, info.Mode().Perm())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "aos init: "+format+"\n", args...)
	os.Exit(1)
}

func init() {
	rootCmd.AddCommand(initCmd)
}
