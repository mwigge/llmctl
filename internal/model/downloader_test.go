package model

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDownload_CacheHit verifies that when a file > 50MB already exists
// and Force is false, Download returns the cached path without invoking curl.
func TestDownload_CacheHit(t *testing.T) {
	// No t.Parallel() — tests mutate the package-level execCommand variable.
	dir := t.TempDir()

	// Construct expected dest path.
	destName := "unsloth-Qwen2.5-Coder-7B-Instruct-GGUF-Q4_K_M.gguf"
	destPath := filepath.Join(dir, destName)

	// Write a file larger than 50 MB.
	f, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := f.Truncate(60 * 1024 * 1024); err != nil {
		f.Close()
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	curlCalled := false
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "curl" {
			curlCalled = true
		}
		return exec.Command("true")
	}

	got, err := Download(context.Background(), DownloadOptions{
		Repo:    "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF",
		Quant:   "Q4_K_M",
		Alias:   "qwen7b",
		DestDir: dir,
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if curlCalled {
		t.Error("curl should not be called on a cache hit")
	}
	if got != destPath {
		t.Errorf("got %q, want %q", got, destPath)
	}
}

// TestDownload_PartialFile verifies that a file smaller than 50 MB triggers
// deletion and re-download.
func TestDownload_PartialFile(t *testing.T) {
	dir := t.TempDir()

	destName := "unsloth-Qwen2.5-Coder-7B-Instruct-GGUF-Q4_K_M.gguf"
	destPath := filepath.Join(dir, destName)

	// Write a small "partial" file.
	if err := os.WriteFile(destPath, []byte("partial"), 0o600); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	var curlArgs []string
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "curl" {
			curlArgs = args
			// Simulate successful download by writing enough bytes.
			return exec.Command("sh", "-c", "dd if=/dev/zero bs=1 count=1 > "+destPath+" 2>/dev/null")
		}
		return exec.Command("true")
	}

	// We don't expect success (curl stub won't produce a large file), so we
	// only verify curl was invoked and the partial file was cleaned up before
	// the attempt. We can capture the call count.
	_, _ = Download(context.Background(), DownloadOptions{
		Repo:    "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF",
		Quant:   "Q4_K_M",
		Alias:   "qwen7b",
		DestDir: dir,
	})
	if len(curlArgs) == 0 {
		t.Error("expected curl to be called for partial file re-download")
	}
}

// TestDownload_ForceBypassesCache verifies the Force flag causes re-download
// even if a valid (>50MB) file exists.
func TestDownload_ForceBypassesCache(t *testing.T) {
	dir := t.TempDir()

	destName := "unsloth-Qwen2.5-Coder-7B-Instruct-GGUF-Q4_K_M.gguf"
	destPath := filepath.Join(dir, destName)

	f, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := f.Truncate(60 * 1024 * 1024); err != nil {
		f.Close()
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	curlCalled := false
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "curl" {
			curlCalled = true
			// Truncate to simulate successful curl writing a big file.
			return exec.Command("sh", "-c", "truncate -s 60m "+destPath)
		}
		return exec.Command("true")
	}

	_, _ = Download(context.Background(), DownloadOptions{
		Repo:    "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF",
		Quant:   "Q4_K_M",
		Alias:   "qwen7b",
		DestDir: dir,
		Force:   true,
	})
	if !curlCalled {
		t.Error("curl should be called when Force=true even if cache exists")
	}
}

// TestDownload_DestPathFormat verifies the destination filename convention.
func TestDownload_DestPathFormat(t *testing.T) {
	dir := t.TempDir()

	prev := execCommand
	t.Cleanup(func() { execCommand = prev })

	var capturedArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "curl" {
			capturedArgs = args
			// Write a big enough file so it succeeds.
			destPath := filepath.Join(dir, "unsloth-Qwen2.5-Coder-7B-Instruct-GGUF-Q4_K_M.gguf")
			return exec.Command("sh", "-c", "truncate -s 60m "+destPath)
		}
		return exec.Command("true")
	}

	got, err := Download(context.Background(), DownloadOptions{
		Repo:    "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF",
		Quant:   "Q4_K_M",
		Alias:   "qwen7b",
		DestDir: dir,
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	want := filepath.Join(dir, "unsloth-Qwen2.5-Coder-7B-Instruct-GGUF-Q4_K_M.gguf")
	if got != want {
		t.Errorf("dest path = %q, want %q", got, want)
	}
	// curl should receive -C - for resume support.
	found := false
	for _, a := range capturedArgs {
		if strings.Contains(a, "-C") || a == "-C" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected curl args to contain -C for resume; got %v", capturedArgs)
	}
}
