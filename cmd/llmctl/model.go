package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/model"
)

func newModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "manage local models",
	}

	cmd.AddCommand(
		newModelListCmd(),
		newModelCatalogCmd(),
		newModelSearchCmd(),
		newModelInstallCmd(),
		newModelRemoveCmd(),
		newModelReplaceCmd(),
		newModelUpdateCmd(),
		newModelDefaultCmd(),
		newModelInfoCmd(),
	)
	return cmd
}

func newModelListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list installed models from registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Config is accessed via parent persistent flag in tests; here we
			// read it from a sibling lookup to keep the command self-contained.
			cfgPath := configPathFromFlags(cmd)
			cfg, err := config.Load(cfgPath)
			if err != nil {
				// Not fatal — an empty registry is valid.
				cfg = config.DefaultConfig()
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "alias\tpath\trole")
			for _, m := range cfg.Models {
				fmt.Fprintf(w, "%s\t%s\t%s\n", m.Alias, m.Path, m.Role)
			}
			return w.Flush()
		},
	}
}

func newModelCatalogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "show the built-in model catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "name\trepo\tquant\tsize\tram\ttool_use\treasoning")
			for _, e := range model.BuiltinCatalog {
				toolUse := "no"
				if e.ToolUse {
					toolUse = "yes"
				}
				reasoning := "no"
				if e.Reasoning {
					reasoning = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%dGB\t%s\t%s\n",
					e.Name, e.Repo, e.Quant, e.SizeGB, e.MinRAMGB, toolUse, reasoning,
				)
			}
			return w.Flush()
		},
	}
}

func newModelSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "search HuggingFace for GGUF models",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results := model.CatalogByPrefix(args[0])
			if len(results) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no models found matching %q in catalog\n", args[0])
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "name\trepo\tquant\tsize")
			for _, e := range results {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, e.Repo, e.Quant, e.SizeGB)
			}
			return w.Flush()
		},
	}
}

func newModelInstallCmd() *cobra.Command {
	var (
		quant string
		alias string
	)
	cmd := &cobra.Command{
		Use:   "install <repo-or-name>",
		Short: "download and register a model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "install %s quant=%s alias=%s — not yet implemented\n",
				args[0], quant, alias)
			return nil
		},
	}
	cmd.Flags().StringVarP(&quant, "quant", "Q", "Q4_K_M", "quantisation level")
	cmd.Flags().StringVarP(&alias, "alias", "A", "", "local alias for the model")
	return cmd
}

func newModelRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <alias>",
		Short: "unregister and delete a local model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "remove %s — not yet implemented\n", args[0])
			return nil
		},
	}
}

func newModelReplaceCmd() *cobra.Command {
	var quant string
	cmd := &cobra.Command{
		Use:   "replace <alias> <new-repo>",
		Short: "download new model and unregister old",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "replace %s with %s quant=%s — not yet implemented\n",
				args[0], args[1], quant)
			return nil
		},
	}
	cmd.Flags().StringVarP(&quant, "quant", "Q", "Q4_K_M", "quantisation level")
	return cmd
}

func newModelUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <alias>",
		Short: "re-download same repo with latest quant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "update %s — not yet implemented\n", args[0])
			return nil
		},
	}
}

func newModelDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <alias>",
		Short: "set model as default in config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "default set to %s — not yet implemented\n", args[0])
			return nil
		},
	}
}

func newModelInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <alias>",
		Short: "show model path, size, installed date, last used",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "info for %s — not yet implemented\n", args[0])
			return nil
		},
	}
}

// configPathFromFlags walks up to the root command to find --config flag value.
func configPathFromFlags(cmd *cobra.Command) string {
	root := cmd.Root()
	if f := root.PersistentFlags().Lookup("config"); f != nil {
		return f.Value.String()
	}
	return ""
}
