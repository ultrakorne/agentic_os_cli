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
