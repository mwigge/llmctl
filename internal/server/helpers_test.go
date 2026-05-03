package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/server"
)

// ----------------------------------------------------------------------------
// FindLlamaSwap
// ----------------------------------------------------------------------------

func TestFindLlamaSwap_NotFound_EmptyPATH(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	t.Setenv("PATH", "")

	_, err := server.FindLlamaSwap()
	if err == nil {
		t.Skip("llama-swap found in well-known dir; skipping absence test")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("FindLlamaSwap() error = %q, want 'not found'", err.Error())
	}
}

func TestFindLlamaServer_Found_InCustomDir(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	binDir := t.TempDir()
	fakeBin := filepath.Join(binDir, "llama-server")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	path, err := server.FindLlamaServer()
	if err != nil {
		t.Fatalf("FindLlamaServer() unexpected error: %v", err)
	}
	if path == "" {
		t.Error("FindLlamaServer() returned empty path")
	}
}

// ----------------------------------------------------------------------------
// PID file helpers
// ----------------------------------------------------------------------------

func TestPIDFileRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := server.WritePIDFileAt(path, 12345); err != nil {
		t.Fatalf("WritePIDFileAt: %v", err)
	}

	pid, err := server.ReadPIDFileAt(path)
	if err != nil {
		t.Fatalf("ReadPIDFileAt: %v", err)
	}
	if pid != 12345 {
		t.Errorf("pid = %d, want 12345", pid)
	}
}

func TestReadPIDFile_NotExist(t *testing.T) {
	t.Parallel()

	_, err := server.ReadPIDFileAt(filepath.Join(t.TempDir(), "noexist.pid"))
	if err == nil {
		t.Error("ReadPIDFileAt() expected error for missing file, got nil")
	}
}

func TestRemovePIDFile_NotExist_NoError(t *testing.T) {
	t.Parallel()

	// Removing a non-existent PID file must not return an error.
	err := server.RemovePIDFileAt(filepath.Join(t.TempDir(), "noexist.pid"))
	if err != nil {
		t.Errorf("RemovePIDFileAt() on absent file = %v, want nil", err)
	}
}

