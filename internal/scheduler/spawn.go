package scheduler

import (
	"os"
	"os/exec"
	"syscall"
)

// SpawnOpts configures one wrapper invocation. The wrapper takes no
// positional args — every value (aos_home, agent id, script path, run id,
// trigger) is passed via env. Callers pass an explicit RunID so the in-memory
// stub returned by the spawning command matches the file the wrapper writes
// on disk.
type SpawnOpts struct {
	AosHome    string
	AgentID    string
	ScriptPath string
	RunID      string
	Trigger    string
}

// SpawnWrapperDetached starts wrapper.sh in a new session so it survives the
// caller exiting. stdout/stderr/stdin default to /dev/null in os/exec when
// left nil, decoupling the wrapper from whatever shell aos was launched in.
func SpawnWrapperDetached(wrapperPath string, opts SpawnOpts) error {
	cmd := exec.Command(wrapperPath)
	trigger := opts.Trigger
	if trigger == "" {
		trigger = "schedule"
	}
	cmd.Env = append(os.Environ(),
		"AGENTIC_OS_DATA_DIR="+opts.AosHome,
		"AGENTIC_OS_AGENT_ID="+opts.AgentID,
		"AGENTIC_OS_AGENT_SCRIPT="+opts.ScriptPath,
		"AGENTIC_OS_RUN_ID="+opts.RunID,
		"AGENTIC_OS_TRIGGER="+trigger,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
