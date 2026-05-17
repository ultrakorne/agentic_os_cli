package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withFakeHome redirects $HOME to a fresh temp dir for the duration of the
// test so config writes never touch the real user's filesystem.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestDirAndPath(t *testing.T) {
	home := withFakeHome(t)
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	wantDir := filepath.Join(home, ".config", "aos")
	if dir != wantDir {
		t.Errorf("Dir = %q, want %q", dir, wantDir)
	}
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	wantPath := filepath.Join(wantDir, "config.toml")
	if p != wantPath {
		t.Errorf("Path = %q, want %q", p, wantPath)
	}
}

func TestLoadMissingReturnsNilNil(t *testing.T) {
	withFakeHome(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c != nil {
		t.Errorf("Load on missing file = %+v, want nil", c)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	home := withFakeHome(t)
	want := &Config{AosHome: filepath.Join(home, "aos-data")}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.AosHome != want.AosHome {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestLoadInvalidTOMLErrors(t *testing.T) {
	withFakeHome(t)
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("not [ valid toml ="), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Error("Load on invalid TOML returned nil error")
	}
}

func TestRemoveExistingClearsConfigAndDir(t *testing.T) {
	withFakeHome(t)
	if err := Save(&Config{AosHome: "/tmp/x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfgRemoved, dirRemoved, err := Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !cfgRemoved {
		t.Error("configRemoved = false, want true")
	}
	if !dirRemoved {
		t.Error("dirRemoved = false, want true")
	}
	dir, _ := Dir()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected dir gone, got stat err = %v", err)
	}
}

func TestRemoveMissingIsNoop(t *testing.T) {
	withFakeHome(t)
	cfgRemoved, dirRemoved, err := Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if cfgRemoved || dirRemoved {
		t.Errorf("Remove on missing returned (%v, %v), want (false, false)", cfgRemoved, dirRemoved)
	}
}

func TestEffectiveRunsHardCapDefaultsWhenUnset(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
		want int
	}{
		{"nil config", nil, DefaultRunsHardCap},
		{"zero value", &Config{}, DefaultRunsHardCap},
		{"negative value", &Config{RunsHardCap: -5}, DefaultRunsHardCap},
		{"explicit override", &Config{RunsHardCap: 250}, 250},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EffectiveRunsHardCap(); got != tc.want {
				t.Errorf("EffectiveRunsHardCap = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEffectiveTickCronExprValidInputs(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"nil config falls back to default", nil, "*/10 * * * *"},
		{"empty string falls back to default", &Config{}, "*/10 * * * *"},
		{"minute interval", &Config{TickInterval: "5m"}, "*/5 * * * *"},
		{"lower minute bound", &Config{TickInterval: "1m"}, "*/1 * * * *"},
		{"upper minute bound", &Config{TickInterval: "59m"}, "*/59 * * * *"},
		{"60m promotes to hour", &Config{TickInterval: "60m"}, "0 */1 * * *"},
		{"1h", &Config{TickInterval: "1h"}, "0 */1 * * *"},
		{"2h", &Config{TickInterval: "2h"}, "0 */2 * * *"},
		{"6h", &Config{TickInterval: "6h"}, "0 */6 * * *"},
		{"23h upper bound", &Config{TickInterval: "23h"}, "0 */23 * * *"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.cfg.EffectiveTickCronExpr()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("EffectiveTickCronExpr = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEffectiveTickCronExprRejectsBadValues(t *testing.T) {
	// Bad inputs must return both an error AND the default cron expression
	// so refresh/tick can log the warning and still install a working cron
	// block.
	cases := []struct {
		name string
		raw  string
	}{
		{"garbage", "not-a-duration"},
		{"sub-minute", "30s"},
		{"zero", "0m"},
		{"fractional minutes", "1.5m"},
		{"90m not whole hours", "90m"},
		{"24h too large", "24h"},
		{"100h too large", "100h"},
		{"negative", "-5m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&Config{TickInterval: tc.raw}).EffectiveTickCronExpr()
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.raw)
			}
			if got != "*/10 * * * *" {
				t.Errorf("fallback expr = %q, want default %q", got, "*/10 * * * *")
			}
		})
	}
}

func TestSaveLoadPreservesTickInterval(t *testing.T) {
	withFakeHome(t)
	want := &Config{AosHome: "/tmp/x", TickInterval: "15m"}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.TickInterval != "15m" {
		t.Errorf("Load = %+v, want TickInterval=\"15m\"", got)
	}
}

func TestEffectiveCatchupEnabledDefaultsToTrue(t *testing.T) {
	tru, fls := true, false
	cases := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"nil config", nil, true},
		{"zero value", &Config{}, true},
		{"explicit true", &Config{CatchupEnabled: &tru}, true},
		{"explicit false", &Config{CatchupEnabled: &fls}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EffectiveCatchupEnabled(); got != tc.want {
				t.Errorf("EffectiveCatchupEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSaveLoadPreservesCatchupEnabledFalse(t *testing.T) {
	withFakeHome(t)
	fls := false
	want := &Config{AosHome: "/tmp/x", CatchupEnabled: &fls}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.CatchupEnabled == nil || *got.CatchupEnabled != false {
		t.Errorf("Load = %+v, want CatchupEnabled=false", got)
	}
}

func TestSaveLoadPreservesRunsHardCap(t *testing.T) {
	withFakeHome(t)
	want := &Config{AosHome: "/tmp/x", RunsHardCap: 500}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.RunsHardCap != 500 {
		t.Errorf("Load = %+v, want RunsHardCap=500", got)
	}
}

func TestRemoveLeavesNonEmptyDir(t *testing.T) {
	withFakeHome(t)
	if err := Save(&Config{AosHome: "/tmp/x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dir, _ := Dir()
	sibling := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(sibling, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}
	cfgRemoved, dirRemoved, err := Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !cfgRemoved {
		t.Error("configRemoved = false, want true")
	}
	if dirRemoved {
		t.Error("dirRemoved = true, want false (sibling should keep dir)")
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("sibling vanished: %v", err)
	}
}
