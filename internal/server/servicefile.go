package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/mwigge/llmctl/internal/config"
)

const serviceTemplate = `[Unit]
Description=llmctl local model server

[Service]
ExecStart={{.ExecStart}}
Restart=on-failure

[Install]
WantedBy=default.target
`

// ServiceName returns the systemd user unit name for llmctl.
func ServiceName() string {
	return "llmctl-server.service"
}

// serviceFilePath returns ~/.config/systemd/user/llmctl-server.service.
func serviceFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", ServiceName()), nil
}

// WriteServiceFile writes the systemd user unit file and returns its path.
func WriteServiceFile(cfg *config.Config) (string, error) {
	execStart, err := resolveExecStart(cfg)
	if err != nil {
		return "", err
	}

	path, err := serviceFilePath()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create systemd dir: %w", err)
	}

	tmpl, err := template.New("service").Parse(serviceTemplate)
	if err != nil {
		return "", fmt.Errorf("parse service template: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("open service file: %w", err)
	}
	defer f.Close()

	data := struct{ ExecStart string }{ExecStart: execStart}
	if err := tmpl.Execute(f, data); err != nil {
		return "", fmt.Errorf("render service template: %w", err)
	}

	return path, nil
}

// RemoveServiceFile deletes the systemd user unit file.
func RemoveServiceFile() error {
	path, err := serviceFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove service file: %w", err)
	}
	return nil
}

// resolveExecStart finds the llmctl binary and builds the ExecStart value.
func resolveExecStart(_ *config.Config) (string, error) {
	bin, err := exec.LookPath("llmctl")
	if err != nil {
		// Fall back to a relative path placeholder that the user can adjust.
		bin = "/usr/local/bin/llmctl"
	}
	return bin + " server start --foreground", nil
}
