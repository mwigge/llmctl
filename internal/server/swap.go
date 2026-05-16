package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mwigge/llmctl/internal/config"
	"gopkg.in/yaml.v3"
)

// swapManager manages a single llama-swap process (cold-swap or hot-swap).
type swapManager struct {
	cfg     *config.Config
	hot     bool
	startAt time.Time
}

func newSwapManager(cfg *config.Config, hot bool) *swapManager {
	return &swapManager{cfg: cfg, hot: hot}
}

// swapPIDFilePath returns the path to the PID file for llama-swap.
func swapPIDFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "llmctl", "llama-swap.pid"), nil
}

// swapConfigPath returns the path to the generated llama-swap config file.
func swapConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "llmctl", "llama-swap.yaml"), nil
}

// Start writes the llama-swap config, then launches the process.
func (m *swapManager) Start(ctx context.Context) error {
	bin, err := FindLlamaSwap()
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}

	ttl := m.cfg.Server.MaxTokens // repurpose field; 0 = hot
	if m.hot {
		ttl = 0
	} else if ttl == 0 {
		ttl = 600 // default cold TTL
	}

	yamlData, err := BuildSwapConfig(m.cfg.Models, m.hot, ttl, m.cfg.Server.Port, m.cfg.Server.CtxSize)
	if err != nil {
		return fmt.Errorf("build swap config: %w", err)
	}

	cfgPath, err := swapConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(cfgPath, yamlData, 0o600); err != nil {
		return fmt.Errorf("write swap config: %w", err)
	}

	cmd := exec.CommandContext(ctx, bin, "--config", cfgPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start llama-swap: %w", err)
	}

	if err := writePIDFile(swapPIDFilePath, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("write swap pid file: %w", err)
	}

	m.startAt = time.Now()
	return nil
}

// Stop sends SIGTERM to the llama-swap process, waits 5 s, then SIGKILLs.
func (m *swapManager) Stop(_ context.Context) error {
	pid, err := readPIDFile(swapPIDFilePath)
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

	return removePIDFile(swapPIDFilePath)
}

// Status checks whether the llama-swap process is alive.
func (m *swapManager) Status(ctx context.Context) (Status, error) {
	mode := "cold-swap"
	if m.hot {
		mode = "hot-swap"
	}

	pid, err := readPIDFile(swapPIDFilePath)
	if err != nil {
		return Status{Running: false, Mode: mode, Port: m.cfg.Server.Port}, nil
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
		Mode:     mode,
		Endpoint: endpoint,
		Port:     m.cfg.Server.Port,
		Models:   models,
		UptimeS:  uptime,
	}, nil
}

// Restart stops then starts the llama-swap process.
func (m *swapManager) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		_ = err
	}
	return m.Start(ctx)
}

// swapModelEntry is used for YAML serialisation of a single model entry.
type swapModelEntry struct {
	Cmd string `yaml:"cmd"`
	TTL int    `yaml:"ttl"`
}

// BuildSwapConfig generates the llama-swap YAML configuration.
// hot=true sets ttl:0 on every model (never unload).
// ttl is the cold-swap TTL in seconds (ignored when hot=true).
// Exported so that tests in the server_test package can use it directly.
func BuildSwapConfig(models []config.ModelRef, hot bool, ttl, port, ctxSize int) ([]byte, error) {
	type swapCfg struct {
		Models map[string]swapModelEntry `yaml:"models"`
	}

	modelTTL := ttl
	if hot {
		modelTTL = 0
	}

	entries := make(map[string]swapModelEntry, len(models))
	for i, m := range models {
		modelPort := port + i
		cmd := buildModelCmd(m, modelPort, ctxSize)
		alias := m.Alias
		if alias == "" {
			alias = fmt.Sprintf("model-%d", i)
		}
		entries[alias] = swapModelEntry{Cmd: cmd, TTL: modelTTL}
	}

	cfg := swapCfg{Models: entries}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal swap config: %w", err)
	}
	return data, nil
}

// buildModelCmd constructs the llama-server command string for llama-swap.
func buildModelCmd(m config.ModelRef, port, ctxSize int) string {
	var sb strings.Builder
	if bin, err := FindLlamaServer(); err == nil {
		sb.WriteString(bin)
	} else {
		sb.WriteString("llama-server")
	}
	sb.WriteString(" -m ")
	sb.WriteString(m.Path)
	if m.Alias != "" {
		sb.WriteString(" --alias ")
		sb.WriteString(m.Alias)
	}
	sb.WriteString(" --port ")
	sb.WriteString(strconv.Itoa(port))
	sb.WriteString(" --ctx-size ")
	sb.WriteString(strconv.Itoa(ctxSize))
	sb.WriteString(" --jinja -fa on")
	return sb.String()
}
