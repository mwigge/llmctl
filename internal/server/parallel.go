package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mwigge/llmctl/internal/config"
)

// parallelManager manages two llama-server processes side-by-side.
type parallelManager struct {
	cfg     *config.Config
	startAt time.Time
}

func newParallelManager(cfg *config.Config) *parallelManager {
	return &parallelManager{cfg: cfg}
}

// parallelPIDFilePath returns the PID file path for the n-th process (0-based).
func parallelPIDFilePath(n int) func() (string, error) {
	return func() (string, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		name := fmt.Sprintf("llama-server-%d.pid", n)
		return filepath.Join(home, ".local", "state", "llmctl", name), nil
	}
}

// Start launches both llama-server processes.
func (m *parallelManager) Start(ctx context.Context) error {
	bin, err := FindLlamaServer()
	if err != nil {
		return fmt.Errorf("start parallel: %w", err)
	}

	for i := 0; i < 2 && i < len(m.cfg.Models); i++ {
		model := m.cfg.Models[i]
		port := m.cfg.Server.Port + i
		args := buildParallelArgv(model, m.cfg.Server, port)

		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start process %d: %w", i, err)
		}

		if err := writePIDFile(parallelPIDFilePath(i), cmd.Process.Pid); err != nil {
			_ = cmd.Process.Kill()
			return fmt.Errorf("write pid file %d: %w", i, err)
		}
	}

	m.startAt = time.Now()
	return nil
}

// Stop sends SIGTERM to both processes, waits 5 s each, then SIGKILLs.
func (m *parallelManager) Stop(_ context.Context) error {
	var firstErr error
	for i := 0; i < 2; i++ {
		if err := stopProcess(parallelPIDFilePath(i)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Status checks both processes and returns combined info.
func (m *parallelManager) Status(ctx context.Context) (Status, error) {
	allRunning := true
	var models []string

	for i := 0; i < 2 && i < len(m.cfg.Models); i++ {
		port := m.cfg.Server.Port + i
		pid, err := readPIDFile(parallelPIDFilePath(i))
		if err != nil || !pidAlive(pid) {
			allRunning = false
			continue
		}
		endpoint := fmt.Sprintf("http://%s:%d", m.cfg.Server.Host, port)
		ms := fetchModels(ctx, endpoint)
		models = append(models, ms...)
	}

	endpoint := fmt.Sprintf("http://%s:%d", m.cfg.Server.Host, m.cfg.Server.Port)
	uptime := int64(0)
	if !m.startAt.IsZero() && allRunning {
		uptime = int64(time.Since(m.startAt).Seconds())
	}

	return Status{
		Running:  allRunning,
		Mode:     "parallel",
		Endpoint: endpoint,
		Port:     m.cfg.Server.Port,
		Models:   models,
		UptimeS:  uptime,
	}, nil
}

// Restart stops then starts both processes.
func (m *parallelManager) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		_ = err
	}
	return m.Start(ctx)
}

// stopProcess terminates the process described by pathFn.
func stopProcess(pathFn func() (string, error)) error {
	pid, err := readPIDFile(pathFn)
	if err != nil {
		return fmt.Errorf("stop process: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sigterm pid %d: %w", pid, err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := proc.Wait()
		done <- err
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = proc.Signal(syscall.SIGKILL)
		<-done
	}

	return removePIDFile(pathFn)
}

// buildParallelArgv constructs the argument slice for one of the parallel processes.
func buildParallelArgv(model config.ModelRef, srv config.ServerCfg, port int) []string {
	args := []string{
		"-m", model.Path,
		"--host", srv.Host,
		"--port", strconv.Itoa(port),
		"--ctx-size", strconv.Itoa(srv.CtxSize),
		"--threads", strconv.Itoa(srv.Threads),
	}
	if model.Alias != "" {
		args = append(args, "--alias", model.Alias)
	}
	if srv.GPULayers > 0 {
		args = append(args, "--n-gpu-layers", strconv.Itoa(srv.GPULayers))
	}
	return args
}
