# Hardware deployment guide

Recommendations for seven common deployment scenarios. Every configuration runs llama.cpp
(`llama-server`) as the inference backend — the difference is how many layers you offload
to GPU and which model fits within your memory budget.

---

## Comparison table

| # | Hardware | VRAM | RAM | Best model | Quant | GPU layers | Mode | Tokens/s (est.) |
|---|---|---|---|---|---|---|---|---|
| 1 | VM, 64 GB RAM, no GPU | — | 64 GB | Devstral-Small | Q4_K_M | 0 (CPU) | single | 3–8 |
| 2 | Docker, 32 GB RAM, no GPU | — | 32 GB | Devstral-Small | Q4_K_M | 0 (CPU) | single | 2–5 |
| 3 | Laptop, 32 GB RAM, RTX 3080 8 GB | 8 GB | 32 GB | Devstral-Small (partial) or Hermes-3 (full) | Q4_K_M | 20 / 99 | single | 15–25 / 45–65 |
| 4 | Laptop, 16 GB RAM, RTX 4060 8 GB | 8 GB | 16 GB | Hermes-3-Llama-3.1-8B | Q4_K_M | 99 | single | 50–70 |
| 5 | Server, 2× H100 80 GB | 160 GB | — | Llama-3.1-70B or Qwen3-32B | Q8_0 | 99 (split) | hot-swap | 80–200+ |
| 6 | Server, RTX 5070 Ti 16 GB, 16 GB RAM | 16 GB | 16 GB | Devstral-Small | Q4_K_M | 99 | single | 60–90 |
| 7 | Server, Intel 14th gen, RTX 5070 Ti 16 GB, 32 GB RAM | 16 GB | 32 GB | Devstral-Small | Q4_K_M | 99 | cold-swap | 65–95 |

Tokens/s figures are for a single concurrent user. Context size 32768, no batching.

---

## Scenario details

### 1 — VM, 64 GB RAM, no GPU

Pure CPU inference. 64 GB means you can run Devstral-Small (12 GB) and still have 50 GB
free for OS, application, and a large context buffer.

```bash
llmctl model install Devstral-Small
llmctl config set server.gpu_layers 0
llmctl config set server.threads 16        # match physical core count
llmctl config set server.ctx_size 65536    # 64 GB can afford a bigger context
llmctl server start
```

**Notes:**
- Set `threads` to the number of **physical cores** (not hyperthreads). On a 16-core VM,
  use `--threads 16`. Hyperthreading does not help matrix multiplications.
- CPU inference on a modern 8-core server CPU runs Devstral at ~3–8 tokens/s — acceptable
  for batch workloads, slow for interactive chat. For interactive use, try Hermes-3-8B
  (4.9 GB) which will run at ~8–15 tokens/s on the same hardware.
- If latency matters more than model quality, drop to Q4_K_M of a 7B model.
- Cold-swap mode lets you keep multiple models on disk and load on demand without
  wasting RAM on an idle model.

```bash
# Alternative: faster 8B model for interactive use
llmctl model install Hermes-3-Llama-3.1-8B
llmctl config set server.ctx_size 32768
```

---

### 2 — Docker containers, 32 GB RAM, no GPU

Similar to scenario 1 but container overhead reduces effective RAM.
Allocate at least 18 GB of container memory (`--memory 18g`) to give Devstral Q4_K_M (12 GB)
+ 4 GB KV cache + 2 GB headroom for the container runtime.

```bash
# docker-compose.yml example
services:
  llmctl:
    image: ghcr.io/mwigge/llmctl:latest
    mem_limit: 20g
    environment:
      - LLMCTL_MODEL=devstral-small
      - LLMCTL_GPU_LAYERS=0
      - LLMCTL_CTX_SIZE=32768
      - LLMCTL_THREADS=8
    ports:
      - "8765:8765"
    volumes:
      - llmctl-models:/root/.local/share/llmctl/models
```

**Notes:**
- Docker does **not** pass through NVIDIA GPUs by default. GPU access requires
  `nvidia-container-toolkit` and `--gpus all` in the run command (or `deploy.resources`
  in Compose). Without it the container is CPU-only regardless of host GPU.
- CPU allocation in containers is soft-limited by `--cpus`. Set it to at least 4 to
  avoid throttling during matrix multiplications.
