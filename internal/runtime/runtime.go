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