func TestRemovePIDFile_Existing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")
	if err := os.WriteFile(path, []byte("1"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := server.RemovePIDFileAt(path); err != nil {
		t.Fatalf("RemovePIDFileAt() = %v, want nil", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

// ----------------------------------------------------------------------------
// PidAlive
// ----------------------------------------------------------------------------

func TestPIDAlive_CurrentProcess(t *testing.T) {
	t.Parallel()

	if !server.PIDAlive(os.Getpid()) {
		t.Error("PIDAlive(os.Getpid()) = false, want true")
	}
}

func TestPIDAlive_FakePID(t *testing.T) {
	t.Parallel()

	// PID 0 is never a valid process on Linux/macOS.
	if server.PIDAlive(0) {
		t.Error("PIDAlive(0) = true, want false")
	}
}

// ----------------------------------------------------------------------------
// fetchModels via httptest
// ----------------------------------------------------------------------------

func TestFetchModels_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{
				{"id": "model-a"},
				{"id": "model-b"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	models := server.FetchModels(context.Background(), srv.URL)
	if len(models) != 2 {
		t.Fatalf("FetchModels() len = %d, want 2", len(models))
	}
	if models[0] != "model-a" {
		t.Errorf("models[0] = %q, want %q", models[0], "model-a")
	}
}

func TestFetchModels_ServerDown(t *testing.T) {
	t.Parallel()

	// Non-routable address → connection refused → nil return.
	models := server.FetchModels(context.Background(), "http://127.0.0.1:19999")
	if models != nil {
		t.Errorf("FetchModels() on down server = %v, want nil", models)
	}
}

// ----------------------------------------------------------------------------
// buildArgv
// ----------------------------------------------------------------------------

func TestBuildArgv_ContainsEssentialFlags(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Models = []config.ModelRef{
		{Alias: "reason", Path: "/models/r.gguf"},
	}

	args := server.BuildArgv(cfg)

	tests := []struct {
		flag string
		val  string
	}{
		{"-m", "/models/r.gguf"},
		{"--host", cfg.Server.Host},
		{"--port", strconv.Itoa(cfg.Server.Port)},
		{"--ctx-size", strconv.Itoa(cfg.Server.CtxSize)},
		{"--threads", strconv.Itoa(cfg.Server.Threads)},
	}

	argStr := strings.Join(args, " ")
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			t.Parallel()
			needle := tt.flag + " " + tt.val
			if !strings.Contains(argStr, needle) {
				t.Errorf("argv missing %q; got: %s", needle, argStr)
			}
		})
	}
}

func TestBuildArgv_GPULayers(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Server.GPULayers = 32
	cfg.Models = []config.ModelRef{{Path: "/models/m.gguf"}}

	args := server.BuildArgv(cfg)
	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--n-gpu-layers 32") {
		t.Errorf("argv missing --n-gpu-layers 32; got: %s", argStr)
	}
}

func TestBuildArgv_NoModels(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Models = nil

	args := server.BuildArgv(cfg)
	argStr := strings.Join(args, " ")
	// Should not contain -m flag if no models.
	if strings.Contains(argStr, " -m ") {
		t.Errorf("argv contains -m with no models; got: %s", argStr)
	}
}

// ----------------------------------------------------------------------------
// buildParallelArgv
// ----------------------------------------------------------------------------

func TestBuildParallelArgv_PortOffset(t *testing.T) {
	t.Parallel()

	model := config.ModelRef{Alias: "coder", Path: "/models/c.gguf"}
	srv := config.ServerCfg{
		Host:    "127.0.0.1",
		Port:    8765,
		CtxSize: 16384,
		Threads: 4,
	}
	args := server.BuildParallelArgv(model, srv, 8766)
	argStr := strings.Join(args, " ")

	if !strings.Contains(argStr, "--port 8766") {
		t.Errorf("parallel argv missing --port 8766; got: %s", argStr)
	}
	if !strings.Contains(argStr, "--alias coder") {
		t.Errorf("parallel argv missing --alias coder; got: %s", argStr)
	}
}

// ----------------------------------------------------------------------------
// ServiceName
// ----------------------------------------------------------------------------

func TestServiceName(t *testing.T) {
	t.Parallel()

	name := server.ServiceName()
	if name != "llmctl-server.service" {
		t.Errorf("ServiceName() = %q, want %q", name, "llmctl-server.service")
	}
}

// ----------------------------------------------------------------------------
// WriteServiceFile / RemoveServiceFile
// ----------------------------------------------------------------------------

func TestWriteServiceFile_CreatesFile(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := config.DefaultConfig()
	path, err := server.WriteServiceFile(cfg)
	if err != nil {
		t.Fatalf("WriteServiceFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read service file: %v", err)
	}

	content := string(data)
	tests := []struct {
		name    string
		contain string
	}{
		{"unit section", "[Unit]"},
		{"service section", "[Service]"},
		{"install section", "[Install]"},
		{"ExecStart", "ExecStart="},
		{"server start", "server start --foreground"},
		{"restart policy", "Restart=on-failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(content, tt.contain) {
				t.Errorf("service file missing %q\ncontent:\n%s", tt.contain, content)
			}
		})
	}
}

