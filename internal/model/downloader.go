package model

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const partialThresholdBytes = 50 * 1024 * 1024 // 50 MB

// execCommand is the exec.Command function used by Download. It is a
// package-level variable so tests can substitute a stub.
var execCommand = exec.Command

// DownloadOptions controls the behaviour of Download.
type DownloadOptions struct {
	Repo     string
	Quant    string
	Alias    string
	DestDir  string
	HFToken  string
	Force    bool
	Progress io.Writer
}

// Download fetches a GGUF file from HuggingFace (or mirrors) into DestDir.
// It returns the absolute path of the downloaded file.
//
// Cache semantics:
//   - If the file exists and is > 50 MB and Force is false, the cached path
//     is returned immediately.
//   - If the file exists but is ≤ 50 MB it is treated as a partial download,
//     deleted, and re-downloaded.
//   - If Force is true any existing file is removed before re-downloading.
func Download(ctx context.Context, opts DownloadOptions) (string, error) {
	// Derive filename: replace "/" with "-" in the repo base, then append quant.
	// e.g. "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF" -> "unsloth-Qwen2.5-Coder-7B-Instruct-GGUF"
	repoFlat := strings.ReplaceAll(opts.Repo, "/", "-")
	filename := repoFlat + "-" + opts.Quant + ".gguf"
	destPath := filepath.Join(opts.DestDir, filename)

	// Check for existing file.
	if !opts.Force {
		info, err := os.Stat(destPath)
		if err == nil {
			if info.Size() > partialThresholdBytes {
				// Cache hit.
				return destPath, nil
			}
			// Partial file — warn and remove.
			slog.Warn("removing partial download", "path", destPath, "size", info.Size())
			if removeErr := os.Remove(destPath); removeErr != nil {
				return "", fmt.Errorf("remove partial file %s: %w", destPath, removeErr)
			}
		}
	} else {
		// Force re-download: remove any existing file.
		if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove existing file %s: %w", destPath, err)
		}
	}

	base := strings.TrimSuffix(filepath.Base(opts.Repo), "-GGUF")
	ggufFile := base + "-" + opts.Quant + ".gguf"

	// Mirror list, tried in order.
	mirrors := []string{
		"https://huggingface.co/" + opts.Repo + "/resolve/main/" + ggufFile,
		"https://hf-mirror.com/" + opts.Repo + "/resolve/main/" + ggufFile,
		"https://modelscope.cn/api/v1/models/" + opts.Repo + "/repo?Revision=master&FilePath=" + ggufFile,
	}

	var lastErr error
	for _, url := range mirrors {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("download cancelled: %w", err)
		}
		if err := runCurl(ctx, url, destPath, opts.HFToken, opts.Progress); err != nil {
			lastErr = err
			slog.Warn("mirror failed, trying next", "url", url, "err", err)
			continue
		}
		return destPath, nil
	}
	return "", fmt.Errorf("all mirrors failed for %s %s: %w", opts.Repo, opts.Quant, lastErr)
}

// runCurl executes curl with resume-support to download url into dest.
func runCurl(ctx context.Context, url, dest, hfToken string, progress io.Writer) error {
	args := []string{"-fL", "-C", "-", "-o", dest, url}
	if hfToken != "" {
		args = append([]string{"-H", "Authorization: Bearer " + hfToken}, args...)
	}

	cmd := execCommand("curl", args...)
	cmd.Args[0] = "curl" // ensure Args[0] matches the binary name

	if progress != nil {
		cmd.Stderr = progress
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("curl %s: %w", url, err)
	}
	return nil
}
