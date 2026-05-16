package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/model"
	"github.com/mwigge/llmctl/internal/server"
)

type serverInstallOptions struct {
	ConfigPath string
	GPU        bool
	DryRun     bool
	Accel      string
}

func installLocalServer(cmd *cobra.Command, opts serverInstallOptions) error {
	out := cmd.OutOrStdout()
	errw := cmd.ErrOrStderr()
	cfg := config.DefaultConfig()
	if loaded, err := config.Load(opts.ConfigPath); err == nil {
		cfg = loaded
	}

	selected := catalogByName("Qwen3-8B")
	accel := "cpu"
	if opts.GPU {
		gpu, err := detectBestLocalGPU()
		if err != nil {
			return err
		}
		pick, err := selectGPUCatalogModel(gpu.VRAMGB)
		if err != nil {
			return err
		}
		selected = pick
		resolvedAccel, err := gpu.LlamaAccel(opts.Accel)
		if err != nil {
			return err
		}
		accel = resolvedAccel
		fmt.Fprintf(out, "GPU: %s (%s, %.1fGB VRAM)\n", gpu.Name, gpu.Vendor, gpu.VRAMGB)
	}
	fmt.Fprintf(out, "Model: %s (%s %s, %s)\n", selected.Name, selected.Repo, selected.Quant, selected.SizeGB)
	fmt.Fprintf(out, "Accel: %s\n", accel)
	if cfg.Server.CtxSize <= 0 || opts.GPU {
		cfg.Server.CtxSize = 8192
	}
	fmt.Fprintf(out, "Context: %d tokens\n", cfg.Server.CtxSize)
	if opts.DryRun {
		return nil
	}

	if err := ensureLlamaServerInstalled(out, errw); err != nil {
		return err
	}

	modelDir, err := localModelDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}
	fmt.Fprintf(out, "==> Downloading %s (%s) -> %s\n", selected.Repo, selected.Quant, modelDir)
	modelPath, err := model.Download(context.Background(), model.DownloadOptions{
		Repo:     selected.Repo,
		Quant:    selected.Quant,
		Alias:    strings.ToLower(selected.Name),
		DestDir:  modelDir,
		HFToken:  os.Getenv("HF_TOKEN"),
		Progress: errw,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[ok] model cached at %s\n", modelPath)

	cfg.Mode = "single"
	cfg.Server.Host = defaultString(cfg.Server.Host, "127.0.0.1")
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8765
	}
	if cfg.Server.Threads == 0 {
		cfg.Server.Threads = 4
	}
	if opts.GPU {
		cfg.Server.GPULayers = 99
	} else {
		cfg.Server.GPULayers = 0
	}
	cfg.Models = []config.ModelRef{{
		Alias: strings.ToLower(selected.Name),
		Path:  modelPath,
		Role:  "code",
	}}
	if err := config.Save(cfg, opts.ConfigPath); err != nil {
		return err
	}
	fmt.Fprintf(out, "[ok] wrote %s\n", opts.ConfigPath)

	if path, err := server.WriteServiceFile(cfg); err == nil {
		fmt.Fprintf(out, "[ok] wrote systemd unit: %s\n", path)
	}

	fmt.Fprintln(out, "==> Waiting for services to finish installation...")
	if runtime.GOOS == "linux" {
		_ = runInstallCtl(out, errw, "systemctl", "--user", "daemon-reload")
		_ = runInstallCtl(out, errw, "systemctl", "--user", "enable", "--now", server.ServiceName())
		_ = runInstallCtl(out, errw, "systemctl", "--user", "restart", server.ServiceName())
	} else {
		mgr := server.NewManager(cfg)
		_ = mgr.Restart(context.Background())
	}
	if err := waitLocalServerReady(cfg, 90*time.Second); err != nil {
		return err
	}
	fmt.Fprintf(out, "[ok] local server ready at http://%s:%d/v1\n", cfg.Server.Host, cfg.Server.Port)
	return nil
}