func TestRemoveServiceFile_RemovesFile(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := config.DefaultConfig()
	path, err := server.WriteServiceFile(cfg)
	if err != nil {
		t.Fatalf("WriteServiceFile() error = %v", err)
	}

	if err := server.RemoveServiceFile(); err != nil {
		t.Fatalf("RemoveServiceFile() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("service file should have been removed")
	}
}

func TestRemoveServiceFile_NoFile_NoError(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	t.Setenv("HOME", t.TempDir())

	if err := server.RemoveServiceFile(); err != nil {
		t.Errorf("RemoveServiceFile() on absent file = %v, want nil", err)
	}
}

// ----------------------------------------------------------------------------
// BuildSwapConfig — additional coverage
// ----------------------------------------------------------------------------

func TestBuildSwapConfig_EmptyModels(t *testing.T) {
	t.Parallel()

	data, err := server.BuildSwapConfig(nil, false, 600, 8765, 16384)
	if err != nil {
		t.Fatalf("BuildSwapConfig() with nil models error = %v", err)
	}
	if data == nil {
		t.Error("BuildSwapConfig() returned nil data for empty models")
	}
}

func TestBuildSwapConfig_NoAlias_UsesDefault(t *testing.T) {
	t.Parallel()

	models := []config.ModelRef{
		{Path: "/models/m.gguf"}, // no alias
	}
	data, err := server.BuildSwapConfig(models, false, 600, 8765, 16384)
	if err != nil {
		t.Fatalf("BuildSwapConfig() error = %v", err)
	}

	yaml := string(data)
	if !strings.Contains(yaml, "model-0") {
		t.Errorf("BuildSwapConfig() without alias should use 'model-0', got:\n%s", yaml)
	}
}

func TestBuildSwapConfig_PortEmbeddedInCmd(t *testing.T) {
	t.Parallel()

	models := []config.ModelRef{
		{Alias: "m0", Path: "/models/m0.gguf"},
		{Alias: "m1", Path: "/models/m1.gguf"},
	}
	data, err := server.BuildSwapConfig(models, false, 600, 8765, 16384)
	if err != nil {
		t.Fatalf("BuildSwapConfig() error = %v", err)
	}

	yaml := string(data)
	// model[0] gets port 8765, model[1] gets port 8766
	if !strings.Contains(yaml, "--port 8765") {
		t.Errorf("expected --port 8765 for first model; got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "--port 8766") {
		t.Errorf("expected --port 8766 for second model; got:\n%s", yaml)
	}
}

// ----------------------------------------------------------------------------
// Router passthrough path
// ----------------------------------------------------------------------------

func TestRouter_Passthrough_NonChatRoute(t *testing.T) {
	t.Parallel()

	var hit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = fmt.Fprint(w, `{"object":"list"}`)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerCfg{Host: "127.0.0.1", Port: 9902},
		Models: []config.ModelRef{
			{Alias: "m0", Path: "/m0.gguf", Role: "reason"},
		},
	}
	rt := server.NewRouterForTest(cfg, upstream.URL, upstream.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)

	if !hit {
		t.Error("passthrough request should have reached upstream")
	}
}

func TestRouter_NilBody_FallsBackToReason(t *testing.T) {
	t.Parallel()

	var reasonHitCount, codeHitCount int

	reasonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reasonHitCount++
		_, _ = fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer reasonServer.Close()

	codeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		codeHitCount++
		_, _ = fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer codeServer.Close()

	cfg := &config.Config{
		Server: config.ServerCfg{Host: "127.0.0.1", Port: 9903},
		Models: []config.ModelRef{
			{Alias: "reason", Path: "/m.gguf", Role: "reason"},
			{Alias: "code", Path: "/c.gguf", Role: "code"},
		},
	}
	rt := server.NewRouterForTest(cfg, reasonServer.URL, codeServer.URL)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)

	// nil body has no code keywords → reason upstream (first model).
	if reasonHitCount == 0 {
		t.Errorf("nil-body request should have reached reason upstream; reason hits=%d code hits=%d", reasonHitCount, codeHitCount)
	}
	if codeHitCount != 0 {
		t.Error("nil-body request should not have reached code upstream")
	}
}

// ----------------------------------------------------------------------------
// Helpers exported for tests
// These are not part of the public API — tested via wrapper in export_test.go.
// ----------------------------------------------------------------------------

