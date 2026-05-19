package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/crontab"
	"github.com/ultrakorne/aos_cli/internal/runtime"
)

// TickOutcome is the structured result of one scheduler tick. The on-disk
// tick.log keeps its historical "[tick] ..." prefix (dashboard tail consumers
// depend on it); this struct is the JSON wire format under --json.
//
// Missed counts miss records *newly written this tick*, not currently
// outstanding. Catchups counts wrappers *spawned this tick*. Warnings holds
// non-fatal step errors (miss-recording, catch-up spawn, tick-interval parse).
type TickOutcome struct {
	Timestamp string   `json:"timestamp"`
	Scripts   int      `json:"scripts"`
	Scheduled int      `json:"scheduled"`
	Missed    int      `json:"missed"`
	Catchups  int      `json:"catchups"`
	Crontab   string   `json:"crontab"`
	Warnings  []string `json:"warnings,omitempty"`
}

// LogLine returns the single line the on-disk tick.log receives. Verb callers
// can also print this to stdout when --json is not set, so the cron tail and
// human verb output agree byte-for-byte.
func (t TickOutcome) LogLine() string {
	return fmt.Sprintf("[tick] %s scripts=%d scheduled=%d missed=%d catchups=%d crontab=%s\n",
		t.Timestamp, t.Scripts, t.Scheduled, t.Missed, t.Catchups, t.Crontab)
}

type TickDeps struct {
	Cfg *config.Config
	Now time.Time
}

// Tick runs one scheduler tick: scan agents, record missed slots, fire
// catch-ups, compute crontab drift state, and append a summary to tick.log.
// Returns the outcome regardless of whether the log write succeeded — only
// pre-scan failures (no config, missing aos_home, scan error) produce an
// error.
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
	}

	missed, runs, missesErr := RecordMissedRuns(cfg.AosHome, scan.Agents, deps.Now)
	if missesErr != nil {
		// Don't fail the tick — the runs/cron side of the world is still
		// authoritative even if a miss record didn't land this round.
		out.Warnings = append(out.Warnings, fmt.Sprintf("record missed runs: %v", missesErr))
	}
	out.Missed = len(missed)

	if cfg.EffectiveCatchupEnabled() {
		// Reuse the runs slice RecordMissedRuns returned — it already
		// reflects this tick's writes, so fireCatchups doesn't need a
		// second load of the (potentially 2000-entry) runs/ dir.
		store := NewFileRunStore(cfg.AosHome)
		fired, warns, err := fireCatchups(cfg.AosHome, store, scan.Agents, runs)
		if err != nil {
			// Spawn failures don't fail the tick — same posture as miss
			// recording. The next tick retries.
			out.Warnings = append(out.Warnings, fmt.Sprintf("fire catch-ups: %v", err))
		}
		out.Warnings = append(out.Warnings, warns...)
		out.Catchups = fired
	}

	tickSchedule, tickErr := cfg.EffectiveTickCronExpr()
	if tickErr != nil {
		// Same posture as refresh: log and fall back to the default cadence
		// returned by EffectiveTickCronExpr. The next refresh will rewrite
		// the cron block once the user fixes config.toml.
		out.Warnings = append(out.Warnings,
			fmt.Sprintf("%v; using default tick interval (%s)", tickErr, config.DefaultTickInterval))
	}
	out.Crontab = crontabState(cfg.AosHome, scan.Agents, tickSchedule)

	if err := appendTickLog(filepath.Join(cfg.AosHome, "tick.log"), out.LogLine()); err != nil {
		return out, fmt.Errorf("write tick.log: %w", err)
	}
	return out, nil
}

// fireCatchups inspects per-agent latest run state and spawns wrapper.sh with
// AGENTIC_OS_TRIGGER=catch-up for every agent whose latest run is missed.
// Returns (fired, perAgentWarnings, fatalErr). Each spawn failure is captured
// as a warning but does not short-circuit the loop — one broken agent
// shouldn't block catch-ups for siblings. A missing wrapper is fatal because
// nothing can be spawned at all.
//
// `runs` is the post-RecordMissedRuns view of the runs/ directory — pass it
// through from Tick so a 2000-file directory isn't walked twice per tick.
func fireCatchups(aosHome string, store *FileRunStore, agents []Agent, runs []Run) (int, []string, error) {
	wrapperPath := filepath.Join(aosHome, "wrapper.sh")
	if !runtime.FileExists(wrapperPath) || !runtime.IsExecutable(wrapperPath) {
		// Mirrors aos run's posture: without a usable wrapper we can't spawn
		// anything. Treat as a soft error so the tick's other work still lands.
		return 0, nil, fmt.Errorf("%s missing or not executable", wrapperPath)
	}
	candidates := DetectCatchups(agents, runs)
	var warnings []string
	fired := 0
	for _, c := range candidates {
		err := SpawnWrapperDetached(wrapperPath, SpawnOpts{
			AosHome:    aosHome,
			ScheduleID: c.MissedSlot,
			AgentID:    c.AgentID,
			ScriptPath: c.ScriptPath,
			RunID:      store.NewID(),
			Trigger:    "catch-up",
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("catch-up %s: %v", c.AgentID, err))
			continue
		}
		fired++
	}
	return fired, warnings, nil
}

// crontabState returns one of: managed | empty | conflict | drift | error(<msg>).
// "drift" means: a managed block exists, but rebuilding it from the live
// agents would produce a different block. tickSchedule is the configured tick
// cron expression — passing it lets the drift check notice when the on-disk
// block still references the previous schedule after the user edits
// config.toml.
func crontabState(dataDir string, agents []Agent, tickSchedule string) string {
	if !runtime.HasBin("crontab") {
		return "error(no-crontab-bin)"
	}
	text, err := crontab.ReadCrontab()
	if err != nil {
		return "error(" + sanitizeForState(err.Error()) + ")"
	}
	ex := crontab.ExtractManaged(text)
	if ex.Conflict {
		return "conflict"
	}
	if !ex.HasMarker {
		if len(scheduledOnly(agents)) == 0 {
			return "empty"
		}
		return "drift"
	}

	wrapperPath := filepath.Join(dataDir, "wrapper.sh")
	entries := make([]crontab.Entry, 0)
	for _, a := range scheduledOnly(agents) {
		expr, err := CompileToCron(*a.Meta.Schedule)
		if err != nil {
			continue
		}
		entries = append(entries, crontab.Entry{
			AgentID:    a.ID,
			ScriptPath: a.ScriptPath,
			Expression: expr,
		})
	}
	aosBin, err := runtime.AosBinaryPath()
	if err != nil {
		return "error(" + sanitizeForState(err.Error()) + ")"
	}
	expectedBlock := crontab.BuildManagedBlock(entries, wrapperPath, dataDir, tickSchedule, crontab.BuildTickCommand(aosBin, dataDir))
	actualBlock := crontab.BeginMarker + "\n" + strings.Join(ex.Managed, "\n") + "\n" + crontab.EndMarker
	if actualBlock == expectedBlock {
		return "managed"
	}
	return "drift"
}

func scheduledOnly(agents []Agent) []Agent {
	out := make([]Agent, 0)
	for _, a := range agents {
		if a.Meta.Schedule != nil {
			out = append(out, a)
		}
	}
	return out
}

// sanitizeForState scrubs an error string so it can sit inside the
// "error(...)" wrapper used by the tick crontab-state enum without breaking
// downstream string-tokenizers. Same shape the cmd-layer used before.
func sanitizeForState(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
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
