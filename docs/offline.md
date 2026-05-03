# Offline Bundles

## When to Use Offline Bundles

Offline bundles let you package one or more models (and optionally the llmctl binary)
into a single `.tar.gz` for deployment in environments without internet access:

- **Air-gapped servers**: secure networks where outbound internet is blocked
- **VPN-only deployments**: sites where HuggingFace is unreachable
- **Fleet provisioning**: ship one bundle to many machines via Ansible, Salt, or Puppet
- **Reproducible environments**: lock the exact model file and version used in production
- **Offline laptops**: pre-download for travel without reliable connectivity

## Creating a Bundle

Bundles are created from models that are already installed in the local registry.
Install the model first, then bundle it.

```bash
# Install the model locally
llmctl model install Qwen2.5-Coder-7B

# Create a bundle containing one model
llmctl bundle create Qwen2.5-Coder-7B --output qwen7b-bundle.tar.gz

# Bundle multiple models
llmctl bundle create "Qwen2.5-Coder-7B,DeepSeek-R1-7B" \
  --output coding-bundle.tar.gz

# Include the llmctl binary itself (for bootstrapping a new machine)
llmctl bundle create Qwen2.5-Coder-7B \
  --include-binary \
  --output llmctl-full-bundle.tar.gz
```

The `--include-binary` flag copies the running `llmctl` binary into the bundle,
so the target machine does not need internet access to install llmctl itself.

## What's Inside a Bundle

A bundle is a `.tar.gz` archive with the following structure:

```
bundle/
  bundle-manifest.json     metadata: model aliases, versions, file names
  llmctl                   binary (only if --include-binary was used)
  install.sh               optional installer script
  <model-filename>.gguf    one file per bundled model
```

`bundle-manifest.json` example:

```json
{
  "version": "1",
  "created_at": "2026-05-03T12:00:00Z",
  "models": [
    {
      "alias": "Qwen2.5-Coder-7B",
      "repo": "unsloth/Qwen2.5-Coder-7B-Instruct-GGUF",
      "quant": "Q4_K_M",
      "file_name": "unsloth-Qwen2.5-Coder-7B-Instruct-GGUF-Q4_K_M.gguf",
      "size_bytes": 4680000000
    }
  ]
}
```

## Deploying a Bundle

Copy the bundle to the target machine and run:

```bash
# Copy bundle (e.g. via scp, Ansible copy, USB)
scp qwen7b-bundle.tar.gz user@target:/tmp/

# On the target machine:
llmctl bundle install /tmp/qwen7b-bundle.tar.gz
```

The install command:
1. Extracts the archive to `~/.local/share/llmctl/bundles/`
2. Reads `bundle-manifest.json`
3. Copies model files to `~/.local/share/llmctl/models/`
4. Registers each model in the local registry

If `install.sh` is present in the bundle, it is run with `OFFLINE=1` to
skip any download steps.

## Verifying a Bundle

After installation, verify the models are registered:

```bash
llmctl model list
```

Check the model file is accessible:

```bash
llmctl model info Qwen2.5-Coder-7B
```

Start the server with the installed model:

```bash
llmctl config set server.model Qwen2.5-Coder-7B
llmctl server start
```

## Bundle Size

Bundle sizes are dominated by the model files:

| Model | Quant | Bundle size |
|---|---|---|
| Qwen2.5-Coder-7B | Q4_K_M | ~4.4 GB |
| Qwen3-8B | Q4_K_M | ~5.2 GB |
| DeepSeek-R1-7B | Q4_K_M | ~4.7 GB |
| llmctl binary | — | ~15 MB |

Plan storage accordingly. The bundle is uncompressed during extraction, so you
need approximately 2× the bundle size available during the install step.
