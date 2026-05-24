//go:build linux

// Linux systemd-user backend. Unit files live under
// ~/.config/systemd/user/ and are namespaced agentic-os-*. Timer units use
// Persistent=true so make-up-on-wake matches the cron-era behavior.
package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/ultrakorne/aos_cli/internal/scheduler/schedspec"
)

const (
	// SystemdUnitPrefix is the systemd-user namespace aos owns.
	SystemdUnitPrefix = "agentic-os-"
	// SystemdTickUnit is the periodic `aos tick` timer base name.
	SystemdTickUnit = SystemdUnitPrefix + "tick"
)

// SystemdBackend installs systemd-user timer+service pairs.
type SystemdBackend struct {
	aosHome string
	dir     string
	loader  SystemdLoader
}

// SystemdLoader abstracts systemctl --user so tests can stub it.
type SystemdLoader interface {
	DaemonReload() error
	Enable(unitName string) error
	Disable(unitName string) error
	IsActive(unitName string) (bool, error)
	// Probe returns nil if the user manager is reachable.
	Probe() error
}

// NewSystemd constructs a backend rooted at ~/.config/systemd/user.
func NewSystemd(aosHome string) *SystemdBackend {
	home, _ := os.UserHomeDir()
	return &SystemdBackend{
		aosHome: aosHome,
		dir:     filepath.Join(home, ".config", "systemd", "user"),
		loader:  realSystemdLoader{},
	}
}

func (b *SystemdBackend) WithLoader(l SystemdLoader) *SystemdBackend {
	cp := *b
	cp.loader = l
	return &cp
}

func (b *SystemdBackend) WithDir(dir string) *SystemdBackend {
	cp := *b
	cp.dir = dir
	return &cp
}

// systemdUnit is the parsed view of an INI file: section → ordered (key,value)
// pairs. Insertion order is preserved in the slice form so write paths are
// reproducible.
type systemdUnit struct {
	sections []systemdSection
}

type systemdSection struct {
	name   string
	values []systemdKV
}

type systemdKV struct {
	key   string
	value string
}

// SystemdAgentBaseName is the unit stem for an agent. Both .timer and
// .service files use this stem.
func SystemdAgentBaseName(agentID string) string {
	return SystemdUnitPrefix + agentID
}

// renderAgent builds the .service+.timer unit content for one agent job.
func (b *SystemdBackend) renderAgent(j AgentJob) (service, timer systemdUnit, err error) {
	onCalendar, err := systemdOnCalendar(j.Schedule)
	if err != nil {
		return systemdUnit{}, systemdUnit{}, err
	}
	wrapper := filepath.Join(b.aosHome, "wrapper.sh")
	base := SystemdAgentBaseName(j.AgentID)
	service = systemdUnit{
		sections: []systemdSection{
			{name: "Unit", values: []systemdKV{{"Description", "aos agent " + j.AgentID}}},
			// wrapper.sh takes no positional args; every value flows in via
			// Environment= so we don't have to worry about systemd's ExecStart
			// quoting rules vs special chars in paths.
			{name: "Service", values: []systemdKV{
				{"Type", "oneshot"},
				{"ExecStart", systemdShellQuote(wrapper)},
				{"Environment", systemdEnvQuote("AGENTIC_OS_DATA_DIR", b.aosHome)},
				{"Environment", systemdEnvQuote("AGENTIC_OS_AGENT_ID", j.AgentID)},
				{"Environment", systemdEnvQuote("AGENTIC_OS_AGENT_SCRIPT", j.ScriptPath)},
				{"Environment", "AGENTIC_OS_TRIGGER=schedule"},
				{"StandardOutput", "null"},
				{"StandardError", "null"},
			}},
		},
	}
	timer = systemdUnit{
		sections: []systemdSection{
			{name: "Unit", values: []systemdKV{{"Description", "aos agent " + j.AgentID + " timer"}}},
			{name: "Timer", values: []systemdKV{
				{"OnCalendar", onCalendar},
				{"Persistent", "true"},
				{"Unit", base + ".service"},
			}},
			{name: "Install", values: []systemdKV{{"WantedBy", "timers.target"}}},
		},
	}
	return service, timer, nil
}

