# Performance Tuning

## Why llama.cpp Is Fast

llama.cpp is a C++ implementation of LLM inference with the following hardware-specific
optimisations:

- **Apple Silicon (Metal)**: GPU-accelerated matrix multiplication via Metal shaders.
  On M1/M2/M3/M4 chips, Metal offloading can achieve 40–80 tokens/s for 7B models.
- **NVIDIA CUDA**: CUDA kernels for matrix operations. Requires the CUDA build of
  llama-server (`llama-server-cuda`).
- **CPU SIMD**: AVX2 (256-bit) and AVX-512 (512-bit) vectorised matrix operations on
  x86-64. ARM NEON on Apple Silicon CPUs.
- **Flash Attention**: `--flash-attn` reduces memory bandwidth for long contexts.

Python-based inference (Hugging Face `transformers`, `vLLM`) adds Python overhead,
the PyTorch abstraction layer, and CUDA context setup to every request.
llama.cpp bypasses all of that.

## Context Size Tradeoffs

Context size (`ctx_size`) determines how much text the model can "see" at once.

| Context | RAM overhead (7B model) | Throughput | Use when |
|---|---|---|---|
| 4,096 | ~0.5 GB | Fastest | Short tasks, minimal RAM |
| 16,384 | ~1.5 GB | Fast (default) | Most development tasks |
| 32,768 | ~3 GB | Moderate | Large files, long conversations |
| 65,536 | ~6 GB | Slow | Documents, repository-level context |
| 131,072 | ~12 GB | Very slow | Only on 32 GB+ RAM |

Larger context requires more KV-cache memory and reduces throughput.
Match context size to your typical task, not to the maximum the model supports.

Configure:

```bash
llmctl config set server.ctx_size 32768
```

## Thread Count Tuning

Threads control CPU parallelism for the matrix operations. More threads generally
means faster prefill (prompt processing) and faster decoding on CPU.

**Rule of thumb**: set threads to **physical core count minus 1**.

```bash
# Count physical cores (Linux)
grep "^cpu cores" /proc/cpuinfo | head -1 | awk '{print $NF}'

# Count physical cores (macOS)
sysctl -n hw.physicalcpu

# Apply
llmctl config set server.threads 7   # on an 8-core machine
```

Hyperthreading (logical cores) does not help for LLM inference — llama.cpp
already uses SIMD that saturates one physical core.

## GPU Layer Offloading

`n-gpu-layers` controls how many transformer layers are offloaded to the GPU.
Each layer moved to the GPU reduces CPU RAM usage and increases GPU utilisation.

| `n-gpu-layers` | Effect |
|---|---|
| `0` | CPU-only (default) |
| `1–N` | Partial offload: bottom N layers on GPU, rest on CPU |
| `999` (or `-1`) | Full offload: all layers on GPU |

Full offload gives maximum throughput but requires enough VRAM for all layers.
For a 7B Q4_K_M model, full offload needs ~5 GB VRAM.

```bash
llmctl config set server.gpu_layers 32   # offload 32 layers
```

Check available VRAM:

```bash
# NVIDIA
nvidia-smi --query-gpu=memory.free --format=csv

# Apple Silicon (shared memory — use Activity Monitor or system_profiler)
system_profiler SPDisplaysDataType | grep VRAM
```

## Batch Size for Throughput vs Latency

`--batch-size` controls how many tokens are processed in parallel during prompt
processing (prefill phase).

- **Higher batch size** (`512–2048`): better throughput for long prompts,
  higher peak memory.
- **Lower batch size** (`128–256`): lower latency for short prompts,
  lower peak memory.

The default is 512, which is a good all-round value. For a server handling
multiple concurrent users, increase to 1024–2048 if you have the RAM.

## Benchmark: Tokens/Second by Model and Hardware

These are representative numbers on well-optimised hardware. Your results will vary.

| Model | Quant | Hardware | tok/s (generate) | Notes |
|---|---|---|---|---|
| Qwen2.5-7B | Q4_K_M | M3 Pro 12-core (GPU full) | ~65 | Metal full offload |
| Qwen2.5-7B | Q4_K_M | M3 Pro 12-core (CPU only) | ~18 | 11 threads |
| Qwen2.5-7B | Q4_K_M | AMD Ryzen 9 7950X (CPU) | ~22 | 16 threads, AVX-512 |
| Qwen2.5-7B | Q4_K_M | RTX 4090 (CUDA full) | ~95 | 24 GB VRAM |
| Qwen2.5-14B | Q4_K_M | M3 Max 14-core (GPU full) | ~45 | Metal full offload |
| Qwen2.5-14B | Q4_K_M | RTX 4090 (CUDA full) | ~55 | Just fits in 24 GB VRAM |
| DeepSeek-R1-7B | Q4_K_M | M3 Pro (GPU full) | ~58 | Reasoning token overhead |

**Tip**: run `llmctl server start` and watch `llmctl metrics show` to see
actual throughput on your hardware.

## Profiling a Request

Use `curl` with timing to measure time-to-first-token (TTFT) and total latency:

```bash
time curl -s http://localhost:8765/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen7b",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 1
  }' > /dev/null
```

For more detailed per-token timing, use the llama-server `/completion` endpoint
with `stream: true` and timestamp each chunk.