- For multiple containers sharing the same model file, mount the models directory
  as a read-only shared volume — avoid downloading 12 GB per container instance.

```bash
llmctl config set server.gpu_layers 0
llmctl config set server.threads 8
```

---

### 3 — Laptop, 32 GB RAM, RTX 3080 8 GB VRAM

The 3080 8 GB has enough VRAM for a fully-offloaded 7B model or a partially-offloaded 14B+.

**Option A — Devstral-Small, partial offload (quality first)**

Devstral Q4_K_M is 12 GB total. With 8 GB VRAM you can offload roughly 20–25 layers
(~6 GB) to GPU; the remainder runs on CPU using system RAM. This hybrid mode is roughly
2–3× faster than pure CPU.

```bash
llmctl config set server.gpu_layers 22
llmctl config set server.ctx_size 32768
llmctl server start
# expect ~15–25 tokens/s
```

**Option B — Hermes-3 8B, full offload (speed first)**

4.9 GB fits entirely in 8 GB VRAM with 3 GB to spare for the KV cache.
All computation stays on GPU — no CPU ↔ GPU data transfer bottleneck.

```bash
llmctl model install Hermes-3-Llama-3.1-8B
llmctl config set server.gpu_layers 99   # offload all layers
llmctl config set server.ctx_size 32768
llmctl server start
# expect ~45–65 tokens/s
```

**Notes:**
- The RTX 3080 8 GB uses GDDR6X and has 320 GB/s memory bandwidth —
  plenty for a 7B model, limited for larger models.
- Cold-swap lets you flip between Devstral (slow, high quality) and Hermes-3
  (fast, lower ceiling) without restarting the server.

```bash
llmctl config mode cold-swap
llmctl config set server.gpu_layers 99
```

---

### 4 — Laptop, 16 GB RAM, RTX 4060 8 GB VRAM

The tight system RAM means Devstral (12 GB) leaves only 4 GB for the OS — too little
for comfortable use. Stick to 7–8B models.

With 8 GB VRAM and a 4.9 GB model, all layers fit on GPU with ~3 GB free for
the KV cache, giving excellent interactive latency.

```bash
llmctl model install Hermes-3-Llama-3.1-8B
llmctl config set server.gpu_layers 99
llmctl config set server.ctx_size 32768
llmctl server start
# expect ~50–70 tokens/s
```

**Alternatives at this RAM budget:**

| Model | GGUF size | Fits in VRAM? | Notes |
|---|---|---|---|
| Hermes-3-Llama-3.1-8B Q4_K_M | 4.9 GB | Yes (fully) | Best agentic 8B |
| Qwen2.5-Coder-7B Q4_K_M | 4.7 GB | Yes (fully) | Strong coder, XML tools |
| Qwen3-8B Q4_K_M | 5.2 GB | Yes (fully) | Reasoning + tools |
| Devstral-Small Q4_K_M | 12 GB | No — partial only | ~10 layers on GPU, rest on 16GB RAM |

If you want to run Devstral on this machine, reduce ctx-size to 8192 to save RAM:

```bash
llmctl config set server.gpu_layers 15    # ~5 GB on GPU
llmctl config set server.ctx_size 8192    # keep KV cache small
```

This will work but is not recommended for agentic loops that need long context.

**Notes:**
- The RTX 4060 has 128-bit memory bus vs 256-bit on 3080 — lower bandwidth,
  but Ada Lovelace architecture makes better use of it for smaller models.
- For coding and tool-use with a 7–8B model, 4060 8 GB is a very capable setup.

---

### 5 — Server, 2× H100 80 GB SXM

160 GB total VRAM. This is the tier where you run 70B+ models at full or near-full
precision, or multiple concurrent models.

```bash
llmctl model install Llama-3.1-70B --quant Q8_0      # ~70 GB, near-lossless
llmctl config set server.gpu_layers 99
llmctl config set server.ctx_size 65536
llmctl config mode hot-swap
llmctl server start
```

**Recommended split strategy:**

| Use case | Model A | Model B | Config |
|---|---|---|---|
| Quality + speed | Llama-3.1-70B Q8_0 (70 GB) | Devstral-Small Q8_0 (24 GB) | hot-swap |
| Dual workload | Qwen3-32B Q8_0 (32 GB) on H100-0 | Qwen3-32B Q8_0 (32 GB) on H100-1 | parallel |
| Max context | Llama-3.1-70B Q4_K_M (38 GB) | ctx_size 131072 (full train context) | single |

