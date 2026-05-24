//go:build darwin

package backend

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ultrakorne/aos_cli/internal/scheduler/schedspec"
)

type fakeLaunchdLoader struct {
	bootstrapped []string
	booted       []string
	loaded       map[string]bool
	probeErr     error
}

func newFakeLaunchd() *fakeLaunchdLoader { return &fakeLaunchdLoader{loaded: map[string]bool{}} }

func (f *fakeLaunchdLoader) Bootstrap(path string) error {
	f.bootstrapped = append(f.bootstrapped, path)
	name := strings.TrimSuffix(filepath.Base(path), ".plist")
	f.loaded[name] = true
	return nil
}
func (f *fakeLaunchdLoader) Bootout(label string) error {
	f.booted = append(f.booted, label)
	delete(f.loaded, label)
	return nil
}
func (f *fakeLaunchdLoader) IsLoaded(label string) (bool, error) {
	return f.loaded[label], nil
}
func (f *fakeLaunchdLoader) Probe() error { return f.probeErr }

func newLaunchdBackend(t *testing.T) (*LaunchdBackend, *fakeLaunchdLoader, string) {
	t.Helper()
	tmp := t.TempDir()
	aosHome := filepath.Join(tmp, "home")
	if err := os.MkdirAll(aosHome, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir := filepath.Join(tmp, "LaunchAgents")
	fake := newFakeLaunchd()
	b := NewLaunchd(aosHome).WithDir(dir).WithLoader(fake)
	return b, fake, aosHome
}

// TestLaunchdState_driftWhenBootedOutExternally covers the silent-dead case:
// the plist file is on disk and matches the spec, but launchctl has been told
// to bootout the unit (manual debugging, partial uninstall, etc.). State must
// surface this as drift; Sync must heal it.
func TestLaunchdState_driftWhenBootedOutExternally(t *testing.T) {
	b, fake, aosHome := newLaunchdBackend(t)
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
	// Simulate external bootout — file stays, launchd loses the unit.
	delete(fake.loaded, "com.agenticos.planner")

	st, err := b.State(spec)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st != StateDrift {
		t.Errorf("State = %s, want drift (file matches but unit not loaded)", st)
	}

	// Sync should re-bootstrap without rewriting the file.
	before := len(fake.bootstrapped)
	res, err := b.Sync(spec)
	if err != nil {
		t.Fatalf("Sync recovery: %v", err)
	}
	if len(fake.bootstrapped) != before+1 {
		t.Errorf("expected 1 additional Bootstrap call, got %d", len(fake.bootstrapped)-before)
	}
	if res.Wrote != 1 {
		t.Errorf("res.Wrote = %d, want 1", res.Wrote)
	}
	if !fake.loaded["com.agenticos.planner"] {
		t.Errorf("unit should be loaded after recovery Sync")
	}
}

func TestLaunchdSync_writesAndBootstrapsAgent(t *testing.T) {
	b, fake, aosHome := newLaunchdBackend(t)
	spec := Spec{
		Agents: []AgentJob{
			{
				AgentID:    "planner",
				ScriptPath: filepath.Join(aosHome, "agents", "planner.sh"),
				Schedule:   schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 5},
			},
		},
	}
	res, err := b.Sync(spec)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Wrote != 1 || res.Unchanged != 0 || res.Removed != 0 || len(res.Failed) != 0 {
		t.Errorf("Sync result = %+v", res)
	}
	if len(fake.bootstrapped) != 1 || !strings.Contains(fake.bootstrapped[0], "com.agenticos.planner.plist") {
		t.Errorf("bootstrap calls = %v", fake.bootstrapped)
	}
	plistPath := b.PlistPath(LaunchdAgentLabel("planner"))
	if _, err := os.Stat(plistPath); err != nil {
		t.Errorf("plist not written: %v", err)
	}
}

func TestLaunchdSync_unchangedWhenIdentical(t *testing.T) {
	b, _, aosHome := newLaunchdBackend(t)
	spec := Spec{
		Agents: []AgentJob{{
			AgentID: "planner", ScriptPath: filepath.Join(aosHome, "agents", "planner.sh"),
			Schedule: schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 5},
		}},
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	res, err := b.Sync(spec)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if res.Wrote != 0 || res.Unchanged != 1 {
		t.Errorf("expected unchanged on rerun, got %+v", res)
	}
}