func TestWritePIDFile_CreatesParentDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "test.pid")

	if err := server.WritePIDFileAt(path, 99); err != nil {
		t.Fatalf("WritePIDFileAt() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("pid file not created: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Router URL helpers
// ----------------------------------------------------------------------------

func TestRouterUpstreamURL_Port(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig() // port=8765
	url := server.RouterUpstreamURL(cfg, 0)
	if !strings.Contains(url, "8766") { // 8765 + 0 + 1
		t.Errorf("RouterUpstreamURL(0) = %q, want port 8766", url)
	}

	url1 := server.RouterUpstreamURL(cfg, 1)
	if !strings.Contains(url1, "8767") { // 8765 + 1 + 1
		t.Errorf("RouterUpstreamURL(1) = %q, want port 8767", url1)
	}
}

func TestRouterCodeModelURL_FindsByRole(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Models = []config.ModelRef{
		{Alias: "reason", Path: "/r.gguf", Role: "reason"},
		{Alias: "code", Path: "/c.gguf", Role: "code"},
	}
	url := server.RouterCodeModelURL(cfg)
	// code model is at index 1 → port 8765+1+1=8767
	if !strings.Contains(url, "8767") {
		t.Errorf("RouterCodeModelURL() = %q, want port 8767", url)
	}
}

func TestRouterReasonModelURL_FindsByRole(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Models = []config.ModelRef{
		{Alias: "reason", Path: "/r.gguf", Role: "reason"},
		{Alias: "code", Path: "/c.gguf", Role: "code"},
	}
	url := server.RouterReasonModelURL(cfg)
	// reason model is at index 0 → port 8765+0+1=8766
	if !strings.Contains(url, "8766") {
		t.Errorf("RouterReasonModelURL() = %q, want port 8766", url)
	}
}

func TestRouterCodeModelURL_FallbackToModel1(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Models = []config.ModelRef{
		{Alias: "m0", Path: "/m0.gguf"}, // no role
		{Alias: "m1", Path: "/m1.gguf"}, // no role
	}
	url := server.RouterCodeModelURL(cfg)
	// no role match → model[1] → port 8765+1+1=8767
	if !strings.Contains(url, "8767") {
		t.Errorf("RouterCodeModelURL() fallback = %q, want port 8767", url)
	}
}

func TestRouterReasonModelURL_FallbackToModel0(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Models = []config.ModelRef{
		{Alias: "m0", Path: "/m0.gguf"}, // no role
	}
	url := server.RouterReasonModelURL(cfg)
	// no role match → model[0] → port 8765+0+1=8766
	if !strings.Contains(url, "8766") {
		t.Errorf("RouterReasonModelURL() fallback = %q, want port 8766", url)
	}
}

// ----------------------------------------------------------------------------
// looksLikeCode
// ----------------------------------------------------------------------------

func TestLooksLikeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		body  string
		want  bool
	}{
		{"code keyword", `write a function`, true},
		{"implement keyword", `implement this`, true},
		{"debug keyword", `debug this error`, true},
		{"upper case", `WRITE A FUNCTION`, true},
		{"no keyword", `explain quantum physics`, false},
		{"empty", ``, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := server.LooksLikeCode(tt.body)
			if got != tt.want {
				t.Errorf("LooksLikeCode(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Status with running PID
// ----------------------------------------------------------------------------

func TestSingleManager_Status_WithLivePID(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write the current process's PID as if it were llama-server.
	pidPath := filepath.Join(home, ".local", "state", "llmctl", "llama-server.pid")
	if err := server.WritePIDFileAt(pidPath, os.Getpid()); err != nil {
		t.Fatalf("WritePIDFileAt: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Mode = "single"

	mgr := server.NewManager(cfg)
	// Status will find the PID alive (our own process) but the HTTP endpoint
	// is not reachable — it should still return Running=true.
	st, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !st.Running {
		t.Error("Status().Running = false, want true when process is alive")
	}
	if st.Mode != "single" {
		t.Errorf("Status().Mode = %q, want single", st.Mode)
	}
}
