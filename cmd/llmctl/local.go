package main

import "github.com/spf13/cobra"

func newLocalCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "local",
		Short: "compatibility aliases for local model server commands",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "install-server",
			Short: "install or refresh llama-server and the default local model",
			RunE: func(cmd *cobra.Command, args []string) error {
				return installLocalServer(cmd, serverInstallOptions{ConfigPath: *cfgPath, Accel: "auto"})
			},
		},
		newLocalInstallGPUCmd(cfgPath),
		newLocalInstallSwapCmd(cfgPath),
		&cobra.Command{
			Use:   "server-status",
			Short: "show local server status",
			RunE: func(cmd *cobra.Command, args []string) error {
				return newServerStatusCmd(cfgPath).RunE(cmd, args)
			},
		},
		&cobra.Command{
			Use:   "server-port",
			Short: "print local server port",
			RunE: func(cmd *cobra.Command, args []string) error {
				return newServerPortCmd(cfgPath).RunE(cmd, args)
			},
		},
	)
	return cmd
}

func newLocalInstallSwapCmd(cfgPath *string) *cobra.Command {
	var hot bool
	cmd := &cobra.Command{
		Use:   "install-swap",
		Short: "configure hot or cold model swap serving",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := "cold-swap"
			if hot {
				mode = "hot-swap"
			}
			return configureSwapMode(cmd, *cfgPath, mode)
		},
	}
	cmd.Flags().BoolVar(&hot, "hot", false, "preload swap models instead of loading on demand")
	return cmd
}

func newLocalInstallGPUCmd(cfgPath *string) *cobra.Command {
	var (
		dryRun bool
		accel  string
		budget float64
	)
	cmd := &cobra.Command{
		Use:   "install-gpu-server",
		Short: "detect GPU hardware and install the largest fitting curated model",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installLocalServer(cmd, serverInstallOptions{
				ConfigPath: *cfgPath,
				GPU:        true,
				DryRun:     dryRun,
				Accel:      accel,
				Budget:     budget,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the selected hardware/model plan without changing files")
	cmd.Flags().StringVar(&accel, "accel", "auto", "GPU acceleration: auto|vulkan|cuda|hip")
	cmd.Flags().Float64Var(&budget, "resource-budget", 0.80, "fraction of CPU/RAM/GPU memory available for model serving")
	return cmd
}
