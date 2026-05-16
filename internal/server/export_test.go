// Package server — test export shims.
// This file is compiled only during tests. It exposes private helpers so that
// black-box tests in package server_test can call them without making the
// production surface larger than necessary.
package server

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mwigge/llmctl/internal/config"
)

// WritePIDFileAt writes pid to path, creating all parent directories.
func WritePIDFileAt(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

// ReadPIDFileAt reads and returns the PID stored in path.
func ReadPIDFileAt(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// RemovePIDFileAt removes path if it exists; a missing file is not an error.
func RemovePIDFileAt(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PIDAlive delegates to the internal pidAlive function.
func PIDAlive(pid int) bool {
	return pidAlive(pid)
}

// FetchModels delegates to the internal fetchModels function.
func FetchModels(ctx context.Context, endpoint string) []string {
	return fetchModels(ctx, endpoint)
}

// BuildArgv exposes the singleManager's buildArgv for testing.
func BuildArgv(cfg *config.Config) []string {
	return BuildLlamaServerArgs(cfg)
}

// BuildParallelArgv exposes buildParallelArgv for testing.
func BuildParallelArgv(model config.ModelRef, srv config.ServerCfg, port int) []string {
	return buildParallelArgv(model, srv, port)
}

// RouterUpstreamURL exposes Router.upstreamURL for testing.
func RouterUpstreamURL(cfg *config.Config, n int) string {
	return NewRouter(cfg).upstreamURL(n)
}

// RouterCodeModelURL exposes Router.codeModelURL for testing.
func RouterCodeModelURL(cfg *config.Config) string {
	return NewRouter(cfg).codeModelURL()
}

// RouterReasonModelURL exposes Router.reasonModelURL for testing.
func RouterReasonModelURL(cfg *config.Config) string {
	return NewRouter(cfg).reasonModelURL()
}

// LooksLikeCode exposes the looksLikeCode function for testing.
func LooksLikeCode(body string) bool {
	return looksLikeCode(body)
}

// WritePIDFileInternal tests the internal writePIDFile function.
func WritePIDFileInternal(pid int) error {
	return writePIDFile(pidFilePath, pid)
}

// RemovePIDFileInternal tests the internal removePIDFile function.
func RemovePIDFileInternal() error {
	return removePIDFile(pidFilePath)
}

// SwapConfigPath exposes swapConfigPath for testing.
func SwapConfigPath() (string, error) {
	return swapConfigPath()
}

// StopProcess exposes stopProcess for testing.
func StopProcess(pathFn func() (string, error)) error {
	return stopProcess(pathFn)
}
