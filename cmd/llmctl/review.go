package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mwigge/llmctl/internal/runner/review"
	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	var (
		model     string
		out       string
		resume    bool
		noMemory  bool
		gitCommit bool
		lint      bool
	)

	cmd := &cobra.Command{
		Use:   "review <repo-path>",
		Short: "Review a repository with the loaded local model",
		Long: `Runs a structured code review of <repo-path> using the local inference server.

The runner automatically detects languages (Go, Rust, Python, TypeScript, YAML),
groups files by directory, reviews each group with the active model, and writes
findings to a scratch file. A final executive summary is produced at the end.

The local inference server must be running (llmctl server start).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath := args[0]
			if _, err := os.Stat(repoPath); err != nil {
				return fmt.Errorf("repo path: %w", err)
			}

			endpoint := strings.TrimRight(os.Getenv("LLMCTL_ENDPOINT"), "/")
			if endpoint == "" {
				endpoint = "http://localhost:8765/v1"
			}

			cfg := review.Config{
				RepoPath:      repoPath,
				Endpoint:      endpoint,
				ModelAlias:    model,
				OutPath:       out,
				Resume:        resume,
				NoMemory:      noMemory,
				GitCommit:     gitCommit,
				LintAfterEdit: lint,
			}

			runner, err := review.New(cfg)
			if err != nil {
				return fmt.Errorf("init runner: %w", err)
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			fmt.Fprintf(cmd.ErrOrStderr(), "reviewing %s …\n", repoPath)
			result, err := runner.Run(ctx, cfg)
			if err != nil {
				return fmt.Errorf("review: %w", err)
			}

			report := result.Summary
			if out != "" {
				if err := os.WriteFile(out, []byte(report), 0o644); err != nil {
					return fmt.Errorf("write report: %w", err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "report written to %s\n", out)
			} else {
				fmt.Fprint(cmd.OutOrStdout(), report)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "\ngroups: %d  findings: %d  model: %s\n",
				len(result.Groups), len(result.Findings), result.Model)
			return nil
		},
	}

	cmd.Flags().StringVarP(&model, "model", "m", "", "override loaded model alias (default: auto-detect)")
	cmd.Flags().StringVarP(&out, "out", "o", "", "write report to file (default: stdout)")
	cmd.Flags().BoolVar(&resume, "resume", false, "continue from an existing scratch file")
	cmd.Flags().BoolVar(&noMemory, "no-memory", true, "skip MemPalace (default: true for llmctl standalone)")
	cmd.Flags().BoolVar(&gitCommit, "git-commit", false, "auto-commit after each group that produces edits")
	cmd.Flags().BoolVar(&lint, "lint", false, "run build/tests after edits and add failures to findings")

	return cmd
}
