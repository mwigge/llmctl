package model

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistry_AddAndGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")

	r, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	m := InstalledModel{
		Alias:       "qwen7b",
		Repo:        "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF",
		Quant:       "Q4_K_M",
		Path:        "/tmp/qwen7b.gguf",
		SizeBytes:   5_000_000_000,
		InstalledAt: time.Now().UTC().Truncate(time.Second),
		LastUsed:    time.Now().UTC().Truncate(time.Second),
	}

	if err := r.Add(m); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := r.Get("qwen7b")
	if !ok {
		t.Fatal("Get returned not-found after Add")
	}
	if got.Alias != m.Alias {
		t.Errorf("Alias = %q, want %q", got.Alias, m.Alias)
	}
	if got.Repo != m.Repo {
		t.Errorf("Repo = %q, want %q", got.Repo, m.Repo)
	}
	if got.Path != m.Path {
		t.Errorf("Path = %q, want %q", got.Path, m.Path)
	}
}

func TestRegistry_List(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewRegistry(filepath.Join(dir, "models.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	if got := r.List(); len(got) != 0 {
		t.Errorf("List on empty registry returned %d items, want 0", len(got))
	}

	models := []InstalledModel{
		{Alias: "a", Repo: "owner/a", Quant: "Q4_K_M", Path: "/tmp/a.gguf", InstalledAt: time.Now()},
		{Alias: "b", Repo: "owner/b", Quant: "Q4_K_M", Path: "/tmp/b.gguf", InstalledAt: time.Now()},
	}
	for _, m := range models {
		if err := r.Add(m); err != nil {
			t.Fatalf("Add(%q): %v", m.Alias, err)
		}
	}

	list := r.List()
	if len(list) != 2 {
		t.Errorf("List returned %d items, want 2", len(list))
	}
}

func TestRegistry_Remove(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewRegistry(filepath.Join(dir, "models.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	m := InstalledModel{Alias: "toremove", Repo: "owner/m", Quant: "Q4_K_M", Path: "/tmp/m.gguf", InstalledAt: time.Now()}
	if err := r.Add(m); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := r.Remove("toremove"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := r.Get("toremove"); ok {
		t.Error("Get should return not-found after Remove")
	}
}

func TestRegistry_RemoveNonExistent_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewRegistry(filepath.Join(dir, "models.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	err = r.Remove("does-not-exist")
	if err == nil {
		t.Fatal("Remove non-existent alias should return an error")
	}
	if !errors.Is(err, ErrAliasNotFound) {
		t.Errorf("Remove returned %v, want ErrAliasNotFound", err)
	}
}

func TestRegistry_SaveAndReload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "models.json")

	r1, err := NewRegistry(storePath)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	m := InstalledModel{
		Alias:       "persist-test",
		Repo:        "owner/model",
		Quant:       "Q8_0",
		Path:        "/tmp/model.gguf",
		SizeBytes:   9_000_000_000,
		InstalledAt: now,
		LastUsed:    now,
	}
	if err := r1.Add(m); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := r1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	r2, err := NewRegistry(storePath)
	if err != nil {
		t.Fatalf("NewRegistry (reload): %v", err)
	}
	got, ok := r2.Get("persist-test")
	if !ok {
		t.Fatal("Get after reload returned not-found")
	}
	if got.SizeBytes != m.SizeBytes {
		t.Errorf("SizeBytes = %d, want %d", got.SizeBytes, m.SizeBytes)
	}
	if !got.InstalledAt.Equal(m.InstalledAt) {
		t.Errorf("InstalledAt = %v, want %v", got.InstalledAt, m.InstalledAt)
	}
}

func TestRegistry_AddDuplicateAlias_Overwrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewRegistry(filepath.Join(dir, "models.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	m1 := InstalledModel{Alias: "dup", Repo: "owner/v1", Quant: "Q4_K_M", Path: "/tmp/v1.gguf", InstalledAt: time.Now()}
	m2 := InstalledModel{Alias: "dup", Repo: "owner/v2", Quant: "Q8_0", Path: "/tmp/v2.gguf", InstalledAt: time.Now()}
	if err := r.Add(m1); err != nil {
		t.Fatalf("Add m1: %v", err)
	}
	if err := r.Add(m2); err != nil {
		t.Fatalf("Add m2: %v", err)
	}

	list := r.List()
	if len(list) != 1 {
		t.Errorf("List after duplicate add: got %d, want 1", len(list))
	}
	got, _ := r.Get("dup")
	if got.Repo != "owner/v2" {
		t.Errorf("Repo after overwrite = %q, want %q", got.Repo, "owner/v2")
	}
}
