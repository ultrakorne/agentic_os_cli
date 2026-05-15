package runtime

import (
	"os"
	"os/exec"

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

// AosOnCronPath reports whether `aos` resolves through the minimal PATH
// cron daemons typically use. Useful warning at refresh time so the user
// knows a successful interactive invocation doesn't guarantee the cron
// invocation will find the binary.
func AosOnCronPath() bool {
	cronPath := "/usr/bin:/bin:/usr/local/bin"
	prev, ok := os.LookupEnv("PATH")
	if err := os.Setenv("PATH", cronPath); err != nil {
		return false
	}
	defer func() {
		if ok {
			_ = os.Setenv("PATH", prev)
		} else {
			_ = os.Unsetenv("PATH")
		}
	}()
	_, err := exec.LookPath("aos")
	return err == nil
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
