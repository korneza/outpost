// Package config loads and validates the Outpost YAML configuration.
//
// Invariant (spec §10): there is no global retry setting. Retries are a
// per-tool opt-in and default to off; nothing in this package may add a
// top-level retry knob.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen    string                  `yaml:"listen"`
	StateDB   string                  `yaml:"state_db"`
	Upstreams []Upstream              `yaml:"upstreams"`
	Tools     map[string]ToolOverride `yaml:"tools"`
}

type Upstream struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type ToolOverride struct {
	Retry *RetryPolicy `yaml:"retry"`
	Block bool         `yaml:"block"`
}

type RetryPolicy struct {
	MaxAttempts      int `yaml:"max_attempts"`
	InitialBackoffMS int `yaml:"initial_backoff_ms"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.StateDB == "" {
		cfg.StateDB = "outpost.db"
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Listen == "" {
		return fmt.Errorf("listen address is required")
	}
	if len(c.Upstreams) == 0 {
		return fmt.Errorf("at least one upstream is required")
	}
	for i, u := range c.Upstreams {
		if u.Name == "" || u.URL == "" {
			return fmt.Errorf("upstream %d: name and url are required", i)
		}
	}
	return nil
}
