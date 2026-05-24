package main

import (
	"testing"

	"github.com/ultrakorne/aos_cli/internal/config"
)

func TestMergeInitConfig_freshWritesDefaults(t *testing.T) {
	// Fresh install (no existing config): init materializes every tunable
	// with its default so config.toml is self-documenting.
	got := mergeInitConfig(nil, "/tmp/aos-home")

	if got.AosHome != "/tmp/aos-home" {
		t.Errorf("AosHome = %q, want /tmp/aos-home", got.AosHome)
	}
	if got.RunsHardCap != config.DefaultRunsHardCap {
		t.Errorf("RunsHardCap = %d, want %d", got.RunsHardCap, config.DefaultRunsHardCap)
	}
	if got.TickInterval != config.DefaultTickInterval {
		t.Errorf("TickInterval = %q, want %q", got.TickInterval, config.DefaultTickInterval)
	}
}

func TestMergeInitConfig_preservesUserSetValues(t *testing.T) {
	// Re-init must not silently wipe user choices. A user who tightened the
	// runs cap or sped up the tick should find their values intact.
	existing := &config.Config{
		AosHome:      "/old/home",
		RunsHardCap:  500,
		TickInterval: "5m",
	}
	got := mergeInitConfig(existing, "/new/home")

	if got.AosHome != "/new/home" {
		t.Errorf("AosHome = %q, want /new/home", got.AosHome)
	}
	if got.RunsHardCap != 500 {
		t.Errorf("RunsHardCap = %d, want 500 (preserved)", got.RunsHardCap)
	}
	if got.TickInterval != "5m" {
		t.Errorf("TickInterval = %q, want \"5m\" (preserved)", got.TickInterval)
	}
}

func TestMergeInitConfig_normalizesInvalidRunsHardCap(t *testing.T) {
	existing := &config.Config{RunsHardCap: -5}
	got := mergeInitConfig(existing, "/tmp/x")
	if got.RunsHardCap != config.DefaultRunsHardCap {
		t.Errorf("RunsHardCap = %d, want default %d", got.RunsHardCap, config.DefaultRunsHardCap)
	}
}

func TestMergeInitConfig_preservesInvalidTickIntervalForLaterWarning(t *testing.T) {
	// Init doesn't second-guess the user's TickInterval: a malformed value is
	// preserved so the next `aos refresh` logs the concrete error and falls
	// back to the default.
	existing := &config.Config{TickInterval: "90m"}
	got := mergeInitConfig(existing, "/tmp/x")
	if got.TickInterval != "90m" {
		t.Errorf("TickInterval = %q, want \"90m\" preserved", got.TickInterval)
	}
}

func TestMergeInitConfig_doesNotMutateExisting(t *testing.T) {
	existing := &config.Config{AosHome: "/old"}
	got := mergeInitConfig(existing, "/new")
	if existing.AosHome != "/old" {
		t.Errorf("existing.AosHome mutated to %q", existing.AosHome)
	}
	if got == existing {
		t.Errorf("mergeInitConfig returned the same pointer; should be a fresh struct")
	}
}