**Multi-GPU with llama.cpp:**

llama.cpp splits tensor layers across GPUs automatically. Use `--split-mode layer`
(the default) for balanced throughput on matched cards. NVLink between H100 SXM
cards reduces inter-GPU transfer latency significantly.

```bash
# manual tensor split if one card is primary compute
llmctl config set server.extra_args "--split-mode layer --tensor-split 1,1"
```

**Notes:**
- H100 SXM has 3.35 TB/s HBM3 bandwidth. A 70B Q8_0 model runs at 150–250 tokens/s
  for a single user stream.
- With multiple concurrent users, batching (`--parallel N`) multiplies throughput
  at the cost of per-request latency. Set `--parallel 4` for 4 concurrent users.
- This hardware tier is overkill for `llmctl` single-user mode — consider vLLM or
  TGI for multi-user production serving at this scale.

---

### 6 — Server, RTX 5070 Ti 16 GB VRAM, 16 GB system RAM

The RTX 5070 Ti has 16 GB GDDR7 — Devstral-Small Q4_K_M (12 GB) fits entirely with
4 GB to spare for the KV cache. All layers on GPU, no CPU fallback needed.

```bash
llmctl model install Devstral-Small
llmctl config set server.gpu_layers 99
llmctl config set server.ctx_size 32768
llmctl server start
# expect ~60–90 tokens/s (Blackwell GB205 + GDDR7)
```

**Why this is the best single-GPU local setup for Devstral:**
- 16 GB VRAM covers Devstral Q4_K_M (12 GB) + full 32K KV cache without spill.
- GDDR7 bandwidth (~896 GB/s on 5070 Ti) is ~2× the 3080 8 GB — matrix loads are
  faster despite similar CUDA core count.
- Blackwell architecture (sm_120) includes improved FP8 and INT4 execution paths
  that llama.cpp uses via CUDA backend automatically.

**Note on system RAM:** 16 GB system RAM is tight. Since the model weights live in VRAM,
system RAM is only used for the process heap, context overhead, and OS. Keep other
applications closed when running at ctx 32768. To go to ctx 65536 you will need more
system RAM or reduce ctx.

```bash
# if RAM pressure is an issue
llmctl config set server.ctx_size 16384   # halves KV cache system overhead
```

---

### 7 — Server, Intel Core 14th gen, RTX 5070 Ti 16 GB VRAM, 32 GB system RAM

This is scenario 6 with the system RAM constraint resolved. 32 GB of system RAM means
you can run Devstral at full 32K (or even 65K) context, keep the OS comfortable, and
use cold-swap to keep a second model on disk ready to load within ~10 seconds.

```bash
llmctl model install Devstral-Small
llmctl model install Hermes-3-Llama-3.1-8B   # fast fallback model
llmctl config set server.gpu_layers 99
llmctl config set server.ctx_size 32768
llmctl config mode cold-swap
llmctl server start
# expect ~65–95 tokens/s (Blackwell GB205, GDDR7, full VRAM offload)
```

**Why cold-swap here:**
With 16 GB VRAM only one model fits in VRAM at a time. Cold-swap (via `llama-swap`)
evicts the current model and loads the new one on request, with a TTL so the last-used
model stays warm for 10 minutes. The 32 GB system RAM means the OS page cache can hold
the second model file so the swap itself is fast (usually 3–8 s on NVMe).

**Intel 14th gen CPU:**
Intel Core i7/i9 14th gen CPUs have 8P + 16E cores. For llama.cpp, use only the
P-cores for matrix work — the E-cores are less efficient for FP32/FP16 SIMD:

```bash
llmctl config set server.threads 8    # P-cores only for a Core i9-14900K
```

On Linux you can pin to P-cores with `taskset -c 0-15` (first 16 logical processors
on a 14900K map to P-cores). On Windows, llama.cpp respects thread count but does
not pin automatically.

**Extended context with 32 GB RAM:**

```bash
llmctl config set server.ctx_size 65536   # 64K context — fits with 32 GB RAM
```

With Devstral fully in VRAM and KV cache on the GPU side (4 GB spare), setting
`ctx_size 65536` adds ~1 GB system RAM overhead — well within the 32 GB budget.

---

## Choosing GPU layers

