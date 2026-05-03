package config

// DefaultConfig returns a Config populated with sensible defaults.
// Callers should merge user-provided values on top of the defaults.
func DefaultConfig() *Config {
	return &Config{
		Mode: "single",
		Server: ServerCfg{
			Host:      "127.0.0.1",
			Port:      8765,
			CtxSize:   16384,
			Threads:   4,
			GPULayers: 0,
			Temp:      0.7,
			MaxTokens: 512,
		},
		Metrics: MetricsCfg{
			DBPath:         "llmctl.db",
			PrometheusPort: 9090,
		},
		OTel: OTelCfg{
			ServiceName: "llmctl",
		},
		Business: BusinessCfg{
			CostPerToken:   0.0001,
			CurrencySymbol: "$",
		},
	}
}
