# Model Management

## GGUF Format

GGUF (GPT-Generated Unified Format) is the file format used by llama.cpp to store
quantised language model weights. It is a binary container that includes:

- Model weights (quantised)
- Tokeniser vocabulary and merges
- Model metadata (architecture, context length, layer count)
- Hyperparameters

All models in the llmctl catalog are GGUF files. A typical file is named:
`<model>-<quantisation>.gguf`, e.g. `Qwen2.5-Coder-7B-Instruct-Q4_K_M.gguf`.

## Quantisation Levels

Quantisation reduces model size and memory footprint at the cost of some accuracy.
The llama.cpp k-quant format (`K_M`, `K_S`, `K_L`) uses mixed precision — different
layers are quantised at different bit depths for the best accuracy/size tradeoff.

| Level | Bits/weight | Size (7B) | Quality | Use when |
|-------|------------|-----------|---------|----------|
| `Q2_K` | ~2.6 | ~2.7 GB | Poor | Absolute minimum RAM |
| `Q3_K_M` | ~3.3 | ~3.3 GB | Acceptable | Very tight RAM |
| `Q4_K_M` | ~4.5 | ~4.4 GB | Good (default) | Best all-round choice |
| `Q5_K_M` | ~5.5 | ~5.3 GB | Very good | When accuracy matters |
| `Q6_K` | ~6.6 | ~6.1 GB | Excellent | Near-lossless |
| `Q8_0` | ~8.0 | ~7.2 GB | Best | Maximum quality, more RAM |
| `F16` | 16 | ~14 GB | Exact FP16 | Benchmarking / fine-tuning |

**Recommendation**: `Q4_K_M` for everyday use. Upgrade to `Q6_K` or `Q8_0` if
you notice quality degradation on complex tasks.

## Choosing a Model for Your Machine

| Available RAM | Recommended model | Quant | Notes |
|---|---|---|---|
| 4 GB | Phi-3.5-mini (3.8B) | Q4_K_M | Very fast, smaller context |
| 6 GB | Qwen2.5-Coder-7B | Q4_K_M | Excellent coder, tool use |
| 8 GB | Qwen3-8B | Q4_K_M | Reasoning + tool use |
| 12 GB | Qwen2.5-Coder-14B | Q4_K_M | High accuracy, larger context |
| 16 GB | Qwen3-14B or DeepSeek-R1-14B | Q4_K_M | Two 7B models simultaneously |
| 32 GB+ | Any 34B+ model | Q4_K_M | Large context, complex tasks |

These estimates assume the model is the only significant memory consumer.
Leave at least 2 GB free for the OS and other processes.

## The Builtin Catalog

Run `llmctl model catalog` to see all available models. The catalog includes:

| Model | Repo | Quant | Size | Tool Use | Reasoning |
|---|---|---|---|---|---|
| Qwen2.5-Coder-7B | unsloth/Qwen2.5-Coder-7B-Instruct-GGUF | Q4_K_M | 4.7 GB | yes | no |
| Qwen2.5-Coder-14B | unsloth/Qwen2.5-Coder-14B-Instruct-GGUF | Q4_K_M | 8.9 GB | yes | no |
| Qwen3-8B | unsloth/Qwen3-8B-GGUF | Q4_K_M | 5.2 GB | yes | yes |
| Qwen3-14B | unsloth/Qwen3-14B-GGUF | Q4_K_M | 9.3 GB | yes | yes |
| DeepSeek-R1-7B | unsloth/DeepSeek-R1-Distill-Qwen-7B-GGUF | Q4_K_M | 4.7 GB | no | yes |
| DeepSeek-R1-14B | unsloth/DeepSeek-R1-Distill-Qwen-14B-GGUF | Q4_K_M | 8.9 GB | no | yes |
| Llama-3.1-8B | unsloth/Meta-Llama-3.1-8B-Instruct-GGUF | Q4_K_M | 4.9 GB | yes | no |
| Mistral-7B-v0.3 | unsloth/mistral-7b-instruct-v0.3-GGUF | Q4_K_M | 4.1 GB | yes | no |
| Phi-3.5-mini | unsloth/Phi-3.5-mini-instruct-GGUF | Q4_K_M | 2.2 GB | yes | no |
| CodeLlama-13B | TheBloke/CodeLlama-13B-Instruct-GGUF | Q4_K_M | 7.9 GB | no | no |

Install a catalog model:

```bash
llmctl model install Qwen2.5-Coder-7B
```

Install with a specific quantisation:

```bash
llmctl model install Qwen2.5-Coder-7B --quant Q8_0
```

## Custom Models from HuggingFace

Any GGUF model on HuggingFace can be installed. Use `llmctl model search` to
query the HuggingFace API:

```bash
llmctl model search "phi-4 gguf"
```

Install by HuggingFace repo:

```bash
llmctl model install unsloth/Phi-4-GGUF --quant Q4_K_M --alias phi4
```

The alias is what you'll use to reference the model in your config and when
starting the server:

```bash
llmctl config set server.model phi4
llmctl server start
```

## Model Files and Registry

Models are downloaded to `~/.local/share/llmctl/models/` by default.
The registry is stored at `~/.local/share/llmctl/models.json`.

List installed models:

```bash
llmctl model list
```

Remove a model (only unregisters; does not delete the file):

```bash
llmctl model remove phi4 --yes
```
