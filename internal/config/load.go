package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads a YAML configuration file from path and returns a fully populated
// Config. Fields absent from the file are filled from DefaultConfig.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	applyDefaults(cfg)
	return cfg, nil
}

// applyDefaults fills any zero-value scalar fields with the DefaultConfig
// values so partial YAML files get sensible fallbacks.
func applyDefaults(cfg *Config) {
	d := DefaultConfig()

	if cfg.Mode == "" {
		cfg.Mode = d.Mode
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = d.Server.Port
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = d.Server.Host
	}
	if cfg.Server.CtxSize == 0 {
		cfg.Server.CtxSize = d.Server.CtxSize
	}
	if cfg.Server.Threads == 0 {
		cfg.Server.Threads = d.Server.Threads
	}
	if cfg.Server.Temp == 0 {
		cfg.Server.Temp = d.Server.Temp
	}
	if cfg.Server.MaxTokens == 0 {
		cfg.Server.MaxTokens = d.Server.MaxTokens
	}
	if cfg.Metrics.DBPath == "" {
		cfg.Metrics.DBPath = d.Metrics.DBPath
	}
	if cfg.Metrics.PrometheusPort == 0 {
		cfg.Metrics.PrometheusPort = d.Metrics.PrometheusPort
	}
	if cfg.OTel.ServiceName == "" {
		cfg.OTel.ServiceName = d.OTel.ServiceName
	}
	if cfg.Business.CostPerToken == 0 {
		cfg.Business.CostPerToken = d.Business.CostPerToken
	}
	if cfg.Business.CurrencySymbol == "" {
		cfg.Business.CurrencySymbol = d.Business.CurrencySymbol
	}
}