`gpu_layers` (llama.cpp `--n-gpu-layers`) controls how many transformer layers are
kept on GPU. Setting it to `99` offloads everything including the output projection.

| Value | Effect |
|---|---|
| `0` | Full CPU inference |
| `1–N` | First N layers on GPU, rest on CPU (hybrid) |
| `99` | All layers on GPU (use when model fits in VRAM) |

A rule of thumb for partial offload: each layer of a 7B Q4_K_M model uses ~80 MB VRAM.
A 14B model uses ~160 MB per layer. Devstral-Small (MoE) uses ~120 MB per active layer.
Subtract your target KV cache size from available VRAM, then divide by MB-per-layer.

```
Available VRAM: 8 GB = 8192 MB
KV cache (32K ctx, 7B): ~512 MB
Usable for layers: 7680 MB / 80 MB = ~96 layers → --n-gpu-layers 99 (all)

Available VRAM: 8 GB = 8192 MB
KV cache (32K ctx, 13B): ~1024 MB
Usable for layers: 7168 MB / 160 MB = ~44 layers → --n-gpu-layers 40 (partial)
```

---

## Four-way comparison: M5 Pro 24 GB, M1 Pro 16 GB, RTX 3080 8 GB, RTX 4060 8 GB

A fair comparison runs the **same model** on all four machines. Hermes-3-Llama-3.1-8B
Q4_K_M (4.9 GB) fits fully inside the GPU memory of all four — no hybrid mode, no swap,
no PCIe bottleneck. Devstral (13 GB) is also shown to illustrate what happens when a model
exceeds what a machine can hold natively.

### RTX 4060 vs RTX 3080 — same VRAM, very different bandwidth

This is the counter-intuitive result: the 4060 is a newer architecture (Ada Lovelace, 2023)
but is significantly **slower** than the 3080 (Ampere, 2021) for LLM inference. The reason
is the memory bus — NVIDIA narrowed it by half on the budget tier.

| | RTX 3080 laptop 8 GB | RTX 4060 laptop 8 GB |
|---|---|---|
| Architecture | Ampere (GA104) | Ada Lovelace (AD107) |
| VRAM type | **GDDR6X** | GDDR6 |
| Memory bus | **256-bit** | **128-bit** |
| Memory bandwidth | **448 GB/s** | **272 GB/s** |
| TDP range | 80–150 W | 35–115 W |
| VRAM capacity | 8 GB | 8 GB |

For gaming (DLSS 3, shader throughput, rasterisation), the 4060 is competitive because
its Ada compute units and DLSS 3 frame generation compensate. For LLM inference — which is
purely **memory bandwidth bound** — there is no such compensation. Weight loading speed is
directly proportional to bandwidth.

```
Inference bottleneck =  model_size_bytes / memory_bandwidth

Hermes-3 (4.9 GB) on 3080:  4.9 GB / 448 GB/s ≈ 11 ms/token → ~70–90 t/s theoretical
Hermes-3 (4.9 GB) on 4060:  4.9 GB / 272 GB/s ≈ 18 ms/token → ~45–55 t/s theoretical
```

Real throughput is lower due to compute overhead, but the ratio holds.

### Memory fit at a glance

| Machine | GPU memory | Hermes-3 (4.9 GB) | Devstral (13 GB) |
|---|---|---|---|
| RTX 3080 8 GB | 8 GB GDDR6X (448 GB/s) | ✅ Fully in VRAM | ❌ Hybrid — 20 layers on CPU |
| RTX 4060 8 GB | 8 GB GDDR6 (272 GB/s) | ✅ Fully in VRAM | ❌ Hybrid — 20 layers on CPU |
| M1 Pro 16 GB | 16 GB unified (~12 GB working set, 200 GB/s) | ✅ Fully on Metal | ❌ Exceeds working set |
| M5 Pro 24 GB | 24 GB unified (18.2 GB available, ~300 GB/s) | ✅ Fully on Metal | ✅ Fully on Metal |

### Hermes-3 8B Q4_K_M — all four machines (same model, fair comparison)

