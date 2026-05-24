package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestLoadIgnoresRemovedCatchupField(t *testing.T) {
	// Upgrade path: an old config.toml with `catchup_enabled = false` must
	// still parse cleanly under the new schema. go-toml ignores unknown
	// fields by default; this test pins that behavior so the dead field name
	// never sneaks back in as a parse error.
	withFakeHome(t)
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "aos_home = \"/tmp/x\"\ncatchup_enabled = false\nruns_hard_cap = 100\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.AosHome != "/tmp/x" || got.RunsHardCap != 100 {
		t.Errorf("Load = %+v, want clean parse with legacy field ignored", got)
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

func TestEffectiveTickIntervalValidInputs(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
		want time.Duration
	}{
		{"nil config falls back to default", nil, time.Hour},
		{"empty string falls back to default", &Config{}, time.Hour},
		{"5 minutes", &Config{TickInterval: "5m"}, 5 * time.Minute},
		{"1 hour explicit", &Config{TickInterval: "1h"}, time.Hour},
		{"6 hours", &Config{TickInterval: "6h"}, 6 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.cfg.EffectiveTickInterval()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("EffectiveTickInterval = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEffectiveTickIntervalRejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"garbage", "not-a-duration"},
		{"sub-minute", "30s"},
		{"zero", "0m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&Config{TickInterval: tc.raw}).EffectiveTickInterval()
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.raw)
			}
			if got != time.Hour {
				t.Errorf("fallback = %v, want default 1h", got)
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
