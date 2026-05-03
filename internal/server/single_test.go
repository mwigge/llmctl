package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/server"
)

// ----------------------------------------------------------------------------
// FindLlamaServer
// ----------------------------------------------------------------------------

func TestFindLlamaServer_NotFound_EmptyPATH(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.

	// Clear PATH so neither well-known dirs on this machine nor PATH lookup
	// can succeed. Note: well-known dirs (/usr/bin, etc.) may still have the
	// binary on some machines; we only control PATH here. The test is most
	// reliable on CI where the binary is absent.
	t.Setenv("PATH", "")

	// We can only assert error when the binary is genuinely absent.
	// If the binary exists in a well-known dir, skip rather than fail.
	_, err := server.FindLlamaServer()
	if err == nil {
		t.Skip("llama-server found in well-known dir; skipping absence test")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("FindLlamaServer() error = %q, want message containing 'not found'", err.Error())
	}
}

// ----------------------------------------------------------------------------
// buildSwapConfig (exported via a thin wrapper in the test package)
// ----------------------------------------------------------------------------

func TestBuildSwapConfig_ColdTTL(t *testing.T) {
	t.Parallel()

	models := []config.ModelRef{
		{Alias: "reason-7b", Path: "/models/reason.gguf", Role: "reason"},
		{Alias: "code-13b", Path: "/models/code.gguf", Role: "code"},
	}

	data, err := server.BuildSwapConfig(models, false, 600, 8765, 16384)
	if err != nil {
		t.Fatalf("BuildSwapConfig() error = %v", err)
	}

	yaml := string(data)

	tests := []struct {
		name    string
		contain string
	}{
		{"reason alias present", "reason-7b"},
		{"code alias present", "code-13b"},
		{"reason model path", "/models/reason.gguf"},
		{"code model path", "/models/code.gguf"},
		{"cold TTL value", "ttl: 600"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(yaml, tt.contain) {
				t.Errorf("BuildSwapConfig() output missing %q\ngot:\n%s", tt.contain, yaml)
			}
		})
	}
}

func TestBuildSwapConfig_HotTTLIsZero(t *testing.T) {
	t.Parallel()

	models := []config.ModelRef{
		{Alias: "fast", Path: "/models/fast.gguf"},
	}

	data, err := server.BuildSwapConfig(models, true, 0, 8765, 16384)
	if err != nil {
		t.Fatalf("BuildSwapConfig() error = %v", err)
	}

	yaml := string(data)
	if !strings.Contains(yaml, "ttl: 0") {
		t.Errorf("BuildSwapConfig(hot=true) expected ttl: 0, got:\n%s", yaml)
	}
	// Must not contain a non-zero TTL.
	if strings.Contains(yaml, "ttl: 600") {
		t.Errorf("BuildSwapConfig(hot=true) must not contain ttl: 600, got:\n%s", yaml)
	}
}

// ----------------------------------------------------------------------------
// Status with no PID file
// ----------------------------------------------------------------------------

func TestSingleManager_Status_NoPIDFile(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	// Use a temp HOME so no PID file exists.
	t.Setenv("HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.Mode = "single"

	mgr := server.NewManager(cfg)
	st, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}
	if st.Running {
		t.Error("Status().Running = true, want false when PID file absent")
	}
}

func TestSwapManager_Status_NoPIDFile_Cold(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	t.Setenv("HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.Mode = "cold-swap"

	mgr := server.NewManager(cfg)
	st, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}
	if st.Running {
		t.Error("Status().Running = true, want false when PID file absent")
	}
	if st.Mode != "cold-swap" {
		t.Errorf("Status().Mode = %q, want %q", st.Mode, "cold-swap")
	}
}

func TestSwapManager_Status_NoPIDFile_Hot(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	t.Setenv("HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.Mode = "hot-swap"

	mgr := server.NewManager(cfg)
	st, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}
	if st.Running {
		t.Error("Status().Running = true, want false when PID file absent")
	}
	if st.Mode != "hot-swap" {
		t.Errorf("Status().Mode = %q, want %q", st.Mode, "hot-swap")
	}
}

func TestParallelManager_Status_NoPIDFile(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	t.Setenv("HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.Mode = "parallel"
	cfg.Models = []config.ModelRef{
		{Alias: "m0", Path: "/models/m0.gguf", Role: "reason"},
		{Alias: "m1", Path: "/models/m1.gguf", Role: "code"},
	}

	mgr := server.NewManager(cfg)
	st, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}
	if st.Running {
		t.Error("Status().Running = true, want false when PID files absent")
	}
	if st.Mode != "parallel" {
		t.Errorf("Status().Mode = %q, want %q", st.Mode, "parallel")
	}
}

// ----------------------------------------------------------------------------
// NewManager dispatch
// ----------------------------------------------------------------------------

func TestNewManager_Dispatch(t *testing.T) {
	// Cannot run in parallel: subtests use t.Setenv.
	tests := []struct {
		mode     string
		wantMode string
	}{
		{"single", "single"},
		{"cold-swap", "cold-swap"},
		{"hot-swap", "hot-swap"},
		{"parallel", "parallel"},
		{"unknown", "single"}, // fallback
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.mode, func(t *testing.T) {
			// Cannot run in parallel: uses t.Setenv.
			t.Setenv("HOME", t.TempDir())

			cfg := config.DefaultConfig()
			cfg.Mode = tt.mode
			cfg.Models = []config.ModelRef{
				{Alias: "m0", Path: "/models/m0.gguf"},
				{Alias: "m1", Path: "/models/m1.gguf"},
			}

			mgr := server.NewManager(cfg)
			st, err := mgr.Status(context.Background())
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if st.Mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", st.Mode, tt.wantMode)
			}
		})
	}
}
