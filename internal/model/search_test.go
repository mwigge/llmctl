package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Search tests do not call t.Parallel() because they mutate the package-level
// hfBaseURL variable. Tests that share mutable package state must run serially.

func TestSearch_ReturnsResultsFromHFAPI(t *testing.T) {
	payload := []map[string]any{
		{"modelId": "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF", "downloads": 50000, "description": "A coder model"},
		{"modelId": "TheBloke/CodeLlama-13B-GGUF", "downloads": 30000, "description": ""},
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("filter") != "gguf" {
			http.Error(w, "bad filter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	prevBaseURL := hfBaseURL
	t.Cleanup(func() { hfBaseURL = prevBaseURL })
	hfBaseURL = srv.URL

	results, err := Search(context.Background(), "qwen coder", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ID != "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF" {
		t.Errorf("results[0].ID = %q, want %q", results[0].ID, "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF")
	}
	if results[0].Downloads != 50000 {
		t.Errorf("results[0].Downloads = %d, want 50000", results[0].Downloads)
	}
}

func TestSearch_NetworkError_ReturnsError(t *testing.T) {
	prevBaseURL := hfBaseURL
	t.Cleanup(func() { hfBaseURL = prevBaseURL })
	// Point to a guaranteed-refused address.
	hfBaseURL = "http://127.0.0.1:1"

	results, err := Search(context.Background(), "anything", 5)
	if err == nil {
		t.Error("expected error on network failure, got nil")
	}
	if results != nil {
		t.Errorf("expected nil results on error, got %v", results)
	}
}

func TestSearch_EmptyResponse_ReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	prevBaseURL := hfBaseURL
	t.Cleanup(func() { hfBaseURL = prevBaseURL })
	hfBaseURL = srv.URL

	results, err := Search(context.Background(), "nothing", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSearch_RespectsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "3" {
			http.Error(w, "bad limit", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	prevBaseURL := hfBaseURL
	t.Cleanup(func() { hfBaseURL = prevBaseURL })
	hfBaseURL = srv.URL

	_, err := Search(context.Background(), "test", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}
