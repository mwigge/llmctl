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

func newLocalInstallGPUCmd(cfgPath *string) *cobra.Command {
	var (
		dryRun bool
		accel  string
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
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the selected hardware/model plan without changing files")
	cmd.Flags().StringVar(&accel, "accel", "auto", "GPU acceleration: auto|vulkan|cuda|hip")
	return cmd
}
