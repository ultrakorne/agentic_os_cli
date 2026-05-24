//go:build darwin

// macOS LaunchAgent backend. Plists live under ~/Library/LaunchAgents and are
// namespaced com.agenticos.*. LaunchAgents run inside the user's GUI session
// so `claude -p` can reach the login Keychain — the motivating reason aos
// moved off cron on macOS.
package backend

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"howett.net/plist"

	"github.com/ultrakorne/aos_cli/internal/scheduler/schedspec"
)

const (
	// LaunchdLabelPrefix is the LaunchAgent namespace aos owns.
	LaunchdLabelPrefix = "com.agenticos."
	// LaunchdTickLabel is the periodic `aos tick` LaunchAgent.
	LaunchdTickLabel = LaunchdLabelPrefix + "__tick__"
)

// LaunchdBackend is the macOS backend. Loader is swappable for tests.
type LaunchdBackend struct {
	aosHome string
	dir     string
	uid     int
	loader  LaunchdLoader
	now     func() time.Time
}

// LaunchdLoader abstracts the launchctl subprocess so tests can stub it.
type LaunchdLoader interface {
	Bootstrap(plistPath string) error
	Bootout(label string) error
	IsLoaded(label string) (bool, error)
	// Probe returns nil if the GUI domain is reachable. A real launchctl
	// failure (binary missing, no GUI session) returns an error.
	Probe() error
}

// NewLaunchd constructs a launchd backend rooted at the user's LaunchAgents dir.
func NewLaunchd(aosHome string) *LaunchdBackend {
	home, _ := os.UserHomeDir()
	return &LaunchdBackend{
		aosHome: aosHome,
		dir:     filepath.Join(home, "Library", "LaunchAgents"),
		uid:     os.Getuid(),
		loader:  realLaunchdLoader{uid: os.Getuid()},
		now:     time.Now,
	}
}

// WithLoader returns a copy with loader replaced. Tests inject a fake.
func (b *LaunchdBackend) WithLoader(l LaunchdLoader) *LaunchdBackend {
	cp := *b
	cp.loader = l
	return &cp
}

// WithDir overrides the LaunchAgents directory. Tests write into a temp dir.
func (b *LaunchdBackend) WithDir(dir string) *LaunchdBackend {
	cp := *b
	cp.dir = dir
	return &cp
}

// calendarEntry is one StartCalendarInterval dict. Hour and Weekday are
// pointers so omitting them produces "match every value" semantics.
type calendarEntry struct {
	Minute  *int `plist:"Minute,omitempty"`
	Hour    *int `plist:"Hour,omitempty"`
	Weekday *int `plist:"Weekday,omitempty"`
}

// launchdJob mirrors the launchd plist schema for both agent and tick jobs.
type launchdJob struct {
	Label                 string            `plist:"Label"`
	ProgramArguments      []string          `plist:"ProgramArguments"`
	StartCalendarInterval []calendarEntry   `plist:"StartCalendarInterval,omitempty"`
	StartInterval         int               `plist:"StartInterval,omitempty"`
	RunAtLoad             bool              `plist:"RunAtLoad"`
	StandardOutPath       string            `plist:"StandardOutPath"`
	StandardErrorPath     string            `plist:"StandardErrorPath"`
	EnvironmentVariables  map[string]string `plist:"EnvironmentVariables,omitempty"`
}

// LaunchdAgentLabel returns the LaunchAgent label for an aos agent id.
func LaunchdAgentLabel(agentID string) string { return LaunchdLabelPrefix + agentID }

// PlistPath returns the on-disk path for the given label.
func (b *LaunchdBackend) PlistPath(label string) string {
	return filepath.Join(b.dir, label+".plist")
}

func (b *LaunchdBackend) renderAgent(j AgentJob) (launchdJob, error) {
	intervals, err := launchdCalendarEntries(j.Schedule)
	if err != nil {
		return launchdJob{}, err
	}
	wrapper := filepath.Join(b.aosHome, "wrapper.sh")
	return launchdJob{
		Label: LaunchdAgentLabel(j.AgentID),
		// wrapper.sh takes no positional args; every value flows in via env.
		// Quoting concerns (special chars in paths) collapse to "set env var".
		ProgramArguments:      []string{wrapper},
		StartCalendarInterval: intervals,
		RunAtLoad:             false,
		StandardOutPath:       "/dev/null",
		StandardErrorPath:     "/dev/null",
		EnvironmentVariables: map[string]string{
			"AGENTIC_OS_DATA_DIR":     b.aosHome,
			"AGENTIC_OS_AGENT_ID":     j.AgentID,
			"AGENTIC_OS_AGENT_SCRIPT": j.ScriptPath,
			"AGENTIC_OS_TRIGGER":      "schedule",
		},
	}, nil
}