| | RTX 3080 8 GB | RTX 4060 8 GB | M1 Pro 16 GB | M5 Pro 24 GB |
|---|---|---|---|---|
| Backend | CUDA | CUDA | Metal Apple7 | Metal Apple10 |
| Memory bandwidth | **448 GB/s** | 272 GB/s | 200 GB/s | ~300 GB/s |
| All layers on GPU | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes |
| Token generation | **~50–70 t/s** | ~30–45 t/s | ~30–40 t/s | ~55–75 t/s |
| Prompt prefill | ~150–250 t/s | ~100–160 t/s | ~100–160 t/s | ~200–350 t/s |
| Power draw | ~150–200 W | ~70–110 W | ~15–20 W | ~30–40 W |
| Power per token | ~2–4 W·s | ~1.5–3.5 W·s | **~0.5 W·s** | **~0.5 W·s** |

Key result: the **4060 and M1 Pro land in the same tier** (~30–45 t/s) despite being
completely different architectures. The 3080 is faster than both on Hermes-3 purely
because of its wider memory bus.

### Devstral — only the M5 Pro fits natively

All three 8 GB VRAM and 16 GB unified machines fall back to a slower operating mode.

| | M5 Pro 24 GB | RTX 3080 8 GB | RTX 4060 8 GB | M1 Pro 16 GB |
|---|---|---|---|---|
| Devstral fits natively | ✅ Yes | ❌ Hybrid | ❌ Hybrid | ❌ macOS swap |
| Token generation | **52–57 t/s** (measured) | ~12–18 t/s | ~12–18 t/s | ~3–10 t/s |
| Bottleneck | — | PCIe 32 GB/s | PCIe 32 GB/s | SSD swap |

Both 3080 and 4060 are **equal** in Devstral hybrid mode because the bottleneck shifts
from VRAM bandwidth to PCIe bandwidth — and both cards use PCIe 4.0 x16 (32 GB/s).

### Architecture diagram

```
  Memory bandwidth (GB/s) — higher = faster for inference

  448 │  ███ RTX 3080 8 GB (GDDR6X, 256-bit)
      │
  300 │      ████████ M5 Pro 24 GB (unified)
      │
  272 │          ██████ RTX 4060 8 GB (GDDR6, 128-bit)
      │
  200 │               ████████ M1 Pro 16 GB (unified)
      │
    0 └──────────────────────────────────────────────

  For LLM inference: bandwidth ≈ tokens/s (when model fits in GPU memory)
  For Devstral on 8 GB VRAM: all GPU bandwidth is irrelevant — PCIe (32 GB/s) wins
```

### Summary

- **3080 vs 4060 (same model)**: 3080 is ~40–60% faster. Newer generation does not mean
  faster for inference — the 128-bit bus on the 4060 is a real handicap.
- **4060 vs M1 Pro**: roughly equal throughput on Hermes-3. The 4060 uses 3–5× more power.
- **Devstral on any 8 GB VRAM card**: 3080 and 4060 are identical — both hit the PCIe ceiling.
- **M5 Pro 24 GB**: the only laptop-class machine where both Devstral and speed are possible together.
- **Choosing between 3080 and 4060 for local LLM use**: the 3080 is the better inference card.
  The 4060's advantages (DLSS 3, lower TDP) are irrelevant to this workload.

```bash
# 3080, 4060, or M1 Pro 16 GB — use Hermes-3
llmctl model install Hermes-3-Llama-3.1-8B
llmctl config set server.gpu_layers 99
llmctl config set server.ctx_size 32768

# M5 Pro 24 GB — run Devstral natively
llmctl model install Devstral-Small
llmctl config set server.gpu_layers 99
llmctl config set server.ctx_size 32768
```

---

## Devstral on M5 Pro 24 GB vs M1 Pro 16 GB — Apple Silicon comparison

Both machines share the same unified memory architecture, same Metal backend, and same
llama.cpp code path. The difference is that Devstral-Small Q4_K_M is **13 GB** and the
M1 Pro only has **16 GB total** — leaving almost nothing for the OS once the model loads.

### The memory ceiling problem on M1 Pro 16 GB

macOS Metal enforces a `recommendedMaxWorkingSetSize` — the safe upper limit for GPU
allocations before the OS starts compressing memory and paging data to SSD.

| Machine | Total memory | recommendedMaxWorkingSetSize | Devstral (13 GB) fits? |
|---|---|---|---|
| M5 Pro 24 GB | 24 GB | **19.1 GB** (measured) | ✅ Yes — 6 GB to spare |
| M1 Pro 16 GB | 16 GB | **~12 GB** (75% of 16 GB) | ❌ No — 1 GB over limit |