// renderTick builds the .service+.timer for the periodic tick.
func (b *SystemdBackend) renderTick(t TickJob) (service, timer systemdUnit) {
	service = systemdUnit{
		sections: []systemdSection{
			{name: "Unit", values: []systemdKV{{"Description", "aos scheduler tick"}}},
			{name: "Service", values: []systemdKV{
				{"Type", "oneshot"},
				{"ExecStart", fmt.Sprintf("%s tick", systemdShellQuote(t.AosBinaryPath))},
				{"StandardOutput", "append:" + t.LogPath},
				{"StandardError", "append:" + t.LogPath},
			}},
		},
	}
	timer = systemdUnit{
		sections: []systemdSection{
			{name: "Unit", values: []systemdKV{{"Description", "aos scheduler tick timer"}}},
			{name: "Timer", values: []systemdKV{
				{"OnBootSec", "10s"},
				{"OnUnitActiveSec", fmt.Sprintf("%ds", int(t.Interval.Seconds()))},
				{"Persistent", "true"},
				{"Unit", SystemdTickUnit + ".service"},
			}},
			{name: "Install", values: []systemdKV{{"WantedBy", "timers.target"}}},
		},
	}
	return service, timer
}

// systemdOnCalendar maps a ScheduleSpec to a systemd OnCalendar string.
func systemdOnCalendar(s schedspec.ScheduleSpec) (string, error) {
	switch s.Kind {
	case "hourly":
		if s.EveryHours < 1 || s.EveryHours > 12 || s.Minute < 0 || s.Minute > 59 {
			return "", fmt.Errorf("invalid hourly schedule")
		}
		if s.EveryHours == 1 {
			return fmt.Sprintf("*-*-* *:%02d:00", s.Minute), nil
		}
		return fmt.Sprintf("*-*-* 00/%d:%02d:00", s.EveryHours, s.Minute), nil
	case "daily":
		if len(s.Days) == 0 || s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 {
			return "", fmt.Errorf("invalid daily schedule")
		}
		days := make([]schedspec.Weekday, len(s.Days))
		copy(days, s.Days)
		sort.Slice(days, func(i, j int) bool {
			return systemdWeekdayOrd(days[i]) < systemdWeekdayOrd(days[j])
		})
		parts := make([]string, len(days))
		for i, d := range days {
			n, err := systemdWeekdayName(d)
			if err != nil {
				return "", err
			}
			parts[i] = n
		}
		return fmt.Sprintf("%s *-*-* %02d:%02d:00", strings.Join(parts, ","), s.Hour, s.Minute), nil
	default:
		return "", fmt.Errorf("unknown schedule kind %q", s.Kind)
	}
}

func systemdWeekdayName(d schedspec.Weekday) (string, error) {
	switch d {
	case schedspec.Mon:
		return "Mon", nil
	case schedspec.Tue:
		return "Tue", nil
	case schedspec.Wed:
		return "Wed", nil
	case schedspec.Thu:
		return "Thu", nil
	case schedspec.Fri:
		return "Fri", nil
	case schedspec.Sat:
		return "Sat", nil
	case schedspec.Sun:
		return "Sun", nil
	}
	return "", fmt.Errorf("unknown weekday %q", d)
}

func systemdWeekdayOrd(d schedspec.Weekday) int {
	// Match systemd OnCalendar ordering convention (Mon=1 .. Sun=7).
	switch d {
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
	case schedspec.Sun:
		return 7
	}
	return 99
}

