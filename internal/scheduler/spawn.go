package scheduler

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// SpawnOpts configures one wrapper invocation. The wrapper's argv contract is
//
//	wrapper.sh <aos-home> <schedule-id|''> <agent-id> <script-path> <run-id>
//
// Trigger is conveyed via the AGENTIC_OS_TRIGGER env var (defaults to
// "schedule" inside the wrapper if unset). Callers pass an explicit RunID so
// the in-memory stub returned by the spawning command matches the file the
// wrapper writes on disk.
type SpawnOpts struct {
	AosHome    string
	ScheduleID string
	AgentID    string
	ScriptPath string
	RunID      string
	Trigger    string
}

// SpawnWrapperDetached starts wrapper.sh in a new session so it survives the
// caller exiting. stdout/stderr/stdin default to /dev/null in os/exec when
// left nil, decoupling the wrapper from whatever shell aos was launched in.
func SpawnWrapperDetached(wrapperPath string, opts SpawnOpts) error {
	cmd := exec.Command(wrapperPath, opts.AosHome, opts.ScheduleID, opts.AgentID, opts.ScriptPath, opts.RunID)
	trigger := opts.Trigger
	if trigger == "" {
		trigger = "schedule"
	}
	cmd.Env = append(os.Environ(), "AGENTIC_OS_TRIGGER="+trigger)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

var (
	runIDOnce sync.Once
	runIDRand *rand.Rand
)

// NewRunID mirrors wrapper.sh's fallback id format (<unix-millis>-<rand4>).
// Threading a pre-generated id through the wrapper's 5th argv lets stubs
// returned by aos run / aos tick match the on-disk record the wrapper writes.
func NewRunID() string {
	runIDOnce.Do(func() {
		runIDRand = rand.New(rand.NewSource(time.Now().UnixNano()))
	})
	return fmt.Sprintf("%d-%04x", time.Now().UnixMilli(), runIDRand.Int31()&0xffff)
}
