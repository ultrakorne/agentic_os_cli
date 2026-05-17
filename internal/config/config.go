package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// DefaultRunsHardCap is the maximum number of distinct runs (each run pairs a
// .json metadata file with up to one .out output file) kept under
// <aos_home>/runs/. `aos refresh` deletes the oldest pairs once the count
// exceeds this cap.
const DefaultRunsHardCap = 2000

// DefaultTickInterval is how often the cron `__tick__` entry fires by
// default. Format is a Go duration string accepted by time.ParseDuration.
const DefaultTickInterval = "10m"

type Config struct {
	AosHome string `toml:"aos_home"`
	// RunsHardCap is the on-disk runs cap. Zero or negative values fall back
	// to DefaultRunsHardCap when consumed via EffectiveRunsHardCap.
	RunsHardCap int `toml:"runs_hard_cap"`
	// CatchupEnabled controls whether `aos tick` auto-fires a catch-up run
	// for agents whose latest run is status="missed". Pointer so absence in
	// the TOML (the common case) is distinguishable from an explicit
	// `catchup_enabled = false`; consume via EffectiveCatchupEnabled.
	CatchupEnabled *bool `toml:"catchup_enabled,omitempty"`
	// TickInterval controls how often the managed cron `__tick__` entry
	// fires. Format is a Go duration string ("10m", "30m", "1h", "6h").
	// Accepts whole minutes in [1, 59] (compiled to `*/N * * * *`) or
	// whole hours in [1, 23] (compiled to `0 */H * * *`); anything else is
	// rejected and the default is used. Consume via EffectiveTickCronExpr.
	TickInterval string `toml:"tick_interval"`
}

// EffectiveRunsHardCap returns the configured cap or DefaultRunsHardCap when
// unset. Centralizing the fallback keeps the TOML free of an explicit default
// while letting users override it.
func (c *Config) EffectiveRunsHardCap() int {
	if c == nil || c.RunsHardCap <= 0 {
		return DefaultRunsHardCap
	}
	return c.RunsHardCap
}

// EffectiveCatchupEnabled returns the configured value or true when unset.
// Centralizing the fallback keeps the TOML free of an explicit default while
// letting users opt out with `catchup_enabled = false`.
func (c *Config) EffectiveCatchupEnabled() bool {
	if c == nil || c.CatchupEnabled == nil {
		return true
	}
	return *c.CatchupEnabled
}

// EffectiveTickCronExpr parses the configured TickInterval and returns the
// equivalent crontab(5) schedule. The returned expression is always usable:
// on parse failure the function falls back to DefaultTickInterval and returns
// a non-nil error describing what was wrong so the caller can log it.
//
// Accepted durations are whole minutes in [1, 59] → `*/N * * * *`, or whole
// hours in [1, 23] → `0 */H * * *`. Sub-minute precision, non-divisible
// hour intervals, and intervals ≥ 24h are rejected — system cron's `*/N`
// step syntax can't express them cleanly.
func (c *Config) EffectiveTickCronExpr() (string, error) {
	raw := DefaultTickInterval
	if c != nil && c.TickInterval != "" {
		raw = c.TickInterval
	}
	expr, err := parseTickInterval(raw)
	if err != nil {
		// Defaults are validated by tests, so this second parse never fails.
		def, _ := parseTickInterval(DefaultTickInterval)
		return def, fmt.Errorf("invalid tick_interval %q: %w", raw, err)
	}
	return expr, nil
}

func parseTickInterval(s string) (string, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return "", err
	}
	if d < time.Minute {
		return "", fmt.Errorf("must be at least 1 minute")
	}
	if d%time.Minute != 0 {
		return "", fmt.Errorf("must be a whole number of minutes")
	}
	minutes := int(d / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("*/%d * * * *", minutes), nil
	}
	if minutes%60 != 0 {
		return "", fmt.Errorf("intervals over 59m must be a whole number of hours")
	}
	hours := minutes / 60
	if hours > 23 {
		return "", fmt.Errorf("must be at most 23h")
	}
	return fmt.Sprintf("0 */%d * * *", hours), nil
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "aos"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load returns the on-disk config. If the file does not exist, returns
// (nil, nil) so callers can distinguish "absent" from "broken".
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &c, nil
}

func Save(c *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, "config.toml")
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// Remove deletes the config file, and the ~/.config/aos directory if it is
// empty afterwards. Returns (configRemoved, dirRemoved, error). Missing
// config file is not an error — configRemoved=false in that case.
func Remove() (bool, bool, error) {
	p, err := Path()
	if err != nil {
		return false, false, err
	}
	configRemoved := false
	if err := os.Remove(p); err == nil {
		configRemoved = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, false, err
	}
	dirRemoved := false
	dir, err := Dir()
	if err != nil {
		return configRemoved, false, err
	}
	if err := os.Remove(dir); err == nil {
		dirRemoved = true
	} else if !errors.Is(err, os.ErrNotExist) {
		// non-empty dir → leave it; not an error
	}
	return configRemoved, dirRemoved, nil
}
