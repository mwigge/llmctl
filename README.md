# llmctl — local LLM management

Manage local AI models for your team. Install, configure, and run llama.cpp models
with a single CLI. OpenAI-compatible API — any client that speaks `/v1` works.

## Quick Start

```bash
# Install
curl -sSf https://github.com/mwigge/llmctl/releases/latest/download/install.sh | bash

# Download the default model — Devstral-Small (12 GB, needs 16 GB RAM)
llmctl model install Devstral-Small

# Or a lighter 8 GB model for machines with less RAM
llmctl model install Hermes-3-Llama-3.1-8B

# Install the inference server
llmctl server install

# Start
llmctl server start

# Now point any OpenAI client at http://localhost:8765/v1
```

## Deployment Modes

Choose a deployment topology with `llmctl config mode <mode>`:

### single — one model, minimum RAM

```
client
  |
  v
llama-server :8765
  |
  v
model.gguf (~4-9 GB RAM)
```

### cold-swap — two models, one server, swapped on demand

```
client
  |
  v
llama-swap :8765
  |
  v
llama-server :8766   <-- loads model A or model B (TTL-based eviction)
```

RAM = largest model only. 5–15 s cold-start on model switch.

### hot-swap — two models, both loaded, routed by hint

```
client (X-Model: reason OR X-Model: code)
  |
  v
llama-swap :8765
  |         |
  v         v
server    server
:8766     :8767
reason    code
```

RAM = sum of both models. Zero cold-start latency.

### parallel — two servers, side by side

```
                    server :8765  (model 0)
client ---------->
                    server :8766  (model 1)
```

No router overhead. Clients choose port directly.

## Installation

**Linux (recommended)**

```bash
curl -sSf https://github.com/mwigge/llmctl/releases/latest/download/install.sh | bash
```

**From package** (Ubuntu/Debian):

```bash
curl -L https://github.com/mwigge/llmctl/releases/latest/download/llmctl_amd64.deb -o llmctl.deb
sudo apt install ./llmctl.deb
```

**From package** (Fedora/RHEL):

```bash
curl -L https://github.com/mwigge/llmctl/releases/latest/download/llmctl-amd64.rpm -o llmctl.rpm
sudo dnf install ./llmctl.rpm
```

**Build from source**:

```bash
git clone https://github.com/mwigge/llmctl
cd llmctl
CGO_ENABLED=1 go build -o ~/.local/bin/llmctl ./cmd/llmctl
```

## Model Management

See [docs/tool-formats.md](docs/tool-formats.md) for a guide on OpenAI vs XML tool call formats and how to choose a model for agentic use.

```bash
# Show the builtin catalog
llmctl model catalog

# Install a model from the catalog
llmctl model install Qwen2.5-Coder-7B

# Install a specific quantisation
llmctl model install Qwen2.5-Coder-7B --quant Q8_0

# List installed models
llmctl model list

# Search HuggingFace for GGUF models
llmctl model search "phi-4 gguf"
```

## Server Management

```bash
# Start (uses config in ~/.config/llmctl/config.yaml)
llmctl server start

# Check status
llmctl server status

# Stop
llmctl server stop

# Install as systemd user service
llmctl server service install
systemctl --user enable --now llmctl-server
```

## Configuration

```bash
# Initialise default config
llmctl config init

# Show effective config
llmctl config show

# Change a value
llmctl config set server.port 9000
llmctl config set server.ctx_size 32768
llmctl config set server.gpu_layers 32
llmctl config set server.temp 0.2

# Switch deployment mode
llmctl config mode hot-swap
```

## Offline Bundles

For air-gapped or VPN-only deployments:

```bash
# Create a bundle on an internet-connected machine
llmctl bundle create Qwen2.5-Coder-7B --output qwen7b-bundle.tar.gz

# Copy to the target machine and install
scp qwen7b-bundle.tar.gz user@target:/tmp/
ssh user@target -- llmctl bundle install /tmp/qwen7b-bundle.tar.gz
```

See [docs/offline.md](docs/offline.md) for details.

## Observability

llmctl records every inference (model, token counts, latency, cost) in a local
SQLite database.

```bash
# Show metrics table
llmctl metrics show

# Daily summary (last 7 days)
llmctl metrics summary

# Export to CSV
llmctl metrics export > metrics.csv
```

For OpenTelemetry export (e.g. to Grafana / Tempo):

```bash
llmctl config set otel.endpoint http://localhost:4318
llmctl config set otel.service_name my-llm-service
```

## European Models — Devstral and Mistral AI

The default model is **Devstral-Small-2505**, developed by [Mistral AI](https://mistral.ai) in France.

### Why European models

- **Data sovereignty**: open-weight models run entirely on your hardware — no data leaves your infrastructure.
- **GDPR compliance**: self-hosted inference has no third-party data processor, no retention, no telemetry.
- **License clarity**: Devstral-Small is Apache 2.0 — unrestricted commercial use, no usage-based fees.
- **EU AI Act alignment**: Mistral AI is a French company subject to EU regulation, with transparent training practices.

### Devstral-Small-2505

| Property | Value |
|---|---|
| Developer | Mistral AI (Paris, France) |
| License | Apache 2.0 |
| Parameters | ~24B (MoE, small active parameter count) |
| GGUF size | ~12 GB (Q4_K_M) |
| RAM required | 16 GB minimum |
| Tool calling | Native OpenAI `tool_calls` JSON (needs `--jinja`) |
| SWE-bench Verified | 46.8% — top in its class at time of release |

```bash
llmctl model install Devstral-Small
llmctl server start
```

Devstral is fine-tuned for software engineering tasks: file editing, code generation, repository navigation, and agentic tool loops. The `--jinja` flag is already included in the llmctl server launcher by default.

### Other Mistral models

| Model | Size | Notes |
|---|---|---|
| Devstral-Small-2505 | 12 GB | Default. Coding specialist, Apache 2.0 |
| Mistral-7B-v0.3 | 4.1 GB | General instruction model, lighter RAM footprint |

### Lighter alternatives (non-EU, 8 GB RAM)

If you are on a machine with less than 16 GB RAM and cannot run Devstral, these are the next best options for agentic use:

```bash
# 8 GB RAM — Hermes-3 (NousResearch) — native OpenAI tool_calls
llmctl model install Hermes-3-Llama-3.1-8B

# 8 GB RAM — Qwen3-8B (Alibaba) — tool-use + chain-of-thought reasoning  
llmctl model install Qwen3-8B
```

See [docs/models.md](docs/models.md) for the full catalog and RAM requirements.

## Documentation

| Doc | Contents |
|---|---|
| [docs/local-server.md](docs/local-server.md) | Deployment modes, systemd, ports, monitoring |
| [docs/models.md](docs/models.md) | GGUF format, model selection |
| [docs/quantisation.md](docs/quantisation.md) | Quantisation levels, Unsloth, how to choose Q4_K_M vs Q8_0 |
| [docs/tool-formats.md](docs/tool-formats.md) | OpenAI vs XML tool call formats — how to pick a model for agentic use |
| [docs/temperature.md](docs/temperature.md) | Sampling parameters: temperature, top-p, top-k |
| [docs/tuning.md](docs/tuning.md) | Performance tuning: threads, GPU layers, batch size |
| [docs/offline.md](docs/offline.md) | Offline bundles for air-gapped deployments |

## License

MIT