func TestLaunchdSync_removesOrphans(t *testing.T) {
	b, fake, aosHome := newLaunchdBackend(t)
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
		t.Errorf("expected removed=1, got %+v", res)
	}
	if _, err := os.Stat(b.PlistPath(LaunchdAgentLabel("bravo"))); !os.IsNotExist(err) {
		t.Errorf("bravo plist not removed: %v", err)
	}
	found := false
	for _, l := range fake.booted {
		if l == LaunchdAgentLabel("bravo") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bootout for bravo, got booted=%v", fake.booted)
	}
}

func TestLaunchdSync_includesTickJob(t *testing.T) {
	b, _, _ := newLaunchdBackend(t)
	spec := Spec{
		Tick: TickJob{
			AosBinaryPath: "/usr/local/bin/aos",
			LogPath:       "/tmp/tick.log",
			Interval:      time.Hour,
		},
	}
	res, err := b.Sync(spec)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Wrote != 1 {
		t.Errorf("expected wrote=1 for tick, got %+v", res)
	}
	tickPath := b.PlistPath(LaunchdTickLabel)
	data, err := os.ReadFile(tickPath)
	if err != nil {
		t.Fatalf("read tick plist: %v", err)
	}
	if !strings.Contains(string(data), "<key>StartInterval</key>") {
		t.Errorf("tick plist missing StartInterval: %s", data)
	}
	if !strings.Contains(string(data), "<integer>3600</integer>") {
		t.Errorf("tick plist interval = 3600s expected: %s", data)
	}
}

func TestLaunchdState_managedAndDriftAndEmpty(t *testing.T) {
	b, _, aosHome := newLaunchdBackend(t)
	spec := Spec{
		Agents: []AgentJob{{
			AgentID: "planner", ScriptPath: filepath.Join(aosHome, "planner.sh"),
			Schedule: schedspec.ScheduleSpec{Kind: "daily", Days: []schedspec.Weekday{schedspec.Mon}, Hour: 9, Minute: 0},
		}},
	}
	st, err := b.State(Spec{})
	if err != nil {
		t.Fatalf("State empty: %v", err)
	}
	if st != StateEmpty {
		t.Errorf("State empty = %s, want empty", st)
	}
	st, err = b.State(spec)
	if err != nil {
		t.Fatalf("State drift: %v", err)
	}
	if st != StateDrift {
		t.Errorf("State drift = %s, want drift", st)
	}
	if _, err := b.Sync(spec); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	st, err = b.State(spec)
	if err != nil {
		t.Fatalf("State managed: %v", err)
	}
	if st != StateManaged {
		t.Errorf("State managed = %s, want managed", st)
	}
}

func TestLaunchdRemove_clearsNamespace(t *testing.T) {
	b, fake, aosHome := newLaunchdBackend(t)
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
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".plist") {
			count++
		}
	}
	if count != 0 {
		t.Errorf("plists left after remove: %d", count)
	}
	if len(fake.booted) == 0 {
		t.Errorf("expected bootout calls during Remove")
	}
}

func TestLaunchdCalendarEntries_hourlyEvery4(t *testing.T) {
	spec := schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 4, Minute: 5}
	entries, err := launchdCalendarEntries(spec)
	if err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("expected 6 entries for every-4h, got %d", len(entries))
	}
	for i, e := range entries {
		wantH := i * 4
		if e.Hour == nil || *e.Hour != wantH {
			t.Errorf("entry %d hour = %v, want %d", i, e.Hour, wantH)
		}
		if e.Minute == nil || *e.Minute != 5 {
			t.Errorf("entry %d minute = %v, want 5", i, e.Minute)
		}
	}
}

func TestLaunchdCalendarEntries_hourlyEvery1OmitsHour(t *testing.T) {
	spec := schedspec.ScheduleSpec{Kind: "hourly", EveryHours: 1, Minute: 5}
	entries, err := launchdCalendarEntries(spec)
	if err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("every-1h should produce 1 entry, got %d", len(entries))
	}
	if entries[0].Hour != nil {
		t.Errorf("Hour should be omitted, got %v", entries[0].Hour)
	}
}

func TestLaunchdCalendarEntries_dailyExpands(t *testing.T) {
	spec := schedspec.ScheduleSpec{Kind: "daily", Days: []schedspec.Weekday{schedspec.Mon, schedspec.Tue}, Hour: 9, Minute: 0}
	entries, err := launchdCalendarEntries(spec)
	if err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	got := []int{}
	for _, e := range entries {
		got = append(got, *e.Weekday)
	}
	sort.Ints(got)
	if got[0] != 1 || got[1] != 2 {
		t.Errorf("weekdays = %v, want [1 2]", got)
	}
}
