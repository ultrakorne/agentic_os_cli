//go:build darwin

package backend

import (
	"fmt"
	"os/exec"
	"strings"
)

// realLaunchdLoader shells out to launchctl. Bootstrap and bootout target the
// per-user GUI domain (`gui/$UID`) so jobs run inside the login session and
// inherit keychain access — the whole reason this backend exists.
type realLaunchdLoader struct {
	uid int
}

func (l realLaunchdLoader) domain() string {
	return fmt.Sprintf("gui/%d", l.uid)
}

func (l realLaunchdLoader) Bootstrap(plistPath string) error {
	out, err := exec.Command("launchctl", "bootstrap", l.domain(), plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (l realLaunchdLoader) Bootout(label string) error {
	out, err := exec.Command("launchctl", "bootout", l.domain()+"/"+label).CombinedOutput()
	if err != nil {
		s := strings.ToLower(string(out))
		if strings.Contains(s, "could not find") || strings.Contains(s, "no such process") {
			return nil
		}
		return fmt.Errorf("launchctl bootout: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (l realLaunchdLoader) IsLoaded(label string) (bool, error) {
	cmd := exec.Command("launchctl", "print", l.domain()+"/"+label)
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() != 0 {
		return false, nil
	} else {
		return false, err
	}
}
