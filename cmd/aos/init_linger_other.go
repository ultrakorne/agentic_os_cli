//go:build !linux

package main

import "github.com/ultrakorne/aos_cli/internal/scheduler"

// maybePromptLinger is a no-op on non-Linux. Linger is a systemd-user
// concept; launchd LaunchAgents bind to the GUI session naturally.
func maybePromptLinger(_ scheduler.RefreshOutcome) {}
