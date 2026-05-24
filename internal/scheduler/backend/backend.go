// Package backend defines the platform-native scheduling interface aos uses
// to install agent jobs and the tick job. macOS runs through launchd
// LaunchAgents; Linux through systemd-user units. Select picks per runtime.
package backend

import (
	"fmt"
	"runtime"
	"time"

	"github.com/ultrakorne/aos_cli/internal/scheduler/schedspec"
)

// Backend is the platform-native scheduler aos drives. Implementations write
// their unit files under <aos_home>-derived paths, load/enable them via the
// platform tool, and report drift via State.
type Backend interface {
	Sync(spec Spec) (SyncResult, error)
	Remove() error
	State(spec Spec) (State, error)
}

// Spec is the full set of jobs aos wants installed in one Sync call.
type Spec struct {
	Agents []AgentJob
	Tick   TickJob
}

// AgentJob is one user agent we want fired on a schedule.
type AgentJob struct {
	AgentID    string
	ScriptPath string
	Schedule   schedspec.ScheduleSpec
}

// TickJob is the periodic `aos tick` invocation; empty Interval means skip.
type TickJob struct {
	AosBinaryPath string
	LogPath       string
	Interval      time.Duration
}

// SyncResult is the per-backend reconciliation summary.
type SyncResult struct {
	Wrote     int
	Unchanged int
	Removed   int
	Failed    []FailedJob
}

// FailedJob captures one agent that couldn't be installed; the sweep continues.
type FailedJob struct {
	AgentID string
	Reason  string
}

// State is the drift snapshot for the namespace.
type State string

const (
	StateManaged State = "managed"
	StateDrift   State = "drift"
	StateEmpty   State = "empty"
)

// Select returns the platform backend. macOS → launchd; Linux → systemd-user.
// Platform-specific selection lives in select_darwin.go / select_linux.go;
// the other_unix.go fallback covers builds for unsupported GOOS.
func Select(aosHome string) (Backend, error) {
	if b := platformBackend(aosHome); b != nil {
		return b, nil
	}
	return nil, fmt.Errorf("aos requires macOS or Linux (got %s)", runtime.GOOS)
}
