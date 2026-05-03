package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mwigge/llmctl/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"mode", cfg.Mode, "single"},
		{"port", cfg.Server.Port, 8765},
		{"ctx_size", cfg.Server.CtxSize, 32768},
		{"temp", cfg.Server.Temp, 0.7},
		{"cost_per_token", cfg.Business.CostPerToken, 0.0001},
		{"currency", cfg.Business.CurrencySymbol, "$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("DefaultConfig().%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLoad_FromYAML(t *testing.T) {
	t.Parallel()

	yaml := `
mode: hot-swap
server:
  port: 9000
  ctx_size: 8192
  temp: 0.5
business:
  cost_per_token: 0.0002
  currency_symbol: "€"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "llmctl.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"mode", cfg.Mode, "hot-swap"},
		{"port", cfg.Server.Port, 9000},
		{"ctx_size", cfg.Server.CtxSize, 8192},
		{"temp", cfg.Server.Temp, 0.5},
		{"cost_per_token", cfg.Business.CostPerToken, 0.0002},
		{"currency", cfg.Business.CurrencySymbol, "€"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("Load().%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLoad_DefaultsFillMissingFields(t *testing.T) {
	t.Parallel()

	// Only override mode; everything else should come from defaults.
	yaml := `mode: parallel`

	dir := t.TempDir()
	path := filepath.Join(dir, "llmctl.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 8765 {
		t.Errorf("server.port = %d, want 8765 (default)", cfg.Server.Port)
	}
	if cfg.Server.CtxSize != 32768 {
		t.Errorf("server.ctx_size = %d, want 32768 (default)", cfg.Server.CtxSize)
	}
	if cfg.Business.CurrencySymbol != "$" {
		t.Errorf("business.currency_symbol = %q, want \"$\" (default)", cfg.Business.CurrencySymbol)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := config.Load("/nonexistent/path/llmctl.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestSave_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "llmctl.yaml")

	original := config.DefaultConfig()
	original.Mode = "cold-swap"
	original.Server.Port = 7777
	original.Business.CurrencySymbol = "£"

	if err := config.Save(original, path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify 0o600 permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 600", perm)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() after Save() error = %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"mode", loaded.Mode, "cold-swap"},
		{"port", loaded.Server.Port, 7777},
		{"currency", loaded.Business.CurrencySymbol, "£"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("round-trip %s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestSave_CreatesParentDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "llmctl.yaml")

	if err := config.Save(config.DefaultConfig(), path); err != nil {
		t.Fatalf("Save() with deep path error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file not found: %v", err)
	}
}

func TestLoad_ApplyDefaults_AllZeroFields(t *testing.T) {
	t.Parallel()

	// Write a config that explicitly zeros out every field that has a default.
	// yaml.v3 ignores zero values in Marshal but we can write them manually.
	yaml := `
mode: ""
server:
  host: ""
  port: 0
  ctx_size: 0
  threads: 0
  temp: 0
  max_tokens: 0
metrics:
  db_path: ""
  prometheus_port: 0
otel:
  service_name: ""
business:
  cost_per_token: 0
  currency_symbol: ""
`
	dir := t.TempDir()
	path := filepath.Join(dir, "llmctl.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	d := config.DefaultConfig()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"mode", cfg.Mode, d.Mode},
		{"host", cfg.Server.Host, d.Server.Host},
		{"port", cfg.Server.Port, d.Server.Port},
		{"ctx_size", cfg.Server.CtxSize, d.Server.CtxSize},
		{"threads", cfg.Server.Threads, d.Server.Threads},
		{"temp", cfg.Server.Temp, d.Server.Temp},
		{"max_tokens", cfg.Server.MaxTokens, d.Server.MaxTokens},
		{"db_path", cfg.Metrics.DBPath, d.Metrics.DBPath},
		{"prometheus_port", cfg.Metrics.PrometheusPort, d.Metrics.PrometheusPort},
		{"otel_service_name", cfg.OTel.ServiceName, d.OTel.ServiceName},
		{"cost_per_token", cfg.Business.CostPerToken, d.Business.CostPerToken},
		{"currency_symbol", cfg.Business.CurrencySymbol, d.Business.CurrencySymbol},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("applyDefaults %s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("mode: [invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}
