package server

import (
	"context"

	"github.com/mwigge/llmctl/internal/config"
)

// Manager controls the lifecycle of the local model server.
type Manager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
	Restart(ctx context.Context) error
}

// Status describes the current state of the server.
type Status struct {
	Running  bool
	Mode     string // single|cold-swap|hot-swap|parallel
	Endpoint string
	Port     int
	Models   []string
	UptimeS  int64
}

// NewManager returns a Manager implementation suited to cfg.Mode.
// Supported modes: "single", "cold-swap", "hot-swap", "parallel".
// Any unrecognised mode is normalised to "single" before dispatch.
func NewManager(cfg *config.Config) Manager {
	switch cfg.Mode {
	case "cold-swap":
		return newSwapManager(cfg, false)
	case "hot-swap":
		return newSwapManager(cfg, true)
	case "parallel":
		return newParallelManager(cfg)
	default:
		// Normalise unknown mode to "single" so Status.Mode is always canonical.
		if cfg.Mode != "single" {
			normalised := *cfg
			normalised.Mode = "single"
			cfg = &normalised
		}
		return newSingleManager(cfg)
	}
}
