package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/server"
)

// ----------------------------------------------------------------------------
// Internal PID helpers (writePIDFile / removePIDFile)
// ----------------------------------------------------------------------------

func TestWritePIDFileInternal_And_Remove(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv (HOME affects pidFilePath).
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := server.WritePIDFileInternal(os.Getpid()); err != nil {
		t.Fatalf("WritePIDFileInternal: %v", err)
	}

	// Verify the file exists at the expected location.
	expected := filepath.Join(home, ".local", "state", "llmctl", "llama-server.pid")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("pid file not found at %s: %v", expected, err)
	}

	if err := server.RemovePIDFileInternal(); err != nil {
		t.Fatalf("RemovePIDFileInternal: %v", err)
	}

	if _, err := os.Stat(expected); !os.IsNotExist(err) {
		t.Error("pid file should have been removed")
	}
}

func TestRemovePIDFileInternal_NoFile_NoError(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	t.Setenv("HOME", t.TempDir())

	if err := server.RemovePIDFileInternal(); err != nil {
		t.Errorf("RemovePIDFileInternal() on absent file = %v, want nil", err)
	}
}

// ----------------------------------------------------------------------------
// swapConfigPath
// ----------------------------------------------------------------------------

func TestSwapConfigPath_ContainsLlamaSwap(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := server.SwapConfigPath()
	if err != nil {
		t.Fatalf("SwapConfigPath() error = %v", err)
	}
	if !strings.HasSuffix(path, "llama-swap.yaml") {
		t.Errorf("SwapConfigPath() = %q, want suffix llama-swap.yaml", path)
	}
	if !strings.HasPrefix(path, home) {
		t.Errorf("SwapConfigPath() = %q, want prefix under HOME=%s", path, home)
	}
}

// ----------------------------------------------------------------------------
// stopProcess with live PID
// ----------------------------------------------------------------------------

func TestStopProcess_NoSuchPID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write a PID that is almost certainly not alive (very high PID).
	pidPath := filepath.Join(dir, "test.pid")
	if err := server.WritePIDFileAt(pidPath, 2147483647); err != nil {
		t.Fatalf("WritePIDFileAt: %v", err)
	}

	pathFn := func() (string, error) { return pidPath, nil }
	err := server.StopProcess(pathFn)
	// The PID is not alive so SIGTERM will fail — we expect an error here.
	// The key assertion is that stopProcess does not panic.
	if err == nil {
		t.Log("stopProcess returned nil (process somehow found) — acceptable on some platforms")
	}
}

// ----------------------------------------------------------------------------
// Router.Serve — start/stop via context cancellation
// ----------------------------------------------------------------------------

func TestRouter_Serve_StartsAndStops(t *testing.T) {
	t.Parallel()

	// Find a free port by opening a listener, then closing it.
	l, err := freePort()
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = l

	r := server.NewRouter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.Serve(ctx)
	}()

	// Wait for the server to start.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", l))
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Cancel the context to trigger shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve() returned error after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Serve() did not return within 5s after cancel")
	}
}

// ----------------------------------------------------------------------------
// stopProcess with a real process
// ----------------------------------------------------------------------------

func TestStopProcess_LiveProcess(t *testing.T) {
	t.Parallel()

	// Start a real process that sleeps; we will then stop it.
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep process: %v", err)
	}

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sleep.pid")
	if err := server.WritePIDFileAt(pidPath, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("WritePIDFileAt: %v", err)
	}

	pathFn := func() (string, error) { return pidPath, nil }
	if err := server.StopProcess(pathFn); err != nil {
		t.Errorf("StopProcess() returned unexpected error: %v", err)
	}

	// The PID file should be gone.
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("pid file should be removed after StopProcess")
	}
}

// freePort finds an available TCP port by binding to :0.
func freePort() (int, error) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	// addr is "127.0.0.1:PORT"
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected addr %s", addr)
	}
	var port int
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &port); err != nil {
		return 0, err
	}
	return port, nil
}

// ----------------------------------------------------------------------------
// parallel manager Status with two live PIDs
// ----------------------------------------------------------------------------

func TestParallelManager_Status_WithLivePIDs(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write PID files for both processes using our own PID.
	for i := 0; i < 2; i++ {
		path := filepath.Join(home, ".local", "state", "llmctl",
			fmt.Sprintf("llama-server-%d.pid", i))
		if err := server.WritePIDFileAt(path, os.Getpid()); err != nil {
			t.Fatalf("WritePIDFileAt(%d): %v", i, err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.Mode = "parallel"
	cfg.Models = []config.ModelRef{
		{Alias: "m0", Path: "/m0.gguf", Role: "reason"},
		{Alias: "m1", Path: "/m1.gguf", Role: "code"},
	}

	mgr := server.NewManager(cfg)
	st, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !st.Running {
		t.Error("Status().Running = false, want true when both PIDs alive")
	}
	if st.Mode != "parallel" {
		t.Errorf("Status().Mode = %q, want parallel", st.Mode)
	}
}

// ----------------------------------------------------------------------------
// swap manager Status with live PID
// ----------------------------------------------------------------------------

func TestSwapManager_Status_WithLivePID(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)

	pidPath := filepath.Join(home, ".local", "state", "llmctl", "llama-swap.pid")
	if err := server.WritePIDFileAt(pidPath, os.Getpid()); err != nil {
		t.Fatalf("WritePIDFileAt: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Mode = "cold-swap"

	mgr := server.NewManager(cfg)
	st, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !st.Running {
		t.Error("Status().Running = false, want true when process alive")
	}
}
