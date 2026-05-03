package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mwigge/llmctl/internal/bundle"
)

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "manage offline model bundles",
	}

	cmd.AddCommand(
		newBundleCreateCmd(),
		newBundleInstallCmd(),
	)
	return cmd
}

func newBundleCreateCmd() *cobra.Command {
	var (
		output        string
		includeBinary bool
		registryPath  string
	)
	cmd := &cobra.Command{
		Use:   "create <alias>[,alias2,...]",
		Short: "create an offline bundle from installed models",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			aliases := strings.Split(args[0], ",")
			for i, a := range aliases {
				aliases[i] = strings.TrimSpace(a)
			}

			if output == "" {
				output = "llmctl-bundle.tar.gz"
			}

			if registryPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home dir: %w", err)
				}
				registryPath = filepath.Join(home, ".local", "share", "llmctl", "models.json")
			}

			opts := bundle.BundleOptions{
				ModelAliases:  aliases,
				OutputPath:    output,
				IncludeBinary: includeBinary,
				RegistryPath:  registryPath,
			}

			fmt.Fprintf(cmd.OutOrStdout(), "creating bundle for models: %s\n", strings.Join(aliases, ", "))
			if err := bundle.Create(context.Background(), opts); err != nil {
				return fmt.Errorf("create bundle: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "bundle written to %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output path (default: llmctl-bundle.tar.gz)")
	cmd.Flags().BoolVar(&includeBinary, "include-binary", false, "include llmctl binary in the bundle")
	cmd.Flags().StringVar(&registryPath, "registry", "", "path to the model registry JSON")
	return cmd
}

func newBundleInstallCmd() *cobra.Command {
	var (
		destDir      string
		registryPath string
	)
	cmd := &cobra.Command{
		Use:   "install <bundle-path>",
		Short: "extract and register a bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if destDir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home dir: %w", err)
				}
				destDir = filepath.Join(home, ".local", "share", "llmctl", "bundles")
			}

			if registryPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home dir: %w", err)
				}
				registryPath = filepath.Join(home, ".local", "share", "llmctl", "models.json")
			}

			fmt.Fprintf(cmd.OutOrStdout(), "installing bundle %s\n", args[0])
			if err := bundle.Install(context.Background(), args[0], destDir, registryPath); err != nil {
				return fmt.Errorf("install bundle: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "bundle installed successfully")
			return nil
		},
	}
	cmd.Flags().StringVar(&destDir, "dest", "", "destination directory for extracted files")
	cmd.Flags().StringVar(&registryPath, "registry", "", "path to the model registry JSON")
	return cmd
}
