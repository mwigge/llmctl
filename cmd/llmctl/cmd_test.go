package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// executeCmd runs the root command with the given args and returns
// combined stdout output and any execution error.
func executeCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	root := newRootCmd()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestRootCmd_Version(t *testing.T) {
	t.Parallel()
	out, err := executeCmd(t, "--version")
	if err != nil {
		t.Fatalf("--version error = %v", err)
	}
	if !strings.Contains(out, "llmctl") {
		t.Errorf("--version output %q does not contain 'llmctl'", out)
	}
}

func TestServerCmd_StatusNoConfig(t *testing.T) {
	t.Parallel()
	// Point to a config that doesn't exist so we get a helpful error.
	_, err := executeCmd(t, "--config", "/nonexistent/config.yaml", "server", "status")
	if err == nil {
		t.Fatal("server status with missing config expected error, got nil")
	}
}

func TestModelCmd_Catalog(t *testing.T) {
	t.Parallel()
	out, err := executeCmd(t, "model", "catalog")
	if err != nil {
		t.Fatalf("model catalog error = %v", err)
	}
	if !strings.Contains(out, "Qwen") {
		t.Errorf("model catalog output %q does not contain 'Qwen'", out)
	}
}

func TestConfigCmd_Init(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	_, err := executeCmd(t, "--config", cfgPath, "config", "init")
	if err != nil {
		t.Fatalf("config init error = %v", err)
	}

	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("config file not created at %s: %v", cfgPath, statErr)
	}
}

func TestMetricsCmd_Show_NoData(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")
	cfgPath := filepath.Join(dir, "config.yaml")

	// Write a minimal config pointing at the temp DB.
	cfgContent := "metrics:\n  db_path: " + dbPath + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	out, err := executeCmd(t, "--config", cfgPath, "metrics", "show")
	if err != nil {
		t.Fatalf("metrics show error = %v", err)
	}
	// Must print a header row even with no data.
	if !strings.Contains(out, "model") {
		t.Errorf("metrics show output %q does not contain header 'model'", out)
	}
}