When a Metal allocation exceeds `recommendedMaxWorkingSetSize`, macOS does not hard-fail —
it starts silently evicting GPU pages to the SSD-backed swap file. Apple's NVMe is fast
(~5 GB/s read) but it is still **40–60× slower than unified memory bandwidth**. Every
evicted page that is needed for a matrix multiply causes a stall.

In practice on M1 Pro 16 GB with Devstral:
- The model may load but macOS will immediately show high memory pressure (red bar in Activity Monitor).
- Generation speed drops to **3–10 tokens/s** as GPU pages are swapped in and out mid-inference.
- The machine becomes sluggish for all other applications while inference is running.
- If any other application allocates significant memory, the model may be evicted entirely.

### Memory budget breakdown

```
M1 Pro 16 GB                          M5 Pro 24 GB
─────────────────────────────────────  ─────────────────────────────────────
macOS + processes:     ~3–4 GB         macOS + processes:     ~3–4 GB
Devstral Q4_K_M:       13.0 GB         Devstral Q4_K_M:       13.0 GB
KV cache (32K ctx):     ~0.5 GB        KV cache (32K ctx):     ~0.5 GB
─────────────────────                  ─────────────────────
Total needed:          ~16.5–17.5 GB   Total needed:          ~16.5–17.5 GB
Total available:       16 GB           Total available:        24 GB
                       ─────                                   ─────
Headroom:              -0.5 to -1.5 GB Headroom:              +6.5–7.5 GB ✅
```

### Performance comparison

| | M5 Pro 24 GB | M1 Pro 16 GB |
|---|---|---|
| Memory bandwidth | ~300 GB/s | 200 GB/s |
| GPU cores | 20-core | 16-core |
| GPU family | Apple10 (M5) | Apple7 (M1) |
| Devstral fits in recommended working set | ✅ Yes | ❌ No — exceeds by ~1 GB |
| Prompt prefill | **235–387 t/s** (measured) | ~5–20 t/s (swap-limited) |
| Token generation | **52–57 t/s** (measured) | ~3–10 t/s (swap-limited) |
| macOS memory pressure | None | High (red) |
| Context 32 K comfortable | ✅ Yes | ⚠️ No — worsens swap |

### What to run on M1 Pro 16 GB instead

The M1 Pro 16 GB is well-matched to 7–8B models, which fit entirely within the
recommended working set with room for the OS and a generous KV cache.

| Model | Size | Expected t/s on M1 Pro 16 GB | Notes |
|---|---|---|---|
| Hermes-3-Llama-3.1-8B Q4_K_M | 4.9 GB | **30–40 t/s** | Best agentic 8B, native tool_calls |
| Qwen3-8B Q4_K_M | 5.2 GB | 28–38 t/s | Reasoning + tools |
| Qwen2.5-Coder-7B Q4_K_M | 4.7 GB | 32–42 t/s | Strong coder |
| Qwen2.5-Coder-14B Q4_K_M | 8.9 GB | 15–22 t/s | Fits with ~3 GB headroom |

```bash
# M1 Pro 16 GB — recommended setup
llmctl model install Hermes-3-Llama-3.1-8B
llmctl config set server.gpu_layers 99
llmctl config set server.ctx_size 32768
llmctl server start
```

### Summary: when to upgrade from M1 Pro 16 GB

The M1 Pro 16 GB is not underpowered — it is the wrong memory size for models above ~10 GB.
The M5 Pro 24 GB resolves this completely: the extra 8 GB shifts Devstral from
"memory pressure territory" to "fits with 6 GB to spare." The bandwidth jump (200 → 300 GB/s)
and newer GPU architecture add another ~40–50% speed uplift on top.

If you are on M1 Pro 16 GB and doing agentic coding work, Hermes-3 8B is the correct
Devstral substitute — same OpenAI tool_calls JSON format, same `--jinja` flag, roughly
comparable code quality at 8B scale, and it runs fast and stable within 16 GB.

---

## llmctl config quick reference

```bash
llmctl config set server.gpu_layers <N>    # 0 = CPU, 99 = all on GPU
llmctl config set server.threads <N>       # CPU threads (physical cores)
llmctl config set server.ctx_size <N>      # context window (tokens)
llmctl config mode single|cold-swap|hot-swap|parallel
llmctl server restart                      # apply changes
```
