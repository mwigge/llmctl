# Local Server

llmctl manages a local instance of [llama-server](https://github.com/ggml-org/llama.cpp), the high-performance
inference server from the llama.cpp project.

## Why llama-server?

llama-server is the right foundation for local LLM serving because:

- **C++ kernels**: Metal on Apple Silicon, CUDA on NVIDIA, AVX2/AVX-512 SIMD on CPUs —
  each backend is hand-optimised. Python alternatives (vLLM, transformers) add a Python
  interpreter between the weights and the compute.
- **OpenAI-compatible API**: exposes `/v1/chat/completions`, `/v1/completions`,
  `/v1/models`, and `/v1/embeddings`. Any client that speaks OpenAI works without changes.
- **Cross-platform**: the same binary runs on Linux (x86-64, arm64), macOS, and Windows.
- **Single binary**: no dependency hell, no virtualenv, no CUDA toolkit version matching.

## Deployment Modes

llmctl supports four deployment modes, configured via `llmctl config mode <mode>`.

### single (default)

One model. One server. Minimum RAM.

```
client request
     |
     v
llama-server :8765
     |
     v
  model.gguf
```

Use when you have one primary model and want the lowest resource footprint.
RAM required = model size × (1 + overhead). For Q4_K_M at 4B params: ~3 GB.

### cold-swap

Two models. One server. Models are swapped on demand (unloaded, new model loaded).

```
client request
     |
     v
llama-swap :8765   <-- routes to active model
     |
     v
llama-server :8766 (reason model)  OR  llama-server :8766 (code model)
```

The swap manager uses a TTL: after N seconds of inactivity the current model is
unloaded and the next request triggers a reload (~5–15 s cold start).

Good for machines with 8–16 GB RAM that need two models but cannot run both simultaneously.

### hot-swap

Two models. Two servers. Both loaded simultaneously. No cold-start delay.

```
client request  (header: X-Model: reason)
     |
     v
llama-swap :8765   <-- routes by model hint
     |          |
     v          v
server :8766   server :8767
 reason model   code model
```

RAM required = sum of both model sizes. For two 7B Q4_K_M models: ~8 GB.
Requests are routed by the `X-Model` header or the `model` field in the request body.

### parallel

Two models. Two servers. Side-by-side on adjacent ports. No router — clients
choose the port directly.

```
                 server :8765 (model 0)
client -------->
                 server :8766 (model 1)
```

Use when you have enough RAM and want maximum throughput with no routing overhead.
Each server handles its queue independently.

## systemd Integration

On Linux, llmctl can install a systemd user unit to auto-start the server on login.

```bash
# Install the unit
llmctl server service install

# Enable auto-start
systemctl --user enable llmctl-server
systemctl --user start llmctl-server

# Check status
systemctl --user status llmctl-server
```

The unit is installed to `~/.config/systemd/user/llmctl-server.service`.

To remove it:

```bash
systemctl --user disable --now llmctl-server
llmctl server service remove
```

## Port Management

Default port: **8765**. Change it with:

```bash
llmctl config set server.port 9000
```

In hot-swap and parallel modes, llmctl automatically allocates `port+1` for the
second server. Ensure both ports are open if you use a host firewall.

Firewall example (UFW):

```bash
sudo ufw allow 8765/tcp comment "llmctl model server"
sudo ufw allow 8766/tcp comment "llmctl model server (secondary)"
```

## Monitoring Endpoints

llama-server exposes these HTTP endpoints:

| Endpoint | Description |
|---|---|
| `GET /health` | Returns `{"status":"ok"}` when the server is healthy |
| `GET /metrics` | Prometheus-format metrics (tokens/s, queue depth, latency) |
| `GET /v1/models` | List loaded models |
| `GET /props` | Server properties and compile-time settings |

Example health check:

```bash
curl -s http://localhost:8765/health | jq .
```

Example Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: llmctl
    static_configs:
      - targets: ['localhost:8765']
```

llmctl itself exports OpenTelemetry metrics (token counts, latency, cost) when
`otel.endpoint` is configured. See `llmctl config show` for current settings.