func ensureLlamaServerInstalled(out, errw io.Writer) error {
	if p, err := server.FindLlamaServer(); err == nil && binaryStarts(p) {
		fmt.Fprintf(out, "[ok] llama-server installed: %s\n", p)
		return nil
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("llama-server not found; install llama.cpp first or put llama-server on PATH")
	}
	binDir, libDir, err := localInstallDirs()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "llmctl-llama-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	llamaTag := latestLlamaTag()
	if llamaTag == "" {
		llamaTag = "b5576"
	}
	tarPath := filepath.Join(tmp, "llama.tar.gz")
	url := "https://github.com/ggml-org/llama.cpp/releases/download/" + llamaTag + "/llama-" + llamaTag + "-bin-ubuntu-x64.tar.gz"
	fmt.Fprintf(out, "==> Downloading llama-server %s\n", llamaTag)
	if err := runInstallCtl(out, errw, "curl", "-fL", "-o", tarPath, url); err != nil {
		return err
	}
	if err := runInstallCtl(out, errw, "tar", "-xzf", tarPath, "-C", tmp); err != nil {
		return err
	}
	realBin := ""
	_ = filepath.WalkDir(tmp, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(path) == "llama-server" && realBin == "" {
			realBin = path
		}
		if err == nil && !d.IsDir() && (strings.HasSuffix(path, ".so") || strings.Contains(filepath.Base(path), ".so.")) {
			_ = copyFile(path, filepath.Join(libDir, filepath.Base(path)), 0o755)
		}
		return nil
	})
	if realBin == "" {
		return errors.New("downloaded llama.cpp archive did not contain llama-server")
	}
	if err := copyFile(realBin, filepath.Join(libDir, "llama-server"), 0o755); err != nil {
		return err
	}
	wrapper := fmt.Sprintf("#!/usr/bin/env bash\nexport LD_LIBRARY_PATH=%q${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}\ncd %q\nexec %q \"$@\"\n", libDir, libDir, filepath.Join(libDir, "llama-server"))
	if err := os.WriteFile(filepath.Join(binDir, "llama-server"), []byte(wrapper), 0o755); err != nil {
		return err
	}
	fmt.Fprintf(out, "[ok] llama-server installed: %s\n", filepath.Join(binDir, "llama-server"))
	return nil
}

func latestLlamaTag() string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/ggml-org/llama.cpp/releases/latest", nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	text := string(data)
	marker := `"tag_name"`
	i := strings.Index(text, marker)
	if i < 0 {
		return ""
	}
	rest := text[i+len(marker):]
	q1 := strings.Index(rest, `"`)
	if q1 < 0 {
		return ""
	}
	rest = rest[q1+1:]
	q2 := strings.Index(rest, `"`)
	if q2 < 0 {
		return ""
	}
	return rest[:q2]
}

func binaryStarts(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--help")
	return cmd.Run() == nil
}

func runInstallCtl(out, errw io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = out
	cmd.Stderr = errw
	return cmd.Run()
}

func waitLocalServerReady(cfg *config.Config, timeout time.Duration) error {
	url := fmt.Sprintf("http://%s:%d/v1/models", cfg.Server.Host, cfg.Server.Port)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				cancel()
				return nil
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		cancel()
		time.Sleep(time.Second)
	}
	return fmt.Errorf("local server did not become ready: %w", lastErr)
}

func localInstallDirs() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, ".local", "bin"), filepath.Join(home, ".local", "lib", "llmctl"), nil
}

func localModelDir() (string, error) {
	if v := os.Getenv("MODEL_DIR"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "llmctl", "models"), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func catalogByName(name string) model.CatalogEntry {
	for _, entry := range model.BuiltinCatalog {
		if strings.EqualFold(entry.Name, name) {
			return entry
		}
	}
	return model.BuiltinCatalog[0]
}

type localGPUInfo struct {
	Vendor string
	Name   string
	VRAMGB float64
}

func (g localGPUInfo) LlamaAccel(override string) (string, error) {
	override = strings.ToLower(strings.TrimSpace(override))
	if override != "" && override != "auto" {
		switch override {
		case "vulkan", "cuda", "hip", "rocm":
			if override == "rocm" {
				return "hip", nil
			}
			return override, nil
		default:
			return "", fmt.Errorf("unsupported --accel %q (supported: auto, vulkan, cuda, hip)", override)
		}
	}
	switch g.Vendor {
	case "nvidia":
		if commandExists("nvcc") {
			return "cuda", nil
		}
		return "vulkan", nil
	case "amd":
		if commandExists("hipcc") || commandExists("amdclang++") {
			return "hip", nil
		}
		return "vulkan", nil
	case "apple":
		return "metal", nil
	default:
		return "vulkan", nil
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func selectGPUCatalogModel(vramGB float64) (model.CatalogEntry, error) {
	if vramGB <= 0 {
		return model.CatalogEntry{}, errors.New("GPU VRAM is unknown")
	}
	budget := vramGB * 0.45
	var best model.CatalogEntry
	for _, entry := range model.BuiltinCatalog {
		size := parseSizeGB(entry.SizeGB)
		if size <= 0 || size > budget {
			continue
		}
		if best.Name == "" || size > parseSizeGB(best.SizeGB) {
			best = entry
		}
	}
	if best.Name == "" {
		return model.CatalogEntry{}, fmt.Errorf("no curated GGUF fits %.1fGB VRAM with safety headroom", vramGB)
	}
	return best, nil
}

func parseSizeGB(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(strings.ToUpper(s)), "GB"), 64)
	return v
}

func detectBestLocalGPU() (localGPUInfo, error) {
	var candidates []localGPUInfo
	if gpu, ok := detectNVIDIA(); ok {
		candidates = append(candidates, gpu)
	}
	if gpu, ok := detectAMD(); ok {
		candidates = append(candidates, gpu)
	}
	if gpu, ok := detectDarwinGPU(); ok {
		candidates = append(candidates, gpu)
	}
	if len(candidates) == 0 {
		return localGPUInfo{}, errors.New("no NVIDIA, AMD, or macOS GPU with memory detected")
	}
	best := candidates[0]
	for _, gpu := range candidates[1:] {
		if gpu.VRAMGB > best.VRAMGB {
			best = gpu
		}
	}
	return best, nil
}

func detectNVIDIA() (localGPUInfo, bool) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return localGPUInfo{}, false
	}
	var best localGPUInfo
	for _, line := range strings.Split(string(out), "\n") {
		name, mem, ok := strings.Cut(strings.TrimSpace(line), ",")
		if !ok {
			continue
		}
		mb, err := strconv.ParseFloat(strings.TrimSpace(mem), 64)
		if err != nil || mb <= 0 {
			continue
		}
		gpu := localGPUInfo{Vendor: "nvidia", Name: strings.TrimSpace(name), VRAMGB: mb / 1024}
		if best.Name == "" || gpu.VRAMGB > best.VRAMGB {
			best = gpu
		}
	}
	return best, best.Name != ""
}

