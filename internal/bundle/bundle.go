package bundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mwigge/llmctl/internal/model"
)

// BundleManifest describes the contents of a bundle archive.
type BundleManifest struct {
	Version   string          `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	Models    []ManifestModel `json:"models"`
}

// ManifestModel is a single model entry within a BundleManifest.
type ManifestModel struct {
	Alias     string `json:"alias"`
	Repo      string `json:"repo"`
	Quant     string `json:"quant"`
	FileName  string `json:"file_name"`
	SizeBytes int64  `json:"size_bytes"`
}

// BundleOptions configures the Create operation.
type BundleOptions struct {
	ModelAliases  []string
	OutputPath    string
	IncludeBinary bool
	RegistryPath  string
}

// Create builds a .tar.gz bundle containing the requested models (and
// optionally the llmctl binary) at opts.OutputPath.
func Create(_ context.Context, opts BundleOptions) error {
	reg, err := model.NewRegistry(opts.RegistryPath)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}

	var entries []model.InstalledModel
	for _, alias := range opts.ModelAliases {
		m, ok := reg.Get(alias)
		if !ok {
			return fmt.Errorf("model %q not found in registry", alias)
		}
		entries = append(entries, m)
	}

	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	f, err := os.Create(opts.OutputPath)
	if err != nil {
		return fmt.Errorf("create bundle file: %w", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	manifest := BundleManifest{
		Version:   "1",
		CreatedAt: time.Now().UTC(),
		Models:    make([]ManifestModel, 0, len(entries)),
	}

	for _, m := range entries {
		info, err := os.Stat(m.Path)
		if err != nil {
			return fmt.Errorf("stat model %s: %w", m.Alias, err)
		}

		fileName := filepath.Base(m.Path)
		manifest.Models = append(manifest.Models, ManifestModel{
			Alias:     m.Alias,
			Repo:      m.Repo,
			Quant:     m.Quant,
			FileName:  fileName,
			SizeBytes: info.Size(),
		})

		if err := addFileToTar(tw, m.Path, "bundle/"+fileName); err != nil {
			return fmt.Errorf("add model %s to bundle: %w", m.Alias, err)
		}
	}

	if opts.IncludeBinary {
		binPath, err := exec.LookPath("llmctl")
		if err == nil {
			if err := addFileToTar(tw, binPath, "bundle/llmctl"); err != nil {
				return fmt.Errorf("add llmctl binary to bundle: %w", err)
			}
		}
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := addBytesToTar(tw, "bundle/bundle-manifest.json", manifestJSON); err != nil {
		return fmt.Errorf("add manifest to bundle: %w", err)
	}

	return nil
}

// Install extracts a bundle from tarPath into destDir and registers models in the registry.
func Install(_ context.Context, tarPath, destDir, registryPath string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("open bundle: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	// First pass: read and buffer all entries, decode manifest.
	type entry struct {
		name string
		data []byte
	}
	var files []entry
	var manifest BundleManifest

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("read tar entry %s: %w", hdr.Name, err)
		}
		if filepath.Base(hdr.Name) == "bundle-manifest.json" {
			if err := json.Unmarshal(data, &manifest); err != nil {
				return fmt.Errorf("parse manifest: %w", err)
			}
		}
		files = append(files, entry{name: filepath.Base(hdr.Name), data: data})
	}

	if len(manifest.Models) == 0 {
		return fmt.Errorf("bundle contains no models")
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	reg, err := model.NewRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}

	for _, mm := range manifest.Models {
		var modelData []byte
		for _, fe := range files {
			if fe.name == mm.FileName {
				modelData = fe.data
				break
			}
		}
		if modelData == nil {
			return fmt.Errorf("model file %s not found in bundle", mm.FileName)
		}

		destPath := filepath.Join(destDir, mm.FileName)
		if err := os.WriteFile(destPath, modelData, 0o600); err != nil {
			return fmt.Errorf("write model %s: %w", mm.Alias, err)
		}

		if err := reg.Add(model.InstalledModel{
			Alias:       mm.Alias,
			Repo:        mm.Repo,
			Quant:       mm.Quant,
			Path:        destPath,
			SizeBytes:   mm.SizeBytes,
			InstalledAt: time.Now(),
		}); err != nil {
			return fmt.Errorf("register model %s: %w", mm.Alias, err)
		}
	}

	return reg.Save()
}

// addFileToTar adds the file at src to the tar archive with the given archive name.
func addFileToTar(tw *tar.Writer, src, archiveName string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	hdr := &tar.Header{
		Name:    archiveName,
		Mode:    int64(info.Mode()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy %s to tar: %w", src, err)
	}
	return nil
}

// addBytesToTar adds raw bytes to the tar archive with the given name.
func addBytesToTar(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write tar data: %w", err)
	}
	return nil
}
