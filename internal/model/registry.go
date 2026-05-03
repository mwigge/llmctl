package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrAliasNotFound is returned when an alias does not exist in the registry.
var ErrAliasNotFound = errors.New("alias not found")

// InstalledModel records a locally installed GGUF model.
type InstalledModel struct {
	Alias       string    `json:"alias"`
	Repo        string    `json:"repo"`
	Quant       string    `json:"quant"`
	Path        string    `json:"path"`        // absolute path to .gguf
	SizeBytes   int64     `json:"size_bytes"`
	InstalledAt time.Time `json:"installed_at"`
	LastUsed    time.Time `json:"last_used"`
}

// Registry manages the set of locally installed models, backed by a JSON
// file at storePath.
type Registry struct {
	storePath string
	models    map[string]InstalledModel // keyed by alias
}

// NewRegistry loads the registry from storePath. If the file does not exist
// an empty registry is returned. Any other I/O or parse error is returned.
func NewRegistry(storePath string) (*Registry, error) {
	r := &Registry{
		storePath: storePath,
		models:    make(map[string]InstalledModel),
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r, nil
		}
		return nil, fmt.Errorf("read registry %s: %w", storePath, err)
	}

	var list []InstalledModel
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse registry %s: %w", storePath, err)
	}
	for _, m := range list {
		r.models[m.Alias] = m
	}
	return r, nil
}

// List returns all installed models in insertion-stable order.
// The returned slice is a copy; mutations do not affect the registry.
func (r *Registry) List() []InstalledModel {
	out := make([]InstalledModel, 0, len(r.models))
	for _, m := range r.models {
		out = append(out, m)
	}
	return out
}

// Add inserts or replaces the model with the given alias.
func (r *Registry) Add(m InstalledModel) error {
	r.models[m.Alias] = m
	return nil
}

// Remove deletes the model with the given alias.
// Returns ErrAliasNotFound if alias is not present.
func (r *Registry) Remove(alias string) error {
	if _, ok := r.models[alias]; !ok {
		return fmt.Errorf("%w: %s", ErrAliasNotFound, alias)
	}
	delete(r.models, alias)
	return nil
}

// Get returns the model for the given alias.
func (r *Registry) Get(alias string) (InstalledModel, bool) {
	m, ok := r.models[alias]
	return m, ok
}

// Save persists the registry to storePath.
func (r *Registry) Save() error {
	list := r.List()
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(r.storePath, data, 0o600); err != nil {
		return fmt.Errorf("write registry %s: %w", r.storePath, err)
	}
	return nil
}