func detectAMD() (localGPUInfo, bool) {
	entries, err := os.ReadDir("/sys/class/drm")
	if err == nil {
		var best localGPUInfo
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "card") || strings.Contains(entry.Name(), "-") {
				continue
			}
			deviceDir := filepath.Join("/sys/class/drm", entry.Name(), "device")
			vendor, err := os.ReadFile(filepath.Join(deviceDir, "vendor"))
			if err != nil || strings.TrimSpace(string(vendor)) != "0x1002" {
				continue
			}
			vramRaw, err := os.ReadFile(filepath.Join(deviceDir, "mem_info_vram_total"))
			if err != nil {
				continue
			}
			vram, err := strconv.ParseFloat(strings.TrimSpace(string(vramRaw)), 64)
			if err != nil || vram <= 0 {
				continue
			}
			gpu := localGPUInfo{Vendor: "amd", Name: entry.Name(), VRAMGB: vram / (1024 * 1024 * 1024)}
			if best.Name == "" || gpu.VRAMGB > best.VRAMGB {
				best = gpu
			}
		}
		if best.Name != "" {
			return best, true
		}
	}
	out, err := exec.Command("rocm-smi", "--showproductname", "--showmeminfo", "vram").CombinedOutput()
	if err != nil {
		return localGPUInfo{}, false
	}
	name := "AMD GPU"
	var best float64
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "product name") || strings.Contains(lower, "card series") {
			if _, value, ok := strings.Cut(line, ":"); ok {
				name = strings.TrimSpace(value)
			}
		}
		if strings.Contains(lower, "vram") {
			if gb := firstMemoryGB(line); gb > best {
				best = gb
			}
		}
	}
	if best <= 0 {
		return localGPUInfo{}, false
	}
	return localGPUInfo{Vendor: "amd", Name: name, VRAMGB: best}, true
}

func detectDarwinGPU() (localGPUInfo, bool) {
	if runtime.GOOS != "darwin" {
		return localGPUInfo{}, false
	}
	name := "Apple GPU"
	if out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(strings.ToLower(line), "chipset model:") {
				_, value, _ := strings.Cut(line, ":")
				name = strings.TrimSpace(value)
				break
			}
		}
	}
	if runtime.GOARCH == "arm64" {
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			if bytes, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); err == nil {
				gb := bytes/(1024*1024*1024) - 4
				if gb < 1 {
					gb = 1
				}
				return localGPUInfo{Vendor: "apple", Name: name, VRAMGB: gb}, true
			}
		}
	}
	return localGPUInfo{}, false
}

func firstMemoryGB(s string) float64 {
	fields := strings.Fields(strings.NewReplacer(":", " ", ",", " ", "(", " ", ")", " ").Replace(s))
	for i, field := range fields {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			continue
		}
		if i+1 >= len(fields) {
			continue
		}
		unit := strings.ToLower(fields[i+1])
		switch {
		case strings.HasPrefix(unit, "gib"), strings.HasPrefix(unit, "gb"):
			return value
		case strings.HasPrefix(unit, "mib"), strings.HasPrefix(unit, "mb"):
			return value / 1024
		}
	}
	return 0
}