// systemdShellQuote double-quotes a string for systemd's ExecStart parser.
// Backslash escapes ARE processed inside double quotes per systemd.exec(5),
// so escape `\` and `"`. Single quotes pass through unchanged.
func systemdShellQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// systemdEnvQuote wraps a single KEY=value pair in double quotes with
// backslash escapes so an embedded space, tab, `$`, `"`, or `\` doesn't
// confuse systemd's Environment= parser. The whole KEY=value goes inside one
// pair of quotes (not just the value) because Environment= is
// space-separated and an unbalanced quote on either side breaks the parse.
func systemdEnvQuote(key, value string) string {
	v := strings.ReplaceAll(value, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return `"` + key + `=` + v + `"`
}

// marshalUnit renders a systemdUnit to canonical INI bytes.
func marshalUnit(u systemdUnit) []byte {
	var b strings.Builder
	for i, sec := range u.sections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("[")
		b.WriteString(sec.name)
		b.WriteString("]\n")
		for _, kv := range sec.values {
			b.WriteString(kv.key)
			b.WriteString("=")
			b.WriteString(kv.value)
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

// parseUnit parses a minimal INI file: section headers + key=value lines.
// Blank lines and # / ; comments are skipped; continuation lines are not
// supported.
func parseUnit(data []byte) systemdUnit {
	var u systemdUnit
	var cur *systemdSection
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			u.sections = append(u.sections, systemdSection{name: line[1 : len(line)-1]})
			cur = &u.sections[len(u.sections)-1]
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 || cur == nil {
			continue
		}
		cur.values = append(cur.values, systemdKV{
			key:   strings.TrimSpace(line[:eq]),
			value: strings.TrimSpace(line[eq+1:]),
		})
	}
	return u
}

// Sync reconciles the systemd-user unit directory with spec.
func (b *SystemdBackend) Sync(spec Spec) (SyncResult, error) {
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return SyncResult{}, fmt.Errorf("mkdir %s: %w", b.dir, err)
	}

	type unitPair struct {
		service systemdUnit
		timer   systemdUnit
	}
	expected := map[string]unitPair{}
	failed := []FailedJob{}
	for _, a := range spec.Agents {
		svc, tmr, err := b.renderAgent(a)
		if err != nil {
			failed = append(failed, FailedJob{AgentID: a.AgentID, Reason: err.Error()})
			continue
		}
		expected[SystemdAgentBaseName(a.AgentID)] = unitPair{service: svc, timer: tmr}
	}
	if spec.Tick.AosBinaryPath != "" && spec.Tick.Interval > 0 {
		svc, tmr := b.renderTick(spec.Tick)
		expected[SystemdTickUnit] = unitPair{service: svc, timer: tmr}
	}

	existing, err := b.listManaged()
	if err != nil {
		return SyncResult{Failed: failed}, fmt.Errorf("list %s: %w", b.dir, err)
	}

	res := SyncResult{Failed: failed}
	changed := false

	for base, want := range expected {
		svcPath := filepath.Join(b.dir, base+".service")
		tmrPath := filepath.Join(b.dir, base+".timer")
		changedHere := false
		if !unitsEqualOnDisk(svcPath, want.service) {
			if err := systemdAtomicWrite(svcPath, marshalUnit(want.service)); err != nil {
				res.Failed = append(res.Failed, FailedJob{AgentID: systemdBaseToAgentID(base), Reason: err.Error()})
				continue
			}
			changedHere = true
		}
		if !unitsEqualOnDisk(tmrPath, want.timer) {
			if err := systemdAtomicWrite(tmrPath, marshalUnit(want.timer)); err != nil {
				res.Failed = append(res.Failed, FailedJob{AgentID: systemdBaseToAgentID(base), Reason: err.Error()})
				continue
			}
			changedHere = true
		}
		if changedHere {
			res.Wrote++
			changed = true
		} else {
			res.Unchanged++
		}
	}

	// Orphans: anything in our namespace not in expected.
	type removal struct {
		base string
	}
	var removals []removal
	for base := range existing {
		if _, ok := expected[base]; ok {
			continue
		}
		removals = append(removals, removal{base: base})
	}

	if changed || len(removals) > 0 {
		if err := b.loader.DaemonReload(); err != nil {
			return res, fmt.Errorf("daemon-reload: %w", err)
		}
	}

	for _, r := range removals {
		_ = b.loader.Disable(r.base + ".timer")
		for _, suf := range []string{".service", ".timer"} {
			p := filepath.Join(b.dir, r.base+suf)
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				res.Failed = append(res.Failed, FailedJob{AgentID: systemdBaseToAgentID(r.base), Reason: err.Error()})
				continue
			}
		}
		res.Removed++
	}

	for base := range expected {
		// enable --now is idempotent — safe to call on every Sync.
		if err := b.loader.Enable(base + ".timer"); err != nil {
			res.Failed = append(res.Failed, FailedJob{AgentID: systemdBaseToAgentID(base), Reason: fmt.Sprintf("enable: %v", err)})
		}
	}

	if len(removals) > 0 {
		_ = b.loader.DaemonReload()
	}

	return res, nil
}

