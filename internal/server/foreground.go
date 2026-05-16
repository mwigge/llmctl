package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/mwigge/llmctl/internal/config"
)

// RunForeground execs llama-server in the foreground for service managers.
func RunForeground(ctx context.Context, cfg *config.Config) error {
	bin, err := FindLlamaServer()
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	cmd := exec.CommandContext(ctx, bin, BuildLlamaServerArgs(cfg)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
