package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	DBPath   string `toml:"db_path"`
	OTLPPort int    `toml:"otlp_port"`
	WithLogs bool   `toml:"with_logs"`
}

func defaults() *Config {
	return &Config{
		OTLPPort: 4318,
		WithLogs: false,
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	if path == "" {
		return cfg, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
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
