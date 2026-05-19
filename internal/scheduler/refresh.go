package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/crontab"
	"github.com/ultrakorne/aos_cli/internal/logtrim"
	"github.com/ultrakorne/aos_cli/internal/runtime"
)

// HealthState is the per-probe enum on the refresh outcome.
type HealthState string

const (
	HealthOK      HealthState = "ok"
	HealthMissing HealthState = "missing"
	HealthDown    HealthState = "down"
	HealthUnknown HealthState = "unknown"
)

// CronSyncState is the structured outcome of the managed crontab block write.
// Replaces the old colon-encoded string ("skipped:no-crontab-bin"); consumers
// branch on State and read Reasons for the cause(s).
type CronSyncState string

const (
	CronUnchanged CronSyncState = "unchanged"
	CronWrote     CronSyncState = "wrote"
	CronSkipped   CronSyncState = "skipped"
	CronConflict  CronSyncState = "conflict"
)

type CronSyncOutcome struct {
	State   CronSyncState `json:"state"`
	Reasons []string      `json:"reasons,omitempty"`
}

// LogTrimOutcome reports whether tick.log was rotated this refresh.
type LogTrimOutcome struct {
	Trimmed bool `json:"trimmed"`
}

// RunsSweepOutcome is the structured Sweep result. Deleted counts run-id pairs
// pruned; Skipped is non-empty only when the sweep itself errored out (the
// algorithm proceeds regardless).
type RunsSweepOutcome struct {
	Deleted int    `json:"deleted"`
	Skipped string `json:"skipped,omitempty"`
}

// RefreshOutcome is the structured result of one Refresh call. It is the JSON
// wire format the CLI emits under `aos refresh --json` and the shape Electron
// consumes.
type RefreshOutcome struct {
	Agents    int              `json:"agents"`
	Scheduled int              `json:"scheduled"`
	Issues    int              `json:"issues"`
	Cron      CronSyncOutcome  `json:"cron"`
	Wrapper   HealthState      `json:"wrapper"`
	Python3   HealthState      `json:"python3"`
	Daemon    HealthState      `json:"daemon"`
	Log       LogTrimOutcome   `json:"log"`
	Runs      RunsSweepOutcome `json:"runs"`
	Warnings  []string         `json:"warnings,omitempty"`
}

// RefreshDeps gathers the inputs a Refresh needs from the verb. Now is taken
// explicitly so callers can pin it in tests; the rest of the runtime probes
// are called directly inside Refresh.
type RefreshDeps struct {
	Cfg *config.Config
	Now time.Time
}

// Refresh executes the scan → record-misses → compile-cron → sync-crontab →
// trim-log → sweep-runs pipeline. Non-fatal step errors land on
// RefreshOutcome.Warnings; only an unrecoverable failure (no config, missing
// aos_home, scan error) returns an error.
func Refresh(deps RefreshDeps) (RefreshOutcome, error) {
	out := RefreshOutcome{
		Cron:    CronSyncOutcome{State: CronSkipped, Reasons: []string{"unknown"}},
		Wrapper: HealthMissing,
		Python3: HealthMissing,
		Daemon:  HealthUnknown,
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
	hasCrontab := runtime.HasBin("crontab")
	if running, err := runtime.CronDaemonRunning(); err == nil {
		if running {
			out.Daemon = HealthOK
		} else {
			out.Daemon = HealthDown
		}
	}

	scan, err := ScanAgents(filepath.Join(cfg.AosHome, "agents"))
	if err != nil {
		return out, fmt.Errorf("scan agents: %w", err)
	}
	out.Agents = len(scan.Agents)
	out.Issues = len(scan.Issues)

	// Record any newly-detected missed slots as runs/miss-*.json so the
	// dashboard sees them in the run history. Failure is non-fatal — the
	// cron block is still the more important thing to reconcile.
	if _, _, err := RecordMissedRuns(cfg.AosHome, scan.Agents, deps.Now); err != nil {
		out.Warnings = append(out.Warnings, err.Error())
	}

	entries := make([]crontab.Entry, 0)
	for _, a := range scan.Agents {
		if a.Meta.Schedule == nil {
			continue
		}
		if len(a.Warnings) > 0 {
			// A warned agent (e.g. not-executable) shouldn't enter the
			// managed crontab block — cron would fire a script that can't
			// run. Surface the count so a human reading the summary can see
			// why a scheduled agent isn't showing up under cron.
			out.Issues++
			continue
		}
		expr, err := CompileToCron(*a.Meta.Schedule)
		if err != nil {
			out.Issues++
			continue
		}
		entries = append(entries, crontab.Entry{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			Expression: expr,
		})
	}
	out.Scheduled = len(entries)

	if hasCrontab && out.Wrapper == HealthOK && out.Python3 == HealthOK {
		aosBin, err := runtime.AosBinaryPath()
		if err != nil {
			out.Cron = CronSyncOutcome{State: CronSkipped, Reasons: []string{err.Error()}}
		} else {
			tickCmd := crontab.BuildTickCommand(aosBin, cfg.AosHome)
			tickSchedule, tickErr := cfg.EffectiveTickCronExpr()
			if tickErr != nil {
				// Bad tick_interval doesn't fail the refresh — fall back to
				// the default cadence (returned by EffectiveTickCronExpr) and
				// surface the parse error so the operator can fix config.toml.
				out.Warnings = append(out.Warnings,
					fmt.Sprintf("%v; using default tick interval (%s)", tickErr, config.DefaultTickInterval))
			}
			result, err := crontab.SyncCrontab(crontab.SyncArgs{
				Entries:      entries,
				WrapperPath:  wrapperPath,
				DataDir:      cfg.AosHome,
				TickSchedule: tickSchedule,
				TickCommand:  tickCmd,
			})
			switch {
			case err != nil:
				out.Cron = CronSyncOutcome{State: CronSkipped, Reasons: []string{err.Error()}}
			case result.Conflict:
				out.Cron = CronSyncOutcome{State: CronConflict}
			case result.Wrote:
				out.Cron = CronSyncOutcome{State: CronWrote}
			default:
				out.Cron = CronSyncOutcome{State: CronUnchanged}
			}
		}
	} else {
		reasons := []string{}
		if !hasCrontab {
			reasons = append(reasons, "no-crontab-bin")
		}
		if out.Wrapper != HealthOK {
			reasons = append(reasons, "no-wrapper")
		}
		if out.Python3 != HealthOK {
			reasons = append(reasons, "no-python3")
		}
		out.Cron = CronSyncOutcome{State: CronSkipped, Reasons: reasons}
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
