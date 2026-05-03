# Temperature and Sampling Parameters

## What Temperature Does

When a language model generates text, at each step it produces a probability
distribution over all tokens in its vocabulary. Temperature is a scaling factor
applied to that distribution before sampling.

- **Low temperature (< 1.0)**: sharpens the distribution — the most probable tokens
  become relatively more likely. Output is more predictable and consistent.
- **Temperature = 1.0**: the distribution is unchanged. Output matches the model's
  trained behaviour.
- **High temperature (> 1.0)**: flattens the distribution — lower-probability tokens
  become more likely. Output becomes more varied and unexpected.

Temperature does not affect the model's knowledge — it only controls how the model
samples from what it knows.

## Recommended Values

| Value | Behaviour | Best for |
|---|---|---|
| `0.0` | Deterministic (greedy decoding) | Code generation, structured output, fact lookup |
| `0.2–0.3` | Very focused, near-deterministic | Code review, technical writing, data extraction |
| `0.5–0.7` | Balanced (default: 0.7) | General chat, Q&A, summarisation |
| `0.8–1.0` | Creative, varied | Brainstorming, storytelling, diverse outputs |
| `1.0+` | Experimental / very random | Creative writing, avoiding repetition |

The llmctl default is **0.7**. For code generation tasks, consider dropping to 0.0–0.2
for reproducible output.

## Configuring Temperature

### Persistent (config file)

```bash
llmctl config set server.temp 0.2
```

This sets the default for all requests to the server. Restart the server to apply.

### Per-session (API request)

Pass `temperature` in the request body:

```bash
curl http://localhost:8765/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5-coder-7b",
    "temperature": 0.0,
    "messages": [{"role": "user", "content": "Write a binary search in Go"}]
  }'
```

The per-request value overrides the server default for that request only.

## Related Sampling Parameters

### Top-P (nucleus sampling)

Limits sampling to the smallest set of tokens whose cumulative probability exceeds P.
For example, `top_p=0.9` means: find the tokens that together cover 90% of the
probability mass, then sample only from those.

- **Default**: 0.95 (liberal)
- **Low (0.5–0.7)**: more focused, similar effect to lower temperature
- **High (0.95–1.0)**: near-unrestricted

Top-P and temperature interact: both narrow the candidate pool, so using both at
low values simultaneously can make output very repetitive. Use one or the other as
your primary control.

### Top-K

Limits sampling to the K most probable tokens at each step.

- **Default**: 40 (reasonable for most cases)
- **Low (1–5)**: very deterministic, similar to greedy
- **High (100+)**: similar to no restriction

`top_k=1` is equivalent to greedy decoding. Most users should leave this at the default.

### Repeat Penalty

Discourages the model from repeating tokens that have already appeared in the output
or context. A value of 1.0 applies no penalty; values > 1.0 reduce repetition.

- **Default**: 1.0 (no penalty)
- **Useful range**: 1.05–1.2

Increase `repeat_penalty` if the model gets stuck in loops or repeats phrases.
Values above 1.3 can make output incoherent.

## Setting Sampling Parameters via API

All standard llama.cpp sampling parameters are supported:

```json
{
  "model": "qwen7b",
  "temperature": 0.3,
  "top_p": 0.9,
  "top_k": 40,
  "repeat_penalty": 1.1,
  "messages": [{"role": "user", "content": "..."}]
}
```
