//go:build linux

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// maybePromptLinger inspects the linger probe result and either does nothing
// (linger on, or desktop session) or interactively offers to enable linger
// when running headless. Skipped entirely under --json.
func maybePromptLinger(refresh scheduler.RefreshOutcome) {
	if JSONOutput() {
		return
	}
	if refresh.LingerState == scheduler.HealthOK {
		return
	}
	if refresh.LingerState == "" {
		return
	}

	sessionType := os.Getenv("XDG_SESSION_TYPE")
	headless := sessionType == "" || sessionType == "tty"
	if !headless {
		// Desktop session: agents fire during the GUI session even without
		// linger, so the warning would be noise.
		fmt.Fprintln(os.Stderr, styleMuted.Render("info: linger off; ok for desktop session"))
		return
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stderr, styleWarn.Render("warn: linger disabled on a headless host; scheduled agents won't fire when you're not logged in."))
		fmt.Fprintln(os.Stderr, styleWarn.Render("warn: enable with `sudo loginctl enable-linger $USER`"))
		return
	}

	fmt.Fprintln(os.Stderr, styleWarn.Render("warn: linger is disabled. Without it, scheduled agents won't fire when you're not logged in (headless host)."))
	fmt.Fprint(os.Stderr, "Enable linger so scheduled agents run when you're not logged in? Requires sudo. [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	if ans == "n" || ans == "no" {
		fmt.Fprintln(os.Stderr, styleWarn.Render("warn: linger left disabled; agents may not fire while logged out."))
		return
	}

	user := os.Getenv("USER")
	if user == "" {
		fmt.Fprintln(os.Stderr, styleErr.Render("error: $USER is empty; not running enable-linger"))
		return
	}
	cmd := exec.Command("sudo", "loginctl", "enable-linger", user)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", styleErr.Render("error: enable-linger"), err)
		return
	}
	fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(colorSuccess).Render("linger enabled"))
}
