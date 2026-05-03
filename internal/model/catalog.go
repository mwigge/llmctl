package model

import "strings"

// CatalogEntry describes a curated GGUF model available for download.
type CatalogEntry struct {
	Name        string
	Repo        string // HuggingFace repo: owner/name
	Quant       string // e.g. Q4_K_M, Q8_0
	SizeGB      string
	MinRAMGB    int
	ToolUse     bool
	Reasoning   bool
	Description string
}

// BuiltinCatalog is the curated list of well-known GGUF models.
var BuiltinCatalog = []CatalogEntry{
	{
		Name:        "Qwen2.5-Coder-7B",
		Repo:        "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "4.7GB",
		MinRAMGB:    6,
		ToolUse:     true,
		Description: "Fast 7B coding model with tool-use support",
	},
	{
		Name:        "Qwen2.5-Coder-14B",
		Repo:        "unsloth/Qwen2.5-Coder-14B-Instruct-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "8.9GB",
		MinRAMGB:    12,
		ToolUse:     true,
		Description: "High-quality 14B coding model with tool-use support",
	},
	{
		Name:        "Qwen3-8B",
		Repo:        "unsloth/Qwen3-8B-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "5.2GB",
		MinRAMGB:    8,
		ToolUse:     true,
		Reasoning:   true,
		Description: "Qwen3 8B with tool-use and chain-of-thought reasoning",
	},
	{
		Name:        "Qwen3-14B",
		Repo:        "unsloth/Qwen3-14B-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "9.3GB",
		MinRAMGB:    12,
		ToolUse:     true,
		Reasoning:   true,
		Description: "Qwen3 14B with tool-use and chain-of-thought reasoning",
	},
	{
		Name:        "DeepSeek-R1-7B",
		Repo:        "unsloth/DeepSeek-R1-Distill-Qwen-7B-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "4.7GB",
		MinRAMGB:    6,
		Reasoning:   true,
		Description: "DeepSeek R1 distilled 7B with chain-of-thought reasoning",
	},
	{
		Name:        "DeepSeek-Coder-V2-Lite",
		Repo:        "unsloth/DeepSeek-Coder-V2-Lite-Instruct-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "9.0GB",
		MinRAMGB:    12,
		ToolUse:     true,
		Description: "DeepSeek Coder V2 Lite with tool-use support",
	},
	{
		Name:        "Llama-3.1-8B",
		Repo:        "unsloth/Meta-Llama-3.1-8B-Instruct-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "4.9GB",
		MinRAMGB:    8,
		ToolUse:     true,
		Description: "Meta Llama 3.1 8B instruction-tuned with tool-use",
	},
	{
		Name:        "Mistral-7B-v0.3",
		Repo:        "unsloth/mistral-7b-instruct-v0.3-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "4.1GB",
		MinRAMGB:    6,
		ToolUse:     true,
		Description: "Mistral 7B v0.3 instruction model with tool-use",
	},
	{
		Name:        "Phi-3.5-mini",
		Repo:        "unsloth/Phi-3.5-mini-instruct-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "2.2GB",
		MinRAMGB:    4,
		ToolUse:     true,
		Description: "Microsoft Phi-3.5 mini — compact and fast with tool-use",
	},
	{
		Name:        "CodeLlama-13B",
		Repo:        "TheBloke/CodeLlama-13B-Instruct-GGUF",
		Quant:       "Q4_K_M",
		SizeGB:      "7.9GB",
		MinRAMGB:    10,
		Description: "Meta CodeLlama 13B instruct, strong at code generation",
	},
}

// CatalogByIndex returns the catalog entry at 1-based index i.
// Returns false if i is out of range.
func CatalogByIndex(i int) (CatalogEntry, bool) {
	if i < 1 || i > len(BuiltinCatalog) {
		return CatalogEntry{}, false
	}
	return BuiltinCatalog[i-1], true
}

// CatalogByPrefix returns all entries whose Name contains prefix
// (case-insensitive). An empty prefix matches everything.
func CatalogByPrefix(prefix string) []CatalogEntry {
	lower := strings.ToLower(prefix)
	out := make([]CatalogEntry, 0, len(BuiltinCatalog))
	for _, e := range BuiltinCatalog {
		if strings.Contains(strings.ToLower(e.Name), lower) {
			out = append(out, e)
		}
	}
	return out
}
