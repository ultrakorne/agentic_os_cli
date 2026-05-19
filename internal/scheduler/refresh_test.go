package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ultrakorne/aos_cli/internal/config"
)

func TestRefresh_uninitializedConfig(t *testing.T) {
	_, err := Refresh(RefreshDeps{Cfg: nil, Now: time.Now()})
	if err == nil {
		t.Fatal("Refresh(nil cfg) returned nil error, want failure")
	}
}

func TestRefresh_missingAosHome(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{AosHome: filepath.Join(tmp, "nope")}
	_, err := Refresh(RefreshDeps{Cfg: cfg, Now: time.Now()})
	if err == nil {
		t.Fatal("Refresh with non-existent aos_home returned nil error, want failure")
	}
}

// TestRefresh_emptyHomeRunsWithoutError exercises the happy-but-empty path:
// a valid aos_home with no agents and no wrapper. The probes for wrapper /
// python3 / cron daemon will reflect the host, but the algorithm itself must
// complete and report zero agents.
func TestRefresh_emptyHomeRunsWithoutError(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{AosHome: tmp}
	out, err := Refresh(RefreshDeps{Cfg: cfg, Now: time.Now()})
	if err != nil {
		t.Fatalf("Refresh on empty home: %v", err)
	}
	if out.Agents != 0 {
		t.Errorf("Agents = %d, want 0", out.Agents)
	}
	if out.Scheduled != 0 {
		t.Errorf("Scheduled = %d, want 0", out.Scheduled)
	}
	// Wrapper is missing in a bare tempdir.
	if out.Wrapper != HealthMissing {
		t.Errorf("Wrapper = %q, want %q", out.Wrapper, HealthMissing)
	}
}
