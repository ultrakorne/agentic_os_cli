//go:build linux

package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ultrakorne/aos_cli/internal/scheduler/schedspec"
)

type fakeSystemdLoader struct {
	reloaded int
	enabled  []string
	disabled []string
	active   map[string]bool
	probeErr error
}

func newFakeSystemd() *fakeSystemdLoader { return &fakeSystemdLoader{active: map[string]bool{}} }

func (f *fakeSystemdLoader) DaemonReload() error { f.reloaded++; return nil }
func (f *fakeSystemdLoader) Enable(u string) error {
	f.enabled = append(f.enabled, u)
	f.active[u] = true
	return nil
}
func (f *fakeSystemdLoader) Disable(u string) error {
	f.disabled = append(f.disabled, u)
	delete(f.active, u)
	return nil
}
func (f *fakeSystemdLoader) IsActive(u string) (bool, error) { return f.active[u], nil }
func (f *fakeSystemdLoader) Probe() error                    { return f.probeErr }

func newSystemdBackend(t *testing.T) (*SystemdBackend, *fakeSystemdLoader, string) {
	t.Helper()
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	if err := os.MkdirAll(aosHome, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir := filepath.Join(tmp, "systemd-user")
	fake := newFakeSystemd()
	b := NewSystemd(aosHome).WithDir(dir).WithLoader(fake)
	return b, fake, aosHome
}

// TestSystemdState_driftWhenTimerInactive covers the silent-dead case: unit
// files on disk match the spec, but the timer was disabled out-of-band.
// State must surface this as drift so `aos tick` doesn't claim managed.
func TestSystemdState_driftWhenTimerInactive(t *testing.T) {
	b, fake, aosHome := newSystemdBackend(t)
	spec := Spec{
		Agents: []AgentJob{{
			AgentID:    "planner",
			ScriptPath: filepath.Join(aosHome, "planner.sh"),
			Schedule:   schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 5},
		}},
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// Simulate external `systemctl --user disable agentic-os-planner.timer`.
	delete(fake.active, "agentic-os-planner.timer")

	st, err := b.State(spec)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st != StateDrift {
		t.Errorf("State = %s, want drift (files match but timer inactive)", st)
	}
}

func TestSystemdSync_writesAndEnablesAgent(t *testing.T) {
	b, fake, aosHome := newSystemdBackend(t)
	spec := Spec{
		Agents: []AgentJob{{
			AgentID: "planner", ScriptPath: filepath.Join(aosHome, "planner.sh"),
			Schedule: schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 5},
		}},
	}
	res, err := b.Sync(spec)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Wrote != 1 {
		t.Errorf("Sync result = %+v, want Wrote=1", res)
	}
	if fake.reloaded == 0 {
		t.Errorf("daemon-reload not called")
	}
	if len(fake.enabled) != 1 || fake.enabled[0] != "agentic-os-planner.timer" {
		t.Errorf("enabled = %v", fake.enabled)
	}
	svc := filepath.Join(b.dir, "agentic-os-planner.service")
	tmr := filepath.Join(b.dir, "agentic-os-planner.timer")
	if _, err := os.Stat(svc); err != nil {
		t.Errorf("service not written: %v", err)
	}
	if _, err := os.Stat(tmr); err != nil {
		t.Errorf("timer not written: %v", err)
	}
	tmrData, _ := os.ReadFile(tmr)
	if !strings.Contains(string(tmrData), "OnCalendar=*-*-* *:05:00") {
		t.Errorf("timer OnCalendar wrong:\n%s", tmrData)
	}
}

func TestSystemdSync_unchangedRerun(t *testing.T) {
	b, fake, aosHome := newSystemdBackend(t)
	spec := Spec{
		Agents: []AgentJob{{
			AgentID: "planner", ScriptPath: filepath.Join(aosHome, "planner.sh"),
			Schedule: schedspec.ScheduleSpec{Kind: "daily", Days: []schedspec.Weekday{schedspec.Mon, schedspec.Tue}, Hour: 9, Minute: 0},
		}},
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	reloadsAfterFirst := fake.reloaded
	res, err := b.Sync(spec)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if res.Wrote != 0 || res.Unchanged != 1 {
		t.Errorf("expected unchanged on rerun, got %+v", res)
	}
	if fake.reloaded != reloadsAfterFirst {
		t.Errorf("unnecessary daemon-reload on unchanged Sync")
	}
}

func TestSystemdSync_removesOrphans(t *testing.T) {
	b, fake, aosHome := newSystemdBackend(t)
	spec := Spec{
		Agents: []AgentJob{
			{AgentID: "alpha", ScriptPath: filepath.Join(aosHome, "alpha.sh"), Schedule: schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0}},
			{AgentID: "bravo", ScriptPath: filepath.Join(aosHome, "bravo.sh"), Schedule: schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 30}},
		},
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	spec.Agents = spec.Agents[:1]
	res, err := b.Sync(spec)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if res.Removed != 1 {
		t.Errorf("expected Removed=1, got %+v", res)
	}
	foundDisable := false
	for _, u := range fake.disabled {
		if u == "agentic-os-bravo.timer" {
			foundDisable = true
		}
	}
	if !foundDisable {
		t.Errorf("expected disable for bravo, got %v", fake.disabled)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "agentic-os-bravo.timer")); !os.IsNotExist(err) {
		t.Errorf("bravo.timer still on disk: %v", err)
	}
}

