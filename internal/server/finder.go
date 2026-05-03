package server

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// wellKnownDirs lists directories to search before falling back to PATH.
var wellKnownDirs = []string{
	"/usr/bin",
	"/usr/local/bin",
	"/opt/homebrew/bin",
}

// localBin returns ~/.local/bin resolved from the current user's HOME.
func localBin() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

// findBinary searches wellKnownDirs, ~/.local/bin, then PATH for name.
func findBinary(name string) (string, error) {
	dirs := append(wellKnownDirs, localBin()) //nolint:gocritic // intentional new slice
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	// Fall back to PATH lookup.
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in well-known dirs or PATH: %w", name, errors.New("not found"))
	}
	return path, nil
}

// FindLlamaServer returns the absolute path to llama-server.
func FindLlamaServer() (string, error) {
	return findBinary("llama-server")
}

// FindLlamaSwap returns the absolute path to llama-swap.
func FindLlamaSwap() (string, error) {
	return findBinary("llama-swap")
}
