# Quantisation and Unsloth — how to pick the right model file

## What is a GGUF file

GGUF (GPT-Generated Unified Format) is the model file format used by llama.cpp.
A GGUF file packages the model weights, vocabulary, and metadata into a single portable file.
No Python, no virtualenv, no CUDA toolkit required — llama.cpp links against it directly.

## What quantisation does

A full-precision neural network stores each weight as a 32-bit float (FP32) or 16-bit half-precision (FP16).
Quantisation maps those values to lower-bit integers, trading a small amount of accuracy for a
large reduction in file size and RAM usage.

```
FP16 weights  →  ~14 GB for a 7B model
Q8_0          →  ~7.7 GB   (8-bit)
Q4_K_M        →  ~4.7 GB   (4-bit, mixed precision on key layers)
Q2_K          →  ~2.5 GB   (2-bit, perceptible quality loss)
```

The quality loss from FP16 → Q4_K_M on a well-trained model is typically 1–3% on benchmarks.
For coding and tool-use tasks, Q4_K_M is the practical sweet spot for most hardware.

## The Unsloth quantisation series

[Unsloth](https://github.com/unslothai/unsloth) is a popular provider of pre-quantised GGUF
files on HuggingFace. Their value:

- They quantise within hours of each major model release.
- Every file is tested to confirm it loads and generates correctly.
- Multiple quant levels per model so you can match your RAM budget precisely.
- Clean, predictable naming: `unsloth/<ModelName>-GGUF`.

Most of the llmctl catalog uses Unsloth repos. When a model does not have an Unsloth release
(e.g. Hermes-3 from NousResearch) the catalog points to the upstream maintainer's GGUF repo instead.

## Quant levels and what they mean

| Quant | Bits per weight | RAM for 7B | Quality | When to use |
|---|---|---|---|---|
| F16 | 16 | ~14 GB | Reference | GPU inference, benchmarking |
| Q8_0 | 8 | ~7.7 GB | ≈ F16 | Plenty of RAM, want near-lossless |
| Q6_K | 6 | ~5.9 GB | Excellent | High quality, moderate RAM |
| **Q4_K_M** | **4 (mixed)** | **~4.7 GB** | **Very good** | **Default — best tradeoff** |
| Q4_K_S | 4 (small) | ~4.4 GB | Good | Slightly tighter RAM than Q4_K_M |
| Q3_K_M | 3 (mixed) | ~3.5 GB | Acceptable | RAM-constrained, non-critical tasks |
| Q2_K | 2 | ~2.7 GB | Noticeably degraded | Last resort |

The `_K_M` suffix (K-quant, Medium) means key layers — attention projections, feed-forward
gating — use a higher bit depth while less-sensitive layers are quantised more aggressively.
This preserves model quality at a smaller size than a uniform 4-bit quant.

## How to choose

### Step 1 — know your available RAM

Rule of thumb: GGUF file size + ~10% headroom must fit in RAM (unified memory on Apple Silicon,
system RAM on Linux). The context buffer (`--ctx-size`) adds overhead on top of that.

| Available RAM | Recommended |
|---|---|
| 8 GB | Q4_K_M of a 3–4B model, or Q2_K of a 7B |
| 16 GB | Q4_K_M of a 7–8B model, or Q4_K_S of a 13B |
| 24 GB | Q4_K_M of 13–14B, or Q4_K_M of Devstral-Small (12 GB) |
| 32 GB+ | Q8_0 of a 14B, or Q4_K_M of a 32B model |

### Step 2 — match the task

| Task | Recommendation |
|---|---|
| Agentic coding (file edits, shell, tool loops) | Devstral-Small Q4_K_M, Qwen2.5-Coder-14B Q4_K_M |
| Chat + completions on 8 GB RAM | Hermes-3-Llama-3.1-8B Q4_K_M, Qwen3-8B Q4_K_M |
| Chain-of-thought / reasoning | Qwen3-14B Q4_K_M, DeepSeek-R1-7B Q4_K_M |
| Fastest on CPU-only machine | Phi-3.5-mini Q4_K_M |

### Step 3 — check tool-call format

For agentic use the model must reliably emit structured tool calls.
See [tool-formats.md](tool-formats.md) for the full breakdown. Short version:

- **Native OpenAI `tool_calls` JSON** (needs `--jinja`): Devstral-Small, Hermes-3, Llama-3.x, Mistral v0.3+
- **Qwen XML format** (llmctl translates automatically): Qwen2.5-Coder series
- **No tool-call support**: CodeLlama, BLOOM, older base models

Start with the highest quant your RAM comfortably allows. Drop one level only if the model
refuses to load or inference is too slow for interactive use.

## Devstral-Small — quant notes

Devstral-Small-2505 is a Mixture-of-Experts (MoE) architecture. At Q4_K_M it is ~12 GB on
disk but only activates a fraction of parameters per token, so inference speed is closer to a
dense 7B model than a dense 24B model despite the total parameter count.

Q4_K_M is the right level for Devstral on 16–24 GB machines — there is no meaningful quality
gain from Q8_0 that would justify the extra 8 GB of RAM.

```bash
# Install at the recommended level (default)
llmctl model install Devstral-Small

# Q8_0 on a 32 GB machine
llmctl model install Devstral-Small --quant Q8_0
```
