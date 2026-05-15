package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func HasBin(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func IsExecutable(path string) bool {
	return unix.Access(path, unix.X_OK) == nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// AosBinaryPath returns the absolute path of the running aos binary, with
// symlinks resolved. Used to bake an absolute command path into the cron
// managed block so it works under cron's minimal PATH.
func AosBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate aos binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// EvalSymlinks fails if the file vanished; fall back to the raw path.
		resolved = exe
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("absolutize aos path %q: %w", resolved, err)
	}
	return abs, nil
}

// CronDaemonRunning reports whether a cron daemon process appears to be
// running. (true, nil)=running, (false, nil)=not running but pgrep worked,
// (false, err)=pgrep unavailable or all queries errored.
func CronDaemonRunning() (bool, error) {
	names := []string{"crond", "cron", "cronie"}
	workedAtLeastOnce := false
	for _, n := range names {
		cmd := exec.Command("pgrep", "-x", n)
		err := cmd.Run()
		if err == nil {
			return true, nil
		}
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() == 1 {
				workedAtLeastOnce = true
				continue
			}
		}
	}
	if workedAtLeastOnce {
		return false, nil
	}
	return false, exec.ErrNotFound
}
