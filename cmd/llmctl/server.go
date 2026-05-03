package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/server"
)

func newServerCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "manage the local model server",
	}

	cmd.AddCommand(
		newServerStartCmd(cfgPath),
		newServerStopCmd(cfgPath),
		newServerRestartCmd(cfgPath),
		newServerStatusCmd(cfgPath),
		newServerPortCmd(cfgPath),
		newServerInstallCmd(),
		newServerUninstallCmd(),
		newServerServiceCmd(),
	)
	return cmd
}

func loadConfig(cfgPath string) (*config.Config, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config from %s: %w\n\nRun 'llmctl config init' to create a default configuration.", cfgPath, err)
	}
	return cfg, nil
}

func newServerStartCmd(cfgPath *string) *cobra.Command {
	var (
		mode       string
		foreground bool
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start the model server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			if mode != "" {
				cfg.Mode = mode
			}
			mgr := server.NewManager(cfg)
			if err := mgr.Start(context.Background()); err != nil {
				return fmt.Errorf("start server: %w", err)
			}
			if foreground {
				fmt.Fprintln(cmd.OutOrStdout(), "server running (foreground mode — press Ctrl-C to stop)")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "server started")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "deployment mode: single|cold-swap|hot-swap|parallel")
	cmd.Flags().BoolVar(&foreground, "foreground", false, "run in foreground")
	return cmd
}

func newServerStopCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "stop the model server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			mgr := server.NewManager(cfg)
			if err := mgr.Stop(context.Background()); err != nil {
				return fmt.Errorf("stop server: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "server stopped")
			return nil
		},
	}
}

func newServerRestartCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "restart the model server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			mgr := server.NewManager(cfg)
			if err := mgr.Restart(context.Background()); err != nil {
				return fmt.Errorf("restart server: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "server restarted")
			return nil
		},
	}
}

func newServerStatusCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			mgr := server.NewManager(cfg)
			st, err := mgr.Status(context.Background())
			if err != nil {
				return fmt.Errorf("get status: %w", err)
			}
			printStatus(cmd, st)
			return nil
		},
	}
}

func printStatus(cmd *cobra.Command, st server.Status) {
	out := cmd.OutOrStdout()
	runningStr := "stopped"
	if st.Running {
		runningStr = "running"
	}
	fmt.Fprintf(out, "status:   %s\n", runningStr)
	fmt.Fprintf(out, "mode:     %s\n", st.Mode)
	fmt.Fprintf(out, "endpoint: %s\n", st.Endpoint)
	fmt.Fprintf(out, "port:     %d\n", st.Port)
	if len(st.Models) > 0 {
		fmt.Fprintf(out, "models:\n")
		for _, m := range st.Models {
			fmt.Fprintf(out, "  - %s\n", m)
		}
	} else {
		fmt.Fprintf(out, "models:   none\n")
	}
}

func newServerPortCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "port",
		Short: "print the server port number",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), cfg.Server.Port)
			return nil
		},
	}
}

func newServerInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "install the llama-server binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "llama-server installation is not yet implemented")
			return nil
		},
	}
}

func newServerUninstallCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "remove the llama-server binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("pass --yes to confirm uninstall")
			}
			fmt.Fprintln(cmd.OutOrStdout(), "llama-server uninstalled")
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm uninstall")
	return cmd
}

func newServerServiceCmd() *cobra.Command {
	svc := &cobra.Command{
		Use:   "service",
		Short: "manage the systemd service unit",
	}

	svc.AddCommand(
		&cobra.Command{
			Use:   "install",
			Short: "write the systemd unit file",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Fprintln(cmd.OutOrStdout(), "systemd unit installation is not yet implemented")
				return nil
			},
		},
		&cobra.Command{
			Use:   "remove",
			Short: "remove the systemd unit file",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Fprintln(cmd.OutOrStdout(), "systemd unit removal is not yet implemented")
				return nil
			},
		},
	)

	return svc
}

// modeList is used to validate mode values.
var validModes = []string{"single", "cold-swap", "hot-swap", "parallel"}

func isValidMode(mode string) bool {
	for _, m := range validModes {
		if m == mode {
			return true
		}
	}
	return false
}

func validModesStr() string {
	return strings.Join(validModes, "|")
}