func unitsEqualOnDisk(path string, want systemdUnit) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	got := parseUnit(data)
	return reflect.DeepEqual(got, want)
}

// Remove disables and deletes every namespaced unit.
func (b *SystemdBackend) Remove() error {
	existing, err := b.listManaged()
	if err != nil {
		return err
	}
	for base := range existing {
		_ = b.loader.Disable(base + ".timer")
	}
	for base := range existing {
		for _, suf := range []string{".service", ".timer"} {
			_ = os.Remove(filepath.Join(b.dir, base+suf))
		}
	}
	if len(existing) > 0 {
		_ = b.loader.DaemonReload()
	}
	return nil
}

// State reports drift between the on-disk units and the spec.
func (b *SystemdBackend) State(spec Spec) (State, error) {
	type unitPair struct {
		service systemdUnit
		timer   systemdUnit
	}
	expected := map[string]unitPair{}
	for _, a := range spec.Agents {
		svc, tmr, err := b.renderAgent(a)
		if err != nil {
			continue
		}
		expected[SystemdAgentBaseName(a.AgentID)] = unitPair{service: svc, timer: tmr}
	}
	if spec.Tick.AosBinaryPath != "" && spec.Tick.Interval > 0 {
		svc, tmr := b.renderTick(spec.Tick)
		expected[SystemdTickUnit] = unitPair{service: svc, timer: tmr}
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
	for base, want := range expected {
		if _, ok := existing[base]; !ok {
			return StateDrift, nil
		}
		if !unitsEqualOnDisk(filepath.Join(b.dir, base+".service"), want.service) {
			return StateDrift, nil
		}
		if !unitsEqualOnDisk(filepath.Join(b.dir, base+".timer"), want.timer) {
			return StateDrift, nil
		}
		// Files match; the timer also has to be active. A user-issued
		// `systemctl --user disable` outside aos leaves files pristine but
		// the unit silently dead. Sync would heal on next refresh, but
		// State must surface the gap so `aos tick`'s log says so.
		if active, _ := b.loader.IsActive(base + ".timer"); !active {
			return StateDrift, nil
		}
	}
	return StateManaged, nil
}

// Probe reports whether the systemd-user manager is reachable.
func (b *SystemdBackend) Probe() error {
	return b.loader.Probe()
}

// listManaged returns base → present-on-disk for every aos-prefixed unit.
// A base is "present" only when both .service and .timer exist.
func (b *SystemdBackend) listManaged() (map[string]bool, error) {
	out := map[string]bool{}
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	seenSvc := map[string]bool{}
	seenTmr := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch {
		case strings.HasSuffix(name, ".service"):
			base := strings.TrimSuffix(name, ".service")
			if !strings.HasPrefix(base, SystemdUnitPrefix) {
				continue
			}
			seenSvc[base] = true
		case strings.HasSuffix(name, ".timer"):
			base := strings.TrimSuffix(name, ".timer")
			if !strings.HasPrefix(base, SystemdUnitPrefix) {
				continue
			}
			seenTmr[base] = true
		}
	}
	for base := range seenSvc {
		if seenTmr[base] {
			out[base] = true
		}
	}
	// Standalone timer/service halves: surface as orphans so Sync cleans them.
	for base := range seenTmr {
		if !seenSvc[base] {
			out[base] = true
		}
	}
	for base := range seenSvc {
		if !seenTmr[base] {
			out[base] = true
		}
	}
	return out, nil
}

func systemdBaseToAgentID(base string) string {
	return strings.TrimPrefix(base, SystemdUnitPrefix)
}

func systemdAtomicWrite(path string, data []byte) error {
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
