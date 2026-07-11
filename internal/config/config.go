package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	DBPath        string `toml:"db_path"`
	OTLPPort      int    `toml:"otlp_port"`
	WithLogs      bool   `toml:"with_logs"`
	RetentionDays int    `toml:"retention_days"`

	// UnknownKeys lists config file keys that did not match any field,
	// so callers can warn about typos. Not part of the file format.
	UnknownKeys []string `toml:"-"`
}

func defaults() *Config {
	return &Config{
		OTLPPort: 4318,
		WithLogs: false,
	}
}

// Load reads the config at path, tolerating a missing file (the default
// path may simply not exist yet).
func Load(path string) (*Config, error) {
	return load(path, false)
}

// LoadExplicit reads a config path the user named explicitly; a missing
// file is an error rather than silently falling back to defaults.
func LoadExplicit(path string) (*Config, error) {
	return load(path, true)
}

func load(path string, mustExist bool) (*Config, error) {
	cfg := defaults()
	if path == "" {
		return cfg, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if mustExist {
			return nil, fmt.Errorf("config file %s does not exist", path)
		}
		return cfg, nil
	}
	md, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, err
	}
	for _, key := range md.Undecoded() {
		cfg.UnknownKeys = append(cfg.UnknownKeys, key.String())
	}
	return cfg, nil
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".hedge", "config.toml")
}

func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".hedge", "hedge.db")
}
