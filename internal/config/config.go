package config

// Config is the top-level configuration for llmctl.
type Config struct {
	Mode     string      `yaml:"mode"`
	Models   []ModelRef  `yaml:"models"`
	Server   ServerCfg   `yaml:"server"`
	Metrics  MetricsCfg  `yaml:"metrics"`
	OTel     OTelCfg     `yaml:"otel"`
	Business BusinessCfg `yaml:"business"`
}

// ModelRef identifies a model by alias, filesystem path, and optional role.
type ModelRef struct {
	Alias string `yaml:"alias"`
	Path  string `yaml:"path"`
	Role  string `yaml:"role"`
}

// ServerCfg holds llama.cpp server parameters.
type ServerCfg struct {
	Host      string  `yaml:"host"`
	Port      int     `yaml:"port"`
	CtxSize   int     `yaml:"ctx_size"`
	Threads   int     `yaml:"threads"`
	GPULayers int     `yaml:"gpu_layers"`
	Temp      float64 `yaml:"temp"`
	MaxTokens int     `yaml:"max_tokens"`
}

// MetricsCfg configures the metrics backend.
type MetricsCfg struct {
	DBPath         string `yaml:"db_path"`
	PrometheusPort int    `yaml:"prometheus_port"`
}

// OTelCfg configures the OpenTelemetry exporter.
type OTelCfg struct {
	Endpoint    string `yaml:"endpoint"`
	ServiceName string `yaml:"service_name"`
}

// BusinessCfg holds cost accounting parameters.
type BusinessCfg struct {
	CostPerToken   float64 `yaml:"cost_per_token"`
	CurrencySymbol string  `yaml:"currency_symbol"`
}
