package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/logtrim"
	"github.com/ultrakorne/aos_cli/internal/runtime"
	"github.com/ultrakorne/aos_cli/internal/scheduler/backend"
)

// HealthState is the per-probe enum on the refresh outcome.
type HealthState string

const (
	HealthOK       HealthState = "ok"
	HealthMissing  HealthState = "missing"
	HealthDown     HealthState = "down"
	HealthUnknown  HealthState = "unknown"
	HealthDisabled HealthState = "disabled"
)

// BackendSyncOutcome is the structured reconciliation summary from the
// platform-native backend (launchd on macOS, systemd-user on Linux). Replaces
// the cron-era CronSyncOutcome.
type BackendSyncOutcome struct {
	State     string   `json:"state"`
	Reasons   []string `json:"reasons,omitempty"`
	Wrote     int      `json:"wrote"`
	Unchanged int      `json:"unchanged"`
	Removed   int      `json:"removed"`
	Failed    []string `json:"failed,omitempty"`
}

// LogTrimOutcome reports whether tick.log was rotated this refresh.
type LogTrimOutcome struct {
	Trimmed bool `json:"trimmed"`
}

// RunsSweepOutcome is the structured Sweep result.
type RunsSweepOutcome struct {
	Deleted int    `json:"deleted"`
	Skipped string `json:"skipped,omitempty"`
}

// RefreshOutcome is the structured result of one Refresh call. The JSON
// shape is the contract Electron / scripts consume.
type RefreshOutcome struct {
	Agents        int                `json:"agents"`
	Scheduled     int                `json:"scheduled"`
	Issues        int                `json:"issues"`
	Backend       BackendSyncOutcome `json:"backend"`
	Wrapper       HealthState        `json:"wrapper"`
	Python3       HealthState        `json:"python3"`
	BackendHealth HealthState        `json:"backendHealth"`
	LingerState   HealthState        `json:"lingerState,omitempty"`
	Log           LogTrimOutcome     `json:"log"`
	Runs          RunsSweepOutcome   `json:"runs"`
	Warnings      []string           `json:"warnings,omitempty"`
}

// RefreshDeps gathers the inputs a Refresh needs from the verb.
type RefreshDeps struct {
	Cfg *config.Config
	Now time.Time
}

// Refresh executes the scan → record-misses → backend-sync → trim-log →
// sweep-runs pipeline.
func Refresh(deps RefreshDeps) (RefreshOutcome, error) {
	out := RefreshOutcome{
		Backend:       BackendSyncOutcome{State: "skipped", Reasons: []string{"unknown"}},
		Wrapper:       HealthMissing,
		Python3:       HealthMissing,
		BackendHealth: HealthUnknown,
	}

	cfg := deps.Cfg
	if cfg == nil || cfg.AosHome == "" {
		return out, fmt.Errorf("aos not initialized — run `aos init <path>` first")
	}
	if st, err := os.Stat(cfg.AosHome); err != nil || !st.IsDir() {
		return out, fmt.Errorf("aos_home %q does not exist or is not a directory", cfg.AosHome)
	}

	wrapperPath := filepath.Join(cfg.AosHome, "wrapper.sh")
	if runtime.FileExists(wrapperPath) && runtime.IsExecutable(wrapperPath) {
		out.Wrapper = HealthOK
	}
	if runtime.HasBin("python3") {
		out.Python3 = HealthOK
	}

	scan, err := ScanAgents(filepath.Join(cfg.AosHome, "agents"))
	if err != nil {
		return out, fmt.Errorf("scan agents: %w", err)
	}
	out.Agents = len(scan.Agents)
	out.Issues = len(scan.Issues)

	if _, _, err := RecordMissedRuns(cfg.AosHome, scan.Agents, deps.Now); err != nil {
		out.Warnings = append(out.Warnings, err.Error())
	}

	agentJobs := make([]backend.AgentJob, 0)
	for _, a := range scan.Agents {
		if a.Meta.Schedule == nil {
			continue
		}
		if len(a.Warnings) > 0 {
			out.Issues++
			continue
		}
		agentJobs = append(agentJobs, backend.AgentJob{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			Schedule:   *a.Meta.Schedule,
		})
	}
	out.Scheduled = len(agentJobs)

	interval, intervalErr := cfg.EffectiveTickInterval()
	if intervalErr != nil {
		out.Warnings = append(out.Warnings,
			fmt.Sprintf("%v; using default tick interval (%s)", intervalErr, config.DefaultTickInterval))
	}

	be, beErr := backend.Select(cfg.AosHome)
	if beErr != nil {
		out.Backend = BackendSyncOutcome{State: "skipped", Reasons: []string{beErr.Error()}}
	} else if out.Wrapper != HealthOK || out.Python3 != HealthOK {
		reasons := []string{}
		if out.Wrapper != HealthOK {
			reasons = append(reasons, "no-wrapper")
		}
		if out.Python3 != HealthOK {
			reasons = append(reasons, "no-python3")
		}
		out.Backend = BackendSyncOutcome{State: "skipped", Reasons: reasons}
	} else {
		aosBin, err := runtime.AosBinaryPath()
		if err != nil {
			out.Backend = BackendSyncOutcome{State: "skipped", Reasons: []string{err.Error()}}
		} else {
			spec := backend.Spec{
				Agents: agentJobs,
				Tick: backend.TickJob{
					AosBinaryPath: aosBin,
					LogPath:       filepath.Join(cfg.AosHome, "tick.log"),
					Interval:      interval,
				},
			}
			res, syncErr := be.Sync(spec)
			if syncErr != nil {
				out.Backend = BackendSyncOutcome{State: "skipped", Reasons: []string{syncErr.Error()}}
			} else {
				st, _ := be.State(spec)
				out.Backend = BackendSyncOutcome{
					State:     string(st),
					Wrote:     res.Wrote,
					Unchanged: res.Unchanged,
					Removed:   res.Removed,
				}
				for _, f := range res.Failed {
					out.Backend.Failed = append(out.Backend.Failed, fmt.Sprintf("%s: %s", f.AgentID, f.Reason))
				}
			}
		}
	}

	switch {
	case beErr != nil:
		out.BackendHealth = HealthMissing
	case be.Probe() != nil:
		out.BackendHealth = HealthDown
	default:
		out.BackendHealth = HealthOK
	}

	if linger := probeLinger(); linger != "" {
		out.LingerState = linger
	}

	trimmed, err := logtrim.Trim(filepath.Join(cfg.AosHome, "tick.log"), logtrim.DefaultMaxBytes, logtrim.DefaultKeepBytes)
	if err == nil && trimmed {
		out.Log.Trimmed = true
	}

	runsRes, err := NewFileRunStore(cfg.AosHome).Sweep(cfg.EffectiveRunsHardCap())
	if err != nil {
		out.Runs = RunsSweepOutcome{Skipped: err.Error()}
	} else {
		out.Runs = RunsSweepOutcome{Deleted: runsRes.Deleted}
	}

	return out, nil
}
