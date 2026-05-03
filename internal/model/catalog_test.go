package model

import (
	"strings"
	"testing"
)

func TestBuiltinCatalog_HasMinimumEntries(t *testing.T) {
	t.Parallel()
	if got := len(BuiltinCatalog); got < 8 {
		t.Errorf("BuiltinCatalog has %d entries, want >= 8", got)
	}
}

func TestBuiltinCatalog_AllEntriesHaveRequiredFields(t *testing.T) {
	t.Parallel()
	for i, e := range BuiltinCatalog {
		if e.Name == "" {
			t.Errorf("entry[%d].Name is empty", i)
		}
		if e.Repo == "" {
			t.Errorf("entry[%d] (%s).Repo is empty", i, e.Name)
		}
		if e.Quant == "" {
			t.Errorf("entry[%d] (%s).Quant is empty", i, e.Name)
		}
		if e.SizeGB == "" {
			t.Errorf("entry[%d] (%s).SizeGB is empty", i, e.Name)
		}
	}
}

func TestBuiltinCatalog_RepoOwnerNameFormat(t *testing.T) {
	t.Parallel()
	for _, e := range BuiltinCatalog {
		if !strings.Contains(e.Repo, "/") {
			t.Errorf("entry %q: Repo %q missing owner/name separator", e.Name, e.Repo)
		}
	}
}

func TestCatalogByIndex_ReturnsEntryForValidIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		index int
		valid bool
	}{
		{"first entry", 1, true},
		{"last entry", len(BuiltinCatalog), true},
		{"zero is invalid", 0, false},
		{"beyond range", len(BuiltinCatalog) + 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entry, ok := CatalogByIndex(tt.index)
			if ok != tt.valid {
				t.Errorf("CatalogByIndex(%d) ok=%v, want %v", tt.index, ok, tt.valid)
			}
			if tt.valid && entry.Name == "" {
				t.Errorf("CatalogByIndex(%d) returned entry with empty Name", tt.index)
			}
		})
	}
}

func TestCatalogByPrefix_MatchesNamePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		prefix     string
		wantFound  bool
		wantName   string
	}{
		{"exact match Qwen2.5-Coder-7B", "Qwen2.5-Coder-7B", true, "Qwen2.5-Coder-7B"},
		{"prefix Qwen3-8", "Qwen3-8", true, "Qwen3-8B"},
		{"lowercase prefix", "qwen", true, ""},
		{"DeepSeek prefix", "DeepSeek-R1", true, "DeepSeek-R1-7B"},
		{"no match", "NonExistent-XYZ", false, ""},
		{"empty prefix matches first", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries := CatalogByPrefix(tt.prefix)
			if tt.wantFound && len(entries) == 0 {
				t.Errorf("CatalogByPrefix(%q) returned no results, want >= 1", tt.prefix)
			}
			if !tt.wantFound && len(entries) != 0 {
				t.Errorf("CatalogByPrefix(%q) returned %d results, want 0", tt.prefix, len(entries))
			}
			if tt.wantName != "" {
				found := false
				for _, e := range entries {
					if e.Name == tt.wantName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("CatalogByPrefix(%q): entry %q not in results %v", tt.prefix, tt.wantName, entries)
				}
			}
		})
	}
}

func TestBuiltinCatalog_ToolUseAndReasoningFlags(t *testing.T) {
	t.Parallel()
	// Verify specific known entries have correct flags
	tests := []struct {
		name      string
		toolUse   bool
		reasoning bool
	}{
		{"Qwen2.5-Coder-7B", true, false},
		{"Qwen3-8B", true, true},
		{"DeepSeek-R1-7B", false, true},
		{"CodeLlama-13B", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var found bool
			for _, e := range BuiltinCatalog {
				if e.Name == tt.name {
					found = true
					if e.ToolUse != tt.toolUse {
						t.Errorf("%s: ToolUse=%v, want %v", tt.name, e.ToolUse, tt.toolUse)
					}
					if e.Reasoning != tt.reasoning {
						t.Errorf("%s: Reasoning=%v, want %v", tt.name, e.Reasoning, tt.reasoning)
					}
					break
				}
			}
			if !found {
				t.Errorf("entry %q not found in BuiltinCatalog", tt.name)
			}
		})
	}
}