func TestSystemdSync_tickUsesOnUnitActiveSec(t *testing.T) {
	b, _, _ := newSystemdBackend(t)
	spec := Spec{
		Tick: TickJob{
			AosBinaryPath: "/usr/local/bin/aos",
			LogPath:       "/tmp/tick.log",
			Interval:      time.Hour,
		},
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(b.dir, "agentic-os-tick.timer"))
	if err != nil {
		t.Fatalf("read tick.timer: %v", err)
	}
	if !strings.Contains(string(data), "OnUnitActiveSec=3600s") {
		t.Errorf("missing OnUnitActiveSec=3600s in:\n%s", data)
	}
	if !strings.Contains(string(data), "OnBootSec=10s") {
		t.Errorf("missing OnBootSec in:\n%s", data)
	}
	svc, err := os.ReadFile(filepath.Join(b.dir, "agentic-os-tick.service"))
	if err != nil {
		t.Fatalf("read tick.service: %v", err)
	}
	if !strings.Contains(string(svc), "append:/tmp/tick.log") {
		t.Errorf("tick service should append to log:\n%s", svc)
	}
}

func TestSystemdState_managedDriftEmpty(t *testing.T) {
	b, _, aosHome := newSystemdBackend(t)
	spec := Spec{
		Agents: []AgentJob{{
			AgentID: "planner", ScriptPath: filepath.Join(aosHome, "planner.sh"),
			Schedule: schedspec.ScheduleSpec{Kind: "daily", Days: []schedspec.Weekday{schedspec.Mon}, Hour: 9, Minute: 0},
		}},
	}
	st, _ := b.State(Spec{})
	if st != StateEmpty {
		t.Errorf("empty state = %s", st)
	}
	st, _ = b.State(spec)
	if st != StateDrift {
		t.Errorf("drift state = %s", st)
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	st, _ = b.State(spec)
	if st != StateManaged {
		t.Errorf("managed state = %s", st)
	}
}

func TestSystemdRemove_clearsNamespace(t *testing.T) {
	b, fake, aosHome := newSystemdBackend(t)
	spec := Spec{
		Agents: []AgentJob{{
			AgentID: "x", ScriptPath: filepath.Join(aosHome, "x.sh"),
			Schedule: schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 0},
		}},
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := b.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	entries, _ := os.ReadDir(b.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), SystemdUnitPrefix) {
			t.Errorf("unit left after remove: %s", e.Name())
		}
	}
	if len(fake.disabled) == 0 {
		t.Errorf("expected disable calls during Remove")
	}
}

func TestSystemdOnCalendar_mappings(t *testing.T) {
	cases := []struct {
		name string
		spec schedspec.ScheduleSpec
		want string
	}{
		{"every hour at :05", schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 5}, "*-*-* *:05:00"},
		{"every 4h at :05", schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 4, Minute: 5}, "*-*-* 00/4:05:00"},
		{"daily Mon Tue at 09:00", schedspec.ScheduleSpec{Kind: "daily", Days: []schedspec.Weekday{schedspec.Mon, schedspec.Tue}, Hour: 9, Minute: 0}, "Mon,Tue *-*-* 09:00:00"},
		{"daily Sun at 23:59", schedspec.ScheduleSpec{Kind: "daily", Days: []schedspec.Weekday{schedspec.Sun}, Hour: 23, Minute: 59}, "Sun *-*-* 23:59:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := systemdOnCalendar(tc.spec)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSystemdParseUnit_roundtrip(t *testing.T) {
	u := systemdUnit{
		sections: []systemdSection{
			{name: "Unit", values: []systemdKV{{"Description", "test"}}},
			{name: "Service", values: []systemdKV{
				{"Type", "oneshot"},
				{"ExecStart", "/bin/true"},
			}},
		},
	}
	data := marshalUnit(u)
	got := parseUnit(data)
	if len(got.sections) != 2 {
		t.Fatalf("sections = %d", len(got.sections))
	}
	if got.sections[1].values[1].value != "/bin/true" {
		t.Errorf("ExecStart roundtrip failed: %+v", got)
	}
}
