package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// hfBaseURL is the HuggingFace API base URL. It is a package-level variable
// so tests can substitute a stub server.
var hfBaseURL = "https://huggingface.co"

// SearchResult represents a single model returned by the HuggingFace API.
type SearchResult struct {
	ID          string
	Downloads   int
	Description string
}

// Search queries the HuggingFace API for GGUF models matching query.
// If the request fails, a non-nil error is returned and results is nil.
func Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	endpoint := hfBaseURL + "/api/models"
	params := url.Values{}
	params.Set("filter", "gguf")
	params.Set("search", query)
	params.Set("sort", "downloads")
	params.Set("limit", strconv.Itoa(limit))

	fullURL := endpoint + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build HF search request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HF search request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close on read path

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HF API returned %d", resp.StatusCode)
	}

	var raw []struct {
		ModelID     string `json:"modelId"`
		Downloads   int    `json:"downloads"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode HF search response: %w", err)
	}

	out := make([]SearchResult, 0, len(raw))
	for _, r := range raw {
		out = append(out, SearchResult{
			ID:          r.ModelID,
			Downloads:   r.Downloads,
			Description: r.Description,
		})
	}
	return out, nil
}
