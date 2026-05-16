package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	var cfgPath string

	root := &cobra.Command{
		Use:     "llmctl",
		Short:   "local LLM management",
		Version: version,
	}

	defaultCfg := filepath.Join(os.Getenv("HOME"), ".config", "llmctl", "config.yaml")
	root.PersistentFlags().StringVar(&cfgPath, "config", defaultCfg, "path to config file")

	root.AddCommand(
		newServerCmd(&cfgPath),
		newLocalCmd(&cfgPath),
		newModelCmd(),
		newConfigCmd(&cfgPath),
		newMetricsCmd(&cfgPath),
		newBundleCmd(),
		newReviewCmd(),
	)
	return root
}
