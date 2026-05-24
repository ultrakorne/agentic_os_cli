package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// DefaultRunsHardCap is the maximum number of distinct runs kept under
// <aos_home>/runs/.
const DefaultRunsHardCap = 2000

// DefaultTickInterval is how often the periodic tick fires by default.
// The platform-native backends (launchd / systemd-user) make up missed wakes
// natively, so a 1h cadence is fine — far less than the cron-era 10m.
const DefaultTickInterval = "1h"

type Config struct {
	AosHome string `toml:"aos_home"`
	// RunsHardCap is the on-disk runs cap. Zero or negative values fall back
	// to DefaultRunsHardCap when consumed via EffectiveRunsHardCap.
	RunsHardCap int `toml:"runs_hard_cap"`
	// TickInterval controls how often the periodic tick fires. Format is a
	// Go duration string ("30m", "1h", "6h"). Consume via EffectiveTickInterval.
	TickInterval string `toml:"tick_interval"`
}

// EffectiveRunsHardCap returns the configured cap or DefaultRunsHardCap when
// unset.
func (c *Config) EffectiveRunsHardCap() int {
	if c == nil || c.RunsHardCap <= 0 {
		return DefaultRunsHardCap
	}
	return c.RunsHardCap
}

// EffectiveTickInterval parses the configured TickInterval. On parse failure
// the function falls back to DefaultTickInterval and returns a non-nil error
// describing what was wrong so the caller can log it.
func (c *Config) EffectiveTickInterval() (time.Duration, error) {
	raw := DefaultTickInterval
	if c != nil && c.TickInterval != "" {
		raw = c.TickInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < time.Minute {
		def, _ := time.ParseDuration(DefaultTickInterval)
		if err == nil {
			err = fmt.Errorf("must be at least 1 minute")
		}
		return def, fmt.Errorf("invalid tick_interval %q: %w", raw, err)
	}
	return d, nil
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
	// DisallowUnknownFields is NOT set — fields removed from the schema
	// (`catchup_enabled`) are silently dropped so an upgrade doesn't error.
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