func (b *LaunchdBackend) renderTick(t TickJob) launchdJob {
	secs := int(t.Interval.Seconds())
	if secs < 1 {
		secs = 1
	}
	return launchdJob{
		Label:             LaunchdTickLabel,
		ProgramArguments:  []string{t.AosBinaryPath, "tick"},
		StartInterval:     secs,
		RunAtLoad:         false,
		StandardOutPath:   t.LogPath,
		StandardErrorPath: t.LogPath,
	}
}

func launchdCalendarEntries(s schedspec.ScheduleSpec) ([]calendarEntry, error) {
	switch s.Kind {
	case "hourly":
		if s.EveryHours < 1 || s.EveryHours > 12 || s.Minute < 0 || s.Minute > 59 {
			return nil, fmt.Errorf("invalid hourly schedule")
		}
		minute := s.Minute
		if s.EveryHours == 1 {
			return []calendarEntry{{Minute: &minute}}, nil
		}
		var out []calendarEntry
		for h := 0; h < 24; h += s.EveryHours {
			hh := h
			mm := minute
			out = append(out, calendarEntry{Minute: &mm, Hour: &hh})
		}
		return out, nil
	case "daily":
		if len(s.Days) == 0 || s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 {
			return nil, fmt.Errorf("invalid daily schedule")
		}
		days := make([]schedspec.Weekday, len(s.Days))
		copy(days, s.Days)
		sort.Slice(days, func(i, j int) bool {
			return launchdWeekdayIndex(days[i]) < launchdWeekdayIndex(days[j])
		})
		var out []calendarEntry
		for _, d := range days {
			wd := launchdWeekdayIndex(d)
			if wd < 0 {
				return nil, fmt.Errorf("unknown weekday %q", d)
			}
			hh := s.Hour
			mm := s.Minute
			w := wd
			out = append(out, calendarEntry{Minute: &mm, Hour: &hh, Weekday: &w})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown schedule kind %q", s.Kind)
	}
}

func launchdWeekdayIndex(d schedspec.Weekday) int {
	switch d {
	case schedspec.Sun:
		return 0
	case schedspec.Mon:
		return 1
	case schedspec.Tue:
		return 2
	case schedspec.Wed:
		return 3
	case schedspec.Thu:
		return 4
	case schedspec.Fri:
		return 5
	case schedspec.Sat:
		return 6
	}
	return -1
}

func marshalLaunchdPlist(j launchdJob) ([]byte, error) {
	var buf bytes.Buffer
	enc := plist.NewEncoderForFormat(&buf, plist.XMLFormat)
	enc.Indent("\t")
	if err := enc.Encode(j); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseLaunchdPlist(data []byte) (launchdJob, error) {
	var j launchdJob
	dec := plist.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&j); err != nil {
		return launchdJob{}, err
	}
	return j, nil
}

// Sync reconciles the LaunchAgents directory with spec.
func (b *LaunchdBackend) Sync(spec Spec) (SyncResult, error) {
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return SyncResult{}, fmt.Errorf("mkdir %s: %w", b.dir, err)
	}

	expected := map[string]launchdJob{}
	failed := []FailedJob{}
	for _, a := range spec.Agents {
		j, err := b.renderAgent(a)
		if err != nil {
			failed = append(failed, FailedJob{AgentID: a.AgentID, Reason: err.Error()})
			continue
		}
		expected[j.Label] = j
	}
	if spec.Tick.AosBinaryPath != "" && spec.Tick.Interval > 0 {
		expected[LaunchdTickLabel] = b.renderTick(spec.Tick)
	}

	existing, err := b.listManaged()
	if err != nil {
		return SyncResult{Failed: failed}, fmt.Errorf("list %s: %w", b.dir, err)
	}

	res := SyncResult{Failed: failed}

	for label, want := range expected {
		path := b.PlistPath(label)
		writeNeeded := true
		if data, err := os.ReadFile(path); err == nil {
			if got, perr := parseLaunchdPlist(data); perr == nil && launchdPlistEqual(got, want) {
				writeNeeded = false
			}
		}
		if !writeNeeded {
			// Content matches, but launchctl may have been told to bootout
			// the unit out-of-band (user debugging, partial uninstall). If
			// it isn't loaded, re-bootstrap so the agent actually fires.
			loaded, _ := b.loader.IsLoaded(label)
			if loaded {
				res.Unchanged++
				continue
			}
			if err := b.loader.Bootstrap(path); err != nil {
				res.Failed = append(res.Failed, FailedJob{AgentID: launchdLabelToAgentID(label), Reason: fmt.Sprintf("bootstrap: %v", err)})
				continue
			}
			res.Wrote++
			continue
		}
		buf, err := marshalLaunchdPlist(want)
		if err != nil {
			res.Failed = append(res.Failed, FailedJob{AgentID: launchdLabelToAgentID(label), Reason: err.Error()})
			continue
		}
		if loaded, _ := b.loader.IsLoaded(label); loaded {
			if err := b.loader.Bootout(label); err != nil {
				res.Failed = append(res.Failed, FailedJob{AgentID: launchdLabelToAgentID(label), Reason: fmt.Sprintf("bootout: %v", err)})
				continue
			}
		}
		if err := launchdAtomicWrite(path, buf); err != nil {
			res.Failed = append(res.Failed, FailedJob{AgentID: launchdLabelToAgentID(label), Reason: err.Error()})
			continue
		}
		if err := b.loader.Bootstrap(path); err != nil {
			res.Failed = append(res.Failed, FailedJob{AgentID: launchdLabelToAgentID(label), Reason: fmt.Sprintf("bootstrap: %v", err)})
			continue
		}
		res.Wrote++
	}

	for label, path := range existing {
		if _, ok := expected[label]; ok {
			continue
		}
		if loaded, _ := b.loader.IsLoaded(label); loaded {
			if err := b.loader.Bootout(label); err != nil {
				res.Failed = append(res.Failed, FailedJob{AgentID: launchdLabelToAgentID(label), Reason: fmt.Sprintf("bootout: %v", err)})
				continue
			}
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			res.Failed = append(res.Failed, FailedJob{AgentID: launchdLabelToAgentID(label), Reason: err.Error()})
			continue
		}
		res.Removed++
	}

	return res, nil
}

