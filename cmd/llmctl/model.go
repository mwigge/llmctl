package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

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
		quant     string
		alias     string
		copyLocal bool
		force     bool
	)
	cmd := &cobra.Command{
		Use:   "install <repo-or-name>",
		Short: "install a model from a local GGUF, URL, HF repo, or catalog name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := configPathFromFlags(cmd)
			cfg, err := config.Load(cfgPath)
			if err != nil {
				cfg = config.DefaultConfig()
			}
			installed, err := installModelSource(cmd, args[0], quant, alias, copyLocal, force)
			if err != nil {
				return err
			}
			cfg.Models = upsertModelRef(cfg.Models, config.ModelRef{
				Alias: installed.Alias,
				Path:  installed.Path,
				Role:  "code",
			})
			if err := config.Save(cfg, cfgPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[ok] %s -> %s\n", installed.Alias, installed.Path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&quant, "quant", "Q", "Q4_K_M", "quantisation level")
	cmd.Flags().StringVarP(&alias, "alias", "A", "", "local alias for the model")
	cmd.Flags().BoolVar(&copyLocal, "copy", false, "copy a local GGUF into llmctl's model cache instead of registering it in place")
	cmd.Flags().BoolVar(&force, "force", false, "redownload or overwrite cached model file")
	return cmd
}

type installedModelRef struct {
	Alias string
	Path  string
	Repo  string
	Quant string
}

func installModelSource(cmd *cobra.Command, source, quant, alias string, copyLocal, force bool) (installedModelRef, error) {
	if alias == "" {
		alias = deriveModelAlias(source)
	}
	modelDir, err := localModelDir()
	if err != nil {
		return installedModelRef{}, err
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return installedModelRef{}, fmt.Errorf("create model cache: %w", err)
	}

	if path, ok := localGGUFPath(source); ok {
		target := path
		if copyLocal {
			target = filepath.Join(modelDir, filepath.Base(path))
			if err := copyFile(path, target, 0o644); err != nil {
				return installedModelRef{}, fmt.Errorf("copy local model: %w", err)
			}
		}
		return installedModelRef{Alias: alias, Path: target, Quant: quant}, nil
	}

	if isHTTPURL(source) {
		path, err := downloadURL(context.Background(), source, modelDir, force, cmd.ErrOrStderr())
		if err != nil {
			return installedModelRef{}, err
		}
		return installedModelRef{Alias: alias, Path: path, Repo: source, Quant: quant}, nil
	}

	repo := source
	if entry, ok := catalogEntryByName(source); ok {
		repo = entry.Repo
		if quant == "" {
			quant = entry.Quant
		}
		if alias == "" {
			alias = strings.ToLower(entry.Name)
		}
	}
	if quant == "" {
		quant = "Q4_K_M"
	}
	path, err := model.Download(context.Background(), model.DownloadOptions{
		Repo:     repo,
		Quant:    quant,
		Alias:    alias,
		DestDir:  modelDir,
		HFToken:  os.Getenv("HF_TOKEN"),
		Force:    force,
		Progress: cmd.ErrOrStderr(),
	})
	if err != nil {
		return installedModelRef{}, err
	}
	return installedModelRef{Alias: alias, Path: path, Repo: repo, Quant: quant}, nil
}

func localGGUFPath(source string) (string, bool) {
	if source == "" || isHTTPURL(source) {
		return "", false
	}
	if info, err := os.Stat(source); err == nil && !info.IsDir() {
		abs, _ := filepath.Abs(source)
		return abs, true
	}
	return "", false
}

func isHTTPURL(source string) bool {
	u, err := url.Parse(source)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

func downloadURL(ctx context.Context, source, modelDir string, force bool, progress io.Writer) (string, error) {
	u, err := url.Parse(source)
	if err != nil {
		return "", err
	}
	name := filepath.Base(u.Path)
	if name == "." || name == "/" || name == "" {
		name = "model-" + fmt.Sprint(time.Now().Unix()) + ".gguf"
	}
	dest := filepath.Join(modelDir, name)
	if !force {
		if info, err := os.Stat(dest); err == nil && info.Size() > 50*1024*1024 {
			return dest, nil
		}
	}
	args := []string{"-fL", "-C", "-", "-o", dest, source}
	c := exec.Command("curl", args...)
	c.Stderr = progress
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("download %s: %w", source, err)
	}
	return dest, nil
}

func deriveModelAlias(source string) string {
	for _, entry := range model.BuiltinCatalog {
		if strings.EqualFold(source, entry.Name) || strings.EqualFold(source, entry.Repo) {
			return strings.ToLower(entry.Name)
		}
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	base = strings.TrimSuffix(base, "-GGUF")
	base = strings.TrimSuffix(base, "-Q4_K_M")
	if base == "." || base == "" {
		return "local"
	}
	return strings.ToLower(base)
}

func catalogEntryByName(name string) (model.CatalogEntry, bool) {
	lower := strings.ToLower(name)
	for _, entry := range model.BuiltinCatalog {
		if strings.ToLower(entry.Name) == lower || strings.ToLower(entry.Repo) == lower {
			return entry, true
		}
	}
	return model.CatalogEntry{}, false
}

func upsertModelRef(models []config.ModelRef, ref config.ModelRef) []config.ModelRef {
	out := append([]config.ModelRef(nil), models...)
	for i := range out {
		if out[i].Alias == ref.Alias {
			out[i] = ref
			return out
		}
	}
	return append([]config.ModelRef{ref}, out...)
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
