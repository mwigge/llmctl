package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mwigge/llmctl/internal/config"
)

// singleManager manages exactly one llama-server process.
type singleManager struct {
	cfg     *config.Config
	startAt time.Time
}

func newSingleManager(cfg *config.Config) *singleManager {
	return &singleManager{cfg: cfg}
}

// pidFilePath returns the path to the PID file for llama-server.
func pidFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "llmctl", "llama-server.pid"), nil
}

// Start finds the llama-server binary, builds argv from config, and launches the process.
func (m *singleManager) Start(ctx context.Context) error {
	bin, err := FindLlamaServer()
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}

	args := m.buildArgv()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start llama-server: %w", err)
	}

	if err := writePIDFile(pidFilePath, cmd.Process.Pid); err != nil {
		// Best-effort: log but do not fail startup over PID file.
		_ = cmd.Process.Kill()
		return fmt.Errorf("write pid file: %w", err)
	}

	m.startAt = time.Now()
	return nil
}

// Stop reads the PID file, sends SIGTERM, waits up to 5 s, then SIGKILLs.
func (m *singleManager) Stop(_ context.Context) error {
	pid, err := readPIDFile(pidFilePath)
	if err != nil {
		return fmt.Errorf("stop: %w", err)
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

	return removePIDFile(pidFilePath)
}

// Status checks whether the process is alive and the API endpoint is reachable.
func (m *singleManager) Status(ctx context.Context) (Status, error) {
	pid, err := readPIDFile(pidFilePath)
	if err != nil {
		return Status{
			Running: false,
			Mode:    "single",
			Port:    m.cfg.Server.Port,
		}, nil
	}

	running := pidAlive(pid)

	endpoint := fmt.Sprintf("http://%s:%d", m.cfg.Server.Host, m.cfg.Server.Port)
	models := fetchModels(ctx, endpoint)

	uptime := int64(0)
	if !m.startAt.IsZero() && running {
		uptime = int64(time.Since(m.startAt).Seconds())
	}

	return Status{
		Running:  running,
		Mode:     "single",
		Endpoint: endpoint,
		Port:     m.cfg.Server.Port,
		Models:   models,
		UptimeS:  uptime,
	}, nil
}

// Restart stops then starts the server.
func (m *singleManager) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		// Ignore "not running" errors during restart.
		_ = err
	}
	return m.Start(ctx)
}

// buildArgv constructs the argument slice for llama-server.
func (m *singleManager) buildArgv() []string {
	cfg := m.cfg
	var args []string

	if len(cfg.Models) > 0 {
		args = append(args, "-m", cfg.Models[0].Path)
		if cfg.Models[0].Alias != "" {
			args = append(args, "--alias", cfg.Models[0].Alias)
		}
	}

	args = append(args,
		"--host", cfg.Server.Host,
		"--port", strconv.Itoa(cfg.Server.Port),
		"--ctx-size", strconv.Itoa(cfg.Server.CtxSize),
		"--threads", strconv.Itoa(cfg.Server.Threads),
	)

	if cfg.Server.GPULayers > 0 {
		args = append(args, "--n-gpu-layers", strconv.Itoa(cfg.Server.GPULayers))
	}

	return args
}

// pidFilePath returns the PID file path; fn is passed as a func to allow testing.
func writePIDFile(pathFn func() (string, error), pid int) error {
	path, err := pathFn()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

func readPIDFile(pathFn func() (string, error)) (int, error) {
	path, err := pathFn()
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pid: %w", err)
	}
	return pid, nil
}

func removePIDFile(pathFn func() (string, error)) error {
	path, err := pathFn()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pid file: %w", err)
	}
	return nil
}

// pidAlive returns true if the process with the given PID is alive.
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// modelsResponse is the subset of /v1/models we care about.
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// fetchModels calls GET /v1/models and returns the list of model IDs.
// Returns nil on any error (non-fatal for Status calls).
func fetchModels(ctx context.Context, endpoint string) []string {
	url := endpoint + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil
	}

	models := make([]string, 0, len(mr.Data))
	for _, d := range mr.Data {
		models = append(models, d.ID)
	}
	return models
}
