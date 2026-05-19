package crontab

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	BeginMarker = "# BEGIN agentic_os (managed - do not edit)"
	EndMarker   = "# END agentic_os"

	TickMarkerID = "__tick__"
)

type Entry struct {
	AgentID    string
	ScriptPath string
	Expression string // pre-compiled cron expression
}

type ManagedExtract struct {
	Before    string
	Managed   []string
	After     string
	HasMarker bool
	Conflict  bool
}

type SyncResult struct {
	Wrote    bool
	Conflict bool
	Reason   string
}

func ReadCrontab() (string, error) {
	cmd := exec.Command("crontab", "-l")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	se := stderr.String()
	if strings.Contains(strings.ToLower(se), "no crontab") {
		return "", nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 && stdout.Len() == 0 {
		// some `crontab -l` impls exit 1 with no output when no crontab.
		return "", nil
	}
	return "", fmt.Errorf("crontab -l: %w (%s)", err, strings.TrimSpace(se))
}

func WriteCrontab(text string) error {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("crontab -: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func ExtractManaged(text string) ManagedExtract {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	beginIdx, endIdx := -1, -1
	beginCount, endCount := 0, 0
	inBlock := false
	conflict := false
	managed := []string{}

	for i, raw := range lines {
		t := strings.TrimSpace(raw)
		switch {
		case t == BeginMarker:
			beginCount++
			if inBlock {
				conflict = true
				continue
			}
			inBlock = true
			if beginIdx < 0 {
				beginIdx = i
			}
		case t == EndMarker:
			endCount++
			if !inBlock {
				conflict = true
				continue
			}
			inBlock = false
			endIdx = i
		default:
			if inBlock {
				managed = append(managed, raw)
			}
		}
	}
	if inBlock {
		conflict = true
	}
	if beginCount > 1 || endCount > 1 {
		conflict = true
	}
	hasMarker := beginIdx >= 0 && endIdx >= 0 && !conflict
	var before, after string
	if hasMarker {
		before = strings.Join(lines[:beginIdx], "\n")
		after = strings.Join(lines[endIdx+1:], "\n")
	} else {
		before = strings.Join(lines, "\n")
		after = ""
	}
	return ManagedExtract{
		Before:    before,
		Managed:   managed,
		After:     after,
		HasMarker: hasMarker,
		Conflict:  conflict,
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildManagedBlock formats the managed crontab block. tickSchedule is the
// cron expression for the __tick__ entry (typically the result of
// TickCronExpr); it is only emitted when both tickSchedule and tickCmd are
// non-empty.
func BuildManagedBlock(entries []Entry, wrapperPath, dataDir, tickSchedule, tickCmd string) string {
	var b strings.Builder
	b.WriteString(BeginMarker)
	if tickCmd != "" && tickSchedule != "" {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("%s %s # agentic_os:%s", tickSchedule, tickCmd, TickMarkerID))
	}
	for _, e := range entries {
		b.WriteString("\n")
		cmd := strings.Join([]string{
			shellQuote(wrapperPath),
			shellQuote(dataDir),
			shellQuote(e.AgentID),
			shellQuote(e.AgentID),
			shellQuote(e.ScriptPath),
		}, " ")
		b.WriteString(fmt.Sprintf("%s %s # agentic_os:%s", e.Expression, cmd, e.AgentID))
	}
	b.WriteString("\n")
	b.WriteString(EndMarker)
	return b.String()
}

// BuildTickCommand returns the cron command line (after the schedule) for the
// __tick__ entry. aosBin must be the absolute path to the aos binary so the
// command works under cron's minimal PATH.
func BuildTickCommand(aosBin, dataDir string) string {
	logPath := filepath.Join(dataDir, "tick.log")
	return fmt.Sprintf("%s tick >> %s 2>&1", shellQuote(aosBin), shellQuote(logPath))
}

type SyncArgs struct {
	Entries      []Entry
	WrapperPath  string
	DataDir      string
	TickSchedule string
	TickCommand  string
	Force        bool
}

// SyncCrontab rebuilds the managed block from the given entries and writes
// the crontab if the result differs from what is currently installed. When
// entries is empty AND tickCommand is empty, the managed block is removed
// entirely.
func SyncCrontab(args SyncArgs) (SyncResult, error) {
	release, err := acquireLock(args.DataDir)
	if err != nil {
		return SyncResult{}, err
	}
	if release == nil {
		return SyncResult{Reason: "crontab lock contended"}, nil
	}
	defer release()

	current, err := ReadCrontab()
	if err != nil {
		return SyncResult{}, err
	}
	ex := ExtractManaged(current)
	if ex.Conflict && !args.Force {
		return SyncResult{Conflict: true, Reason: "managed section damaged or duplicated"}, nil
	}
	base := current
	baseEx := ex
	if ex.Conflict {
		base = purgeAllManaged(current)
		baseEx = ExtractManaged(base)
	}

	next := computeNext(base, baseEx, args.Entries, args.WrapperPath, args.DataDir, args.TickSchedule, args.TickCommand)
	if next == current {
		return SyncResult{}, nil
	}
	if err := WriteCrontab(next); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Wrote: true}, nil
}

// MatchesTarget reports whether `current` already contains the managed block
// SyncCrontab would write for the same args. Drift detection should call this
// instead of reassembling the expected block from markers — so detect and sync
// share the same comparison and cannot disagree.
func MatchesTarget(current string, args SyncArgs) bool {
	ex := ExtractManaged(current)
	if ex.Conflict {
		return false
	}
	next := computeNext(current, ex, args.Entries, args.WrapperPath, args.DataDir, args.TickSchedule, args.TickCommand)
	return next == current
}

// RemoveManaged strips the managed block entirely. Returns Wrote=false when
// no managed block was present.
func RemoveManaged() (SyncResult, error) {
	current, err := ReadCrontab()
	if err != nil {
		return SyncResult{}, err
	}
	ex := ExtractManaged(current)
	if !ex.HasMarker && !ex.Conflict {
		return SyncResult{}, nil
	}
	cleaned := purgeAllManaged(current)
	if cleaned == current {
		return SyncResult{}, nil
	}
	if err := WriteCrontab(cleaned); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Wrote: true}, nil
}

func computeNext(current string, ex ManagedExtract, entries []Entry, wrapperPath, dataDir, tickSchedule, tickCmd string) string {
	if len(entries) == 0 && tickCmd == "" {
		if !ex.HasMarker {
			return current
		}
		before := strings.TrimRight(ex.Before, "\n")
		after := strings.TrimLeft(ex.After, "\n")
		if before == "" {
			return after
		}
		if after == "" {
			return before + "\n"
		}
		return before + "\n" + after
	}
	block := BuildManagedBlock(entries, wrapperPath, dataDir, tickSchedule, tickCmd)
	if ex.HasMarker {
		before := strings.TrimRight(ex.Before, "\n")
		after := strings.TrimLeft(ex.After, "\n")
		parts := []string{}
		if before != "" {
			parts = append(parts, before)
		}
		parts = append(parts, block)
		if after != "" {
			parts = append(parts, after)
		}
		return strings.Join(parts, "\n") + "\n"
	}
	trimmed := strings.TrimRight(current, "\n")
	if trimmed != "" {
		return trimmed + "\n" + block + "\n"
	}
	return block + "\n"
}

func purgeAllManaged(text string) string {
	lines := strings.Split(text, "\n")
	keep := make([]bool, len(lines))
	for i := range keep {
		keep[i] = true
	}
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == BeginMarker {
			j := i + 1
			for j < len(lines) && strings.TrimSpace(lines[j]) != EndMarker {
				j++
			}
			if j < len(lines) {
				for k := i; k <= j; k++ {
					keep[k] = false
				}
				i = j + 1
				continue
			}
			keep[i] = false
		} else if t == EndMarker {
			keep[i] = false
		}
		i++
	}
	var out []string
	for idx, l := range lines {
		if keep[idx] {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// timestamp helper exposed for callers that may want fresh "now" injection.
var nowFn = time.Now
