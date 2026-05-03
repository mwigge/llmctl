package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mwigge/llmctl/internal/config"
)

func newConfigCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "manage llmctl configuration",
	}

	cmd.AddCommand(
		newConfigShowCmd(cfgPath),
		newConfigSetCmd(cfgPath),
		newConfigModeCmd(cfgPath),
		newConfigInitCmd(cfgPath),
	)
	return cmd
}

func newConfigShowCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "print current configuration as YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}

func newConfigSetCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "set a configuration value by dotted key path",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			if err := applyConfigKey(cfg, args[0], args[1]); err != nil {
				return err
			}
			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func newConfigModeCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "mode <mode>",
		Short: "switch deployment mode (" + validModesStr() + ")",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isValidMode(args[0]) {
				return fmt.Errorf("unknown mode %q; valid modes: %s", args[0], validModesStr())
			}
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			cfg.Mode = args[0]
			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "mode set to %s\n", args[0])
			return nil
		},
	}
}

func newConfigInitCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "write default config to the config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.DefaultConfig()
			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("write default config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config written to %s\n", *cfgPath)
			return nil
		},
	}
}

// applyConfigKey sets a dotted key path on cfg.
// Supported keys: server.host, server.port, server.ctx_size, server.threads,
// server.gpu_layers, server.temp, server.max_tokens,
// metrics.db_path, metrics.prometheus_port,
// otel.endpoint, otel.service_name,
// business.cost_per_token, business.currency_symbol,
// mode.
func applyConfigKey(cfg *config.Config, key, val string) error {
	switch strings.ToLower(key) {
	case "mode":
		if !isValidMode(val) {
			return fmt.Errorf("invalid mode %q; valid: %s", val, validModesStr())
		}
		cfg.Mode = val
	case "server.host":
		cfg.Server.Host = val
	case "server.port":
		var v int
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("server.port must be integer: %w", err)
		}
		cfg.Server.Port = v
	case "server.ctx_size":
		var v int
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("server.ctx_size must be integer: %w", err)
		}
		cfg.Server.CtxSize = v
	case "server.threads":
		var v int
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("server.threads must be integer: %w", err)
		}
		cfg.Server.Threads = v
	case "server.gpu_layers":
		var v int
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("server.gpu_layers must be integer: %w", err)
		}
		cfg.Server.GPULayers = v
	case "server.temp":
		var v float64
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("server.temp must be float: %w", err)
		}
		cfg.Server.Temp = v
	case "server.max_tokens":
		var v int
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("server.max_tokens must be integer: %w", err)
		}
		cfg.Server.MaxTokens = v
	case "metrics.db_path":
		cfg.Metrics.DBPath = val
	case "metrics.prometheus_port":
		var v int
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("metrics.prometheus_port must be integer: %w", err)
		}
		cfg.Metrics.PrometheusPort = v
	case "otel.endpoint":
		cfg.OTel.Endpoint = val
	case "otel.service_name":
		cfg.OTel.ServiceName = val
	case "business.cost_per_token":
		var v float64
		if _, err := fmt.Sscan(val, &v); err != nil {
			return fmt.Errorf("business.cost_per_token must be float: %w", err)
		}
		cfg.Business.CostPerToken = v
	case "business.currency_symbol":
		cfg.Business.CurrencySymbol = val
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}
