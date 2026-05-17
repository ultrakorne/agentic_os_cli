package main

import (
	"testing"

	"github.com/ultrakorne/aos_cli/internal/config"
)

func TestMergeInitConfig_freshWritesDefaults(t *testing.T) {
	// Fresh install (no existing config): init should materialize every
	// tunable with its default so the user can see what's available in the
	// TOML without reading docs.
	got := mergeInitConfig(nil, "/tmp/aos-home")

	if got.AosHome != "/tmp/aos-home" {
		t.Errorf("AosHome = %q, want /tmp/aos-home", got.AosHome)
	}
	if got.RunsHardCap != config.DefaultRunsHardCap {
		t.Errorf("RunsHardCap = %d, want %d", got.RunsHardCap, config.DefaultRunsHardCap)
	}
	if got.CatchupEnabled == nil || !*got.CatchupEnabled {
		t.Errorf("CatchupEnabled = %v, want pointer to true", got.CatchupEnabled)
	}
}

func TestMergeInitConfig_preservesUserSetValues(t *testing.T) {
	// Re-init must not silently wipe user choices. A user who disabled
	// catch-up or tightened the runs cap should find their values intact
	// after re-running `aos init <path>`.
	fls := false
	existing := &config.Config{
		AosHome:        "/old/home",
		RunsHardCap:    500,
		CatchupEnabled: &fls,
	}
	got := mergeInitConfig(existing, "/new/home")

	if got.AosHome != "/new/home" {
		t.Errorf("AosHome = %q, want /new/home", got.AosHome)
	}
	if got.RunsHardCap != 500 {
		t.Errorf("RunsHardCap = %d, want 500 (preserved)", got.RunsHardCap)
	}
	if got.CatchupEnabled == nil || *got.CatchupEnabled {
		t.Errorf("CatchupEnabled = %v, want pointer to false (preserved)", got.CatchupEnabled)
	}
}

func TestMergeInitConfig_normalizesInvalidRunsHardCap(t *testing.T) {
	// Treat negative caps the same way EffectiveRunsHardCap does: replace
	// with the default. Otherwise init would persist a value that the rest
	// of the codebase already considers invalid.
	existing := &config.Config{RunsHardCap: -5}
	got := mergeInitConfig(existing, "/tmp/x")
	if got.RunsHardCap != config.DefaultRunsHardCap {
		t.Errorf("RunsHardCap = %d, want default %d", got.RunsHardCap, config.DefaultRunsHardCap)
	}
}

func TestMergeInitConfig_doesNotMutateExisting(t *testing.T) {
	// The function returns a fresh struct so the in-memory `existing` (read
	// for relocation logic upstream) is left untouched.
	existing := &config.Config{AosHome: "/old"}
	got := mergeInitConfig(existing, "/new")
	if existing.AosHome != "/old" {
		t.Errorf("existing.AosHome mutated to %q", existing.AosHome)
	}
	if got == existing {
		t.Errorf("mergeInitConfig returned the same pointer; should be a fresh struct")
	}
}
