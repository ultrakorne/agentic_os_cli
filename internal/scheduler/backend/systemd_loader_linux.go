//go:build linux

package backend

import (
	"fmt"
	"os/exec"
	"strings"
)

// realSystemdLoader shells out to systemctl --user.
type realSystemdLoader struct{}

func (realSystemdLoader) DaemonReload() error {
	out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (realSystemdLoader) Enable(unitName string) error {
	out, err := exec.Command("systemctl", "--user", "enable", "--now", unitName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user enable --now %s: %w (%s)", unitName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (realSystemdLoader) Disable(unitName string) error {
	out, err := exec.Command("systemctl", "--user", "disable", "--now", unitName).CombinedOutput()
	if err != nil {
		s := strings.ToLower(string(out))
		if strings.Contains(s, "does not exist") || strings.Contains(s, "no such") {
			return nil
		}
		return fmt.Errorf("systemctl --user disable --now %s: %w (%s)", unitName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (realSystemdLoader) IsActive(unitName string) (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", unitName)
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() != 0 {
		return false, nil
	} else {
		return false, err
	}
}
