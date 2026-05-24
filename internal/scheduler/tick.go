package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/runtime"
	"github.com/ultrakorne/aos_cli/internal/scheduler/backend"
)

// TickOutcome is the structured result of one scheduler tick. The on-disk
// tick.log keeps its historical "[tick] ..." prefix (dashboard tail consumers
// depend on it); this struct is the JSON wire format under --json.
type TickOutcome struct {
	Timestamp     string   `json:"timestamp"`
	Scripts       int      `json:"scripts"`
	Scheduled     int      `json:"scheduled"`
	Missed        int      `json:"missed"`
	StaleResolved int      `json:"staleResolved"`
	Backend       string   `json:"backend"`
	Warnings      []string `json:"warnings,omitempty"`
}

// LogLine returns the single line the on-disk tick.log receives.
func (t TickOutcome) LogLine() string {
	return fmt.Sprintf("[tick] %s scripts=%d scheduled=%d missed=%d staleResolved=%d backend=%s\n",
		t.Timestamp, t.Scripts, t.Scheduled, t.Missed, t.StaleResolved, t.Backend)
}

type TickDeps struct {
	Cfg *config.Config
	Now time.Time
}

// Tick runs one scheduler tick: scan agents, record missed slots, sweep
// stale running runs, probe backend state, and append a summary to tick.log.
func Tick(deps TickDeps) (TickOutcome, error) {
	cfg := deps.Cfg
	if cfg == nil || cfg.AosHome == "" {
		return TickOutcome{}, fmt.Errorf("aos not initialized")
	}

	scan, err := ScanAgents(filepath.Join(cfg.AosHome, "agents"))
	if err != nil {
		return TickOutcome{}, fmt.Errorf("scan agents: %w", err)
	}

	scheduled := 0
	for _, a := range scan.Agents {
		if a.Meta.Schedule != nil {
			scheduled++
		}
	}

	out := TickOutcome{
		Timestamp: deps.Now.UTC().Format(time.RFC3339),
		Scripts:   len(scan.Agents),
		Scheduled: scheduled,
		Backend:   "unknown",
	}

	missed, runs, missesErr := RecordMissedRuns(cfg.AosHome, scan.Agents, deps.Now)
	if missesErr != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf("record missed runs: %v", missesErr))
	}
	out.Missed = len(missed)

	store := NewFileRunStore(cfg.AosHome)
	if n, swErr := SweepStaleRunning(store, runs, deps.Now, StaleRunningThreshold); swErr != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf("sweep stale running: %v", swErr))
	} else {
		out.StaleResolved = n
	}

	interval, intervalErr := cfg.EffectiveTickInterval()
	if intervalErr != nil {
		out.Warnings = append(out.Warnings,
			fmt.Sprintf("%v; using default tick interval (%s)", intervalErr, config.DefaultTickInterval))
	}

	be, beErr := backend.Select(cfg.AosHome)
	if beErr != nil {
		out.Backend = "error(" + sanitizeForState(beErr.Error()) + ")"
	} else {
		spec, specWarn := buildBackendSpec(cfg.AosHome, scan.Agents, interval)
		if specWarn != "" {
			out.Warnings = append(out.Warnings, specWarn)
		}
		st, err := be.State(spec)
		if err != nil {
			out.Backend = "error(" + sanitizeForState(err.Error()) + ")"
		} else {
			out.Backend = string(st)
		}
	}

	if err := appendTickLog(filepath.Join(cfg.AosHome, "tick.log"), out.LogLine()); err != nil {
		return out, fmt.Errorf("write tick.log: %w", err)
	}
	return out, nil
}

// buildBackendSpec assembles the backend.Spec the tick's State drift check
// consumes. The second return value is a non-empty warning string when
// runtime.AosBinaryPath fails — tick surfaces it so its State observation
// stays consistent with refresh's (which records the same error explicitly).
// Without that, tick's spec ends up with an empty AosBinaryPath, the backend
// drops the tick job from `expected`, and State reports drift while refresh
// reports skipped — same machine, different verbs, contradictory output.
func buildBackendSpec(aosHome string, agents []Agent, interval time.Duration) (backend.Spec, string) {
	jobs := make([]backend.AgentJob, 0)
	for _, a := range agents {
		if a.Meta.Schedule == nil {
			continue
		}
		if len(a.Warnings) > 0 {
			continue
		}
		jobs = append(jobs, backend.AgentJob{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			Schedule:   *a.Meta.Schedule,
		})
	}
	aosBin, err := runtime.AosBinaryPath()
	warn := ""
	if err != nil {
		warn = fmt.Sprintf("resolve aos binary: %v", err)
	}
	return backend.Spec{
		Agents: jobs,
		Tick: backend.TickJob{
			AosBinaryPath: aosBin,
			LogPath:       filepath.Join(aosHome, "tick.log"),
			Interval:      interval,
		},
	}, warn
}

// sanitizeForState scrubs an error string so it can sit inside the
// "error(...)" wrapper used by the tick backend-state enum without breaking
// downstream string-tokenizers.
func sanitizeForState(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == ' ' {
			out = append(out, '_')
			continue
		}
		out = append(out, c)
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return string(out)
}

func appendTickLog(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}
