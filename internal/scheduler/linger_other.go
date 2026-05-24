//go:build !linux

package scheduler

// probeLinger is a no-op on non-Linux platforms. macOS LaunchAgents have no
// equivalent concept — they bind to the GUI session and there is nothing to
// enable for off-session firing.
func probeLinger() HealthState { return "" }
