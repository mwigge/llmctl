package bundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mwigge/llmctl/internal/model"
)

// writeFakeGGUF writes a tiny placeholder .gguf file at path.
func writeFakeGGUF(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("GGUF-placeholder"), 0o600); err != nil {
		t.Fatalf("write gguf: %v", err)
	}
}

// tarEntryNames opens a .tar.gz archive and returns all entry names.
func tarEntryNames(t *testing.T, archivePath string) []string {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

// readManifestFromTar reads and parses bundle-manifest.json from the archive.
func readManifestFromTar(t *testing.T, archivePath string) BundleManifest {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if filepath.Base(hdr.Name) == "bundle-manifest.json" {
			var m BundleManifest
			if err := json.NewDecoder(tr).Decode(&m); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
			return m
		}
	}
	t.Fatal("bundle-manifest.json not found in archive")
	return BundleManifest{}
}

func TestCreate_WritesManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create a fake model file.
	modelPath := filepath.Join(dir, "models", "my-model-Q4_K_M.gguf")
	writeFakeGGUF(t, modelPath)

	// Set up a registry with one model.
	registryPath := filepath.Join(dir, "registry.json")
	reg, err := model.NewRegistry(registryPath)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Add(model.InstalledModel{
		Alias:       "my-model",
		Repo:        "owner/my-model-GGUF",
		Quant:       "Q4_K_M",
		Path:        modelPath,
		SizeBytes:   1024,
		InstalledAt: time.Now(),
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	outputPath := filepath.Join(dir, "bundle.tar.gz")
	opts := BundleOptions{
		ModelAliases:  []string{"my-model"},
		OutputPath:    outputPath,
		IncludeBinary: false,
		RegistryPath:  registryPath,
	}

	if err := Create(context.Background(), opts); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify the output file exists.
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("output file not created: %v", err)
	}

	// Verify manifest exists inside the archive.
	names := tarEntryNames(t, outputPath)
	foundManifest := false
	for _, n := range names {
		if filepath.Base(n) == "bundle-manifest.json" {
			foundManifest = true
			break
		}
	}
	if !foundManifest {
		t.Errorf("bundle-manifest.json not found in archive; entries: %v", names)
	}

	// Verify manifest content.
	manifest := readManifestFromTar(t, outputPath)
	if len(manifest.Models) != 1 {
		t.Errorf("manifest.Models len = %d, want 1", len(manifest.Models))
	}
	if manifest.Models[0].Alias != "my-model" {
		t.Errorf("manifest.Models[0].Alias = %q, want %q", manifest.Models[0].Alias, "my-model")
	}
	if manifest.Version == "" {
		t.Error("manifest.Version is empty")
	}
}

func TestCreate_IncludesBinaryWhenRequested(t *testing.T) {
	// Cannot run in parallel: uses t.Setenv.
	dir := t.TempDir()

	modelPath := filepath.Join(dir, "models", "test-Q4_K_M.gguf")
	writeFakeGGUF(t, modelPath)

	registryPath := filepath.Join(dir, "registry.json")
	reg, err := model.NewRegistry(registryPath)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Add(model.InstalledModel{
		Alias: "test", Repo: "owner/test", Quant: "Q4_K_M", Path: modelPath,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create a fake "llmctl" binary in a temp dir and put it in PATH.
	fakeBinDir := t.TempDir()
	fakeBinPath := filepath.Join(fakeBinDir, "llmctl")
	if err := os.WriteFile(fakeBinPath, []byte("#!/bin/sh\necho fake"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	t.Setenv("PATH", fakeBinDir+":"+os.Getenv("PATH"))

	outputPath := filepath.Join(dir, "bundle.tar.gz")
	opts := BundleOptions{
		ModelAliases:  []string{"test"},
		OutputPath:    outputPath,
		IncludeBinary: true,
		RegistryPath:  registryPath,
	}

	if err := Create(context.Background(), opts); err != nil {
		t.Fatalf("Create: %v", err)
	}

	names := tarEntryNames(t, outputPath)
	foundBin := false
	for _, n := range names {
		if filepath.Base(n) == "llmctl" {
			foundBin = true
			break
		}
	}
	if !foundBin {
		t.Errorf("llmctl binary not found in archive; entries: %v", names)
	}
}

func TestCreate_ErrorOnUnknownAlias(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	registryPath := filepath.Join(dir, "registry.json")
	reg, err := model.NewRegistry(registryPath)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	opts := BundleOptions{
		ModelAliases:  []string{"does-not-exist"},
		OutputPath:    filepath.Join(dir, "bundle.tar.gz"),
		IncludeBinary: false,
		RegistryPath:  registryPath,
	}

	if err := Create(context.Background(), opts); err == nil {
		t.Fatal("expected error for unknown model alias, got nil")
	}
}

func TestInstall_RegistersModels(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Build a minimal bundle: a tarball containing a model file and manifest.
	modelContent := []byte("GGUF-data")
	modelFileName := "test-model-Q4_K_M.gguf"

	manifest := BundleManifest{
		Version:   "test",
		CreatedAt: time.Now(),
		Models: []ManifestModel{
			{
				Alias:    "test-model",
				Repo:     "owner/test-model-GGUF",
				Quant:    "Q4_K_M",
				FileName: modelFileName,
				SizeBytes: int64(len(modelContent)),
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	// Write a .tar.gz into dir.
	tarPath := filepath.Join(dir, "test.tar.gz")
	func() {
		f, err := os.Create(tarPath)
		if err != nil {
			t.Fatalf("create tar: %v", err)
		}
		defer f.Close()

		gz := gzip.NewWriter(f)
		defer gz.Close()

		tw := tar.NewWriter(gz)
		defer tw.Close()

		// Write manifest.
		writeEntry := func(name string, data []byte) {
			hdr := &tar.Header{
				Name:    "bundle/" + name,
				Mode:    0o644,
				Size:    int64(len(data)),
				ModTime: time.Now(),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatalf("write tar header: %v", err)
			}
			if _, err := tw.Write(data); err != nil {
				t.Fatalf("write tar data: %v", err)
			}
		}

		writeEntry("bundle-manifest.json", manifestJSON)
		writeEntry(modelFileName, modelContent)
	}()

	// Set up destination dirs.
	destDir := filepath.Join(dir, "dest")
	registryPath := filepath.Join(dir, "registry.json")
	reg, err := model.NewRegistry(registryPath)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Install(context.Background(), tarPath, destDir, registryPath); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Reload registry and verify model was registered.
	reg2, err := model.NewRegistry(registryPath)
	if err != nil {
		t.Fatalf("NewRegistry (reload): %v", err)
	}
	installed, ok := reg2.Get("test-model")
	if !ok {
		t.Fatal("model not registered after Install")
	}
	if installed.Alias != "test-model" {
		t.Errorf("installed.Alias = %q, want %q", installed.Alias, "test-model")
	}
	if installed.Repo != "owner/test-model-GGUF" {
		t.Errorf("installed.Repo = %q, want %q", installed.Repo, "owner/test-model-GGUF")
	}
}