// Remove tears down every LaunchAgent in the namespace.
func (b *LaunchdBackend) Remove() error {
	existing, err := b.listManaged()
	if err != nil {
		return err
	}
	for label, path := range existing {
		if loaded, _ := b.loader.IsLoaded(label); loaded {
			_ = b.loader.Bootout(label)
		}
		_ = os.Remove(path)
	}
	return nil
}

// State reports drift between the on-disk plists and the spec.
func (b *LaunchdBackend) State(spec Spec) (State, error) {
	expected := map[string]launchdJob{}
	for _, a := range spec.Agents {
		j, err := b.renderAgent(a)
		if err != nil {
			continue
		}
		expected[j.Label] = j
	}
	if spec.Tick.AosBinaryPath != "" && spec.Tick.Interval > 0 {
		expected[LaunchdTickLabel] = b.renderTick(spec.Tick)
	}

	existing, err := b.listManaged()
	if err != nil {
		return StateDrift, err
	}
	if len(expected) == 0 && len(existing) == 0 {
		return StateEmpty, nil
	}
	if len(expected) != len(existing) {
		return StateDrift, nil
	}
	for label, want := range expected {
		path, ok := existing[label]
		if !ok {
			return StateDrift, nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return StateDrift, nil
		}
		got, err := parseLaunchdPlist(data)
		if err != nil {
			return StateDrift, nil
		}
		if !launchdPlistEqual(got, want) {
			return StateDrift, nil
		}
		// File matches the spec; verify the unit is actually loaded into
		// launchd. A user-issued `launchctl bootout` outside aos leaves the
		// file pristine but the agent silently dead — that's drift, not
		// managed.
		if loaded, _ := b.loader.IsLoaded(label); !loaded {
			return StateDrift, nil
		}
	}
	return StateManaged, nil
}

// Probe reports whether the user's launchd domain is reachable.
func (b *LaunchdBackend) Probe() error {
	return b.loader.Probe()
}

func (b *LaunchdBackend) listManaged() (map[string]string, error) {
	out := map[string]string{}
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".plist") {
			continue
		}
		label := strings.TrimSuffix(name, ".plist")
		if !strings.HasPrefix(label, LaunchdLabelPrefix) {
			continue
		}
		out[label] = filepath.Join(b.dir, name)
	}
	return out, nil
}

func launchdPlistEqual(a, b launchdJob) bool {
	if len(a.EnvironmentVariables) == 0 {
		a.EnvironmentVariables = nil
	}
	if len(b.EnvironmentVariables) == 0 {
		b.EnvironmentVariables = nil
	}
	return reflect.DeepEqual(a, b)
}

func launchdLabelToAgentID(label string) string {
	return strings.TrimPrefix(label, LaunchdLabelPrefix)
}

func launchdAtomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
