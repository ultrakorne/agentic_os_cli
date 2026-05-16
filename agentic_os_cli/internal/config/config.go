package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// DefaultRunsHardCap is the maximum number of distinct runs (each run pairs a
// .json metadata file with up to one .out output file) kept under
// <aos_home>/runs/. `aos refresh` deletes the oldest pairs once the count
// exceeds this cap.
const DefaultRunsHardCap = 2000

type Config struct {
	AosHome string `toml:"aos_home"`
	// RunsHardCap is the on-disk runs cap. Zero or negative values fall back
	// to DefaultRunsHardCap when consumed via EffectiveRunsHardCap.
	RunsHardCap int `toml:"runs_hard_cap"`
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
