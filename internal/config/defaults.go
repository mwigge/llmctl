package config

// DefaultConfig returns a Config populated with sensible defaults.
// Callers should merge user-provided values on top of the defaults.
func DefaultConfig() *Config {
	return &Config{
		Mode: "single",
		// Default to the 7B coder model — large enough for reliable tool use,
		// small enough for 8GB machines. Path is empty until installed via
		// llmctl model install Qwen2.5-Coder-7B.
		Models: []ModelRef{
			{Alias: "qwen2.5-coder-7b", Path: "", Role: "code"},
		},
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
