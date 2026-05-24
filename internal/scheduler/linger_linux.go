//go:build linux

package scheduler

import (
	"os/exec"
	"os/user"
	"strings"
)

// probeLinger returns "ok" when loginctl reports linger enabled for the
// current user, "disabled" when it's off, or "unknown" when loginctl isn't
// available. linger off on a headless host means agent timers won't fire
// when the user isn't logged in — refresh surfaces this for `aos init` to
// nudge with a prompt.
func probeLinger() HealthState {
	u, err := user.Current()
	if err != nil {
		return HealthUnknown
	}
	out, err := exec.Command("loginctl", "show-user", u.Username, "--property=Linger", "--value").Output()
	if err != nil {
		return HealthUnknown
	}
	switch strings.TrimSpace(string(out)) {
	case "yes":
		return HealthOK
	case "no":
		return HealthDisabled
	}
	return HealthUnknown
}
