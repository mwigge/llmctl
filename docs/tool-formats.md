# Tool Call Formats in Local LLMs

Local language models emit tool calls in fundamentally different ways depending on
how they were trained and how the inference server is configured. Getting this wrong
causes tool calls to appear as raw text in chat output rather than executing — a
silent failure that is easy to miss and time-consuming to debug.

This document explains the two dominant formats, why they diverge, how llmctl
handles the difference transparently, and how to verify that your setup is working.

---

## 1. The Two Tool-Call Formats

### OpenAI JSON format (native, no translation needed)

The OpenAI function-calling protocol returns tool invocations as a structured field
in the response object, separate from the natural-language content:

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "choices": [{
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [{
        "id": "call_abc123",
        "type": "function",
        "function": {
          "name": "bash",
          "arguments": "{\"command\": \"ls /tmp\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

Key properties:
- `content` is `null` when the model is calling a tool.
- `tool_calls` is a JSON array under `message`, parsed by every OpenAI-compatible
  client without any extra work.
- `arguments` is a JSON-encoded string (not an object) — double-serialised by spec.
- `finish_reason` is `"tool_calls"` rather than `"stop"`.

**Models that emit this format natively:**

| Model family | Notes |
|---|---|
| Llama-3.x (Meta) | Llama-3.1-8B, Llama-3.2-3B, Llama-3.3-70B |
| Hermes-3 (NousResearch) | Hermes-3-Llama-3.1-8B, Hermes-3-Llama-3.1-70B |
| Mistral v0.3+ | mistral-7b-instruct-v0.3, Mixtral-8x7B-v0.1 |
| Phi-3.5 (Microsoft) | Phi-3.5-mini-instruct (2.2 GB — smallest capable model) |
| Functionary family | meetkai/functionary-* |

These models have tool-call formatting built into their chat templates and, when
served with `--jinja`, return structured `tool_calls` directly. No client-side
parsing is required.

---

### Devstral / Mistral XML format (user/assistant only — no tool role)

Devstral (Mistral AI) uses a completely different tool-calling contract from both
the OpenAI JSON format and Qwen's XML format. It operates on **user and assistant
messages only** — there is no `tool` role in the conversation. Tool definitions go
into the system prompt as XML, and tool results come back as user messages.

**Why:** Devstral was fine-tuned with this format because it gave better agent
performance than the standard OpenAI tool_calls approach. See the
[Mistral blog](https://docs.mistral.ai/mistral-vibe/local) for background.

#### Tool call emitted by Devstral

```
finish_reason: "stop"   ← always "stop", never "tool_calls"
content:
  <tool_call>
  {"name": "bash", "arguments": {"command": "ls /tmp"}}
  </tool_call>
```

The `<tool_call>` block appears in `content` as plain text. There is no
`tool_calls` array in the response.

#### Full conversation structure

```
[system]    You are a coding assistant.

            Available tools:
            <tools>
            <tool>
            <name>bash</name>
            <description>Execute a shell command</description>
            <parameters_schema>{"type":"object","properties":{"command":{"type":"string"}}}</parameters_schema>
            </tool>
            </tools>

            When you need to call a tool, output ONLY:
            <tool_call>
            {"name": "tool_name", "arguments": {"arg": "value"}}
            </tool_call>

[user]      list files in /tmp

[assistant] <tool_call>
            {"name": "bash", "arguments": {"command": "ls /tmp"}}
            </tool_call>

[user]      <tool_results>
            [{"name": "bash", "output": "file1.txt\nfile2.txt\n"}]
            </tool_results>

[assistant] The directory contains: file1.txt, file2.txt.
```

Key differences from OpenAI format:

| Property | OpenAI format | Devstral XML format |
|---|---|---|
| Tool defs location | API `tools` field | System prompt (`<tools>` XML) |
| Tool call location | `message.tool_calls` array | `message.content` text |
| finish_reason | `"tool_calls"` | `"stop"` |
| Tool result role | `"tool"` | `"user"` |
| Tool result format | `{"role":"tool","content":"..."}` | `<tool_results>[{"name":"...","output":"..."}]</tool_results>` |

#### llmctl handling

When the configured model alias contains `devstral` or `mistral`, llmctl:
- Skips `tool_choice` injection (irrelevant to this format)
- Clients using llmctl's proxy should embed tool definitions in the system prompt
  and send only user/assistant messages

---

### Qwen XML format (requires client-side parsing)

Qwen models emit tool calls inline with the response text using an XML envelope.
The tool call appears in the `content` field as a plain string:

```xml
<function_call>{"name": "bash", "arguments": {"command": "ls /tmp"}}</function_call>
```

Full response example:

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "<function_call>{\"name\": \"bash\", \"arguments\": {\"command\": \"ls /tmp\"}}</function_call>",
      "tool_calls": null
    },
    "finish_reason": "stop"
  }]
}
```

Key properties:
- `content` is a non-null string containing the XML tag.
- `tool_calls` is `null` — the call is not in the structured field.
- The JSON payload inside the tag uses `arguments` as an object, not a double-encoded
  string (unlike the OpenAI spec).
- `finish_reason` is `"stop"`, not `"tool_calls"`.

**Models that emit this format:**

| Model family | Notes |
|---|---|
| Qwen2.5-Coder | Qwen2.5-Coder-7B-Instruct, Qwen2.5-Coder-14B-Instruct |
| Qwen3 family | Qwen3-8B, Qwen3-14B, Qwen3-32B |

---

### Hermes/Nous text format (fallback when `--jinja` is absent)

NousResearch Hermes models use a third variant when llama-server is launched
without the `--jinja` flag. Instead of the `<function_call>` tag, they emit:

```xml
<tool_call>{"name": "bash", "arguments": {"command": "ls /tmp"}}</tool_call>
```

This format is structurally identical to the Qwen format except for the tag name.
It appears in `content`, not `tool_calls`, and requires the same client-side
parsing logic.

The distinction matters for parser configuration: a router that only detects
`<function_call>` will silently drop Hermes tool calls when `--jinja` is missing.

---

## 2. Why the Format Matters

### Structured field vs. free text

An OpenAI-compatible client — including the Go `openai` package, Python `openai`
SDK, or any chat UI — checks `message.tool_calls` for structured tool invocations.
When this field is populated, the client routes the response to tool-execution
logic. When it is `null` or absent, the client treats the message as plain
assistant text and displays it to the user.

This means:

- **OpenAI JSON format models**: the structured `tool_calls` field is populated
  by llama-server directly. Clients work without modification.
- **XML format models**: `tool_calls` is always `null`. Unless something in the
  path detects the XML tags in `content` and rewrites the response, tool calls
  appear verbatim in the chat window instead of executing.

### The silent failure mode

When format detection is missing or misconfigured, you will see output like:

```
Assistant: <function_call>{"name": "bash", "arguments": {"command": "ls /tmp"}}</function_call>
```

The model did the right thing — it decided to call a tool. The failure is in the
delivery layer. This is one of the most common misconfigurations when running local
models and is often mistaken for the model being incapable of tool use.

### finish_reason mismatch

Some clients gate tool-execution logic on `finish_reason == "tool_calls"`. XML
format models emit `"stop"` instead, which causes those clients to skip execution
entirely even if someone manually parses `content`. Always test with both fields.

---

## 3. The `--jinja` Flag

### What it does

llama-server (the binary shipped with llama.cpp) includes a Jinja2 template engine
that can process the chat template embedded in each GGUF file. The template is
stored in the model's `tokenizer_config.json` under the key
`chat_template`. It is a Jinja2 program that maps the OpenAI messages array —
including the `tools` parameter — into the exact token sequence the model was
trained to expect.

The `--jinja` flag activates this engine. Without it:

- llama-server uses a simplified built-in formatter that ignores the `tools`
  parameter entirely.
- The model never sees any tool definitions.
- The model cannot emit tool calls because it has no knowledge that tools exist.

### Before and after

**Request — `--jinja` absent:**

```bash
# Server started as: llama-server --model hermes-3-llama-3.1-8b.gguf
#
# The tools array is silently dropped.

curl -s http://localhost:8765/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "list files in /tmp"}],
    "tools": [{"type": "function", "function": {
      "name": "bash",
      "description": "run a shell command",
      "parameters": {"type": "object",
        "properties": {"command": {"type": "string"}},
        "required": ["command"]}
    }}]
  }'
```

Response:

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "To list files in /tmp, run:\n\n```bash\nls /tmp\n```"
    },
    "finish_reason": "stop"
  }]
}
```

The model answered as a chat message. It had no idea it was supposed to call a tool.

**Request — `--jinja` present:**

```bash
# Server started as: llama-server --model hermes-3-llama-3.1-8b.gguf --jinja
```

Same request body. Response:

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [{
        "id": "call_0",
        "type": "function",
        "function": {
          "name": "bash",
          "arguments": "{\"command\": \"ls /tmp\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

The Jinja2 template injected the tool schema into the prompt. The model decided to
call `bash`. llama-server parsed the model's output back into the structured
`tool_calls` field.

### Template roundtrip

With `--jinja`, llama-server performs a full roundtrip:

1. **Render**: the `tools` array is formatted by the chat template into the
   token stream sent to the model (e.g. a `<|im_start|>tools` section for Hermes,
   a `[AVAILABLE_TOOLS]` block for Mistral, a `<tools>` XML block for Qwen).
2. **Decode**: the model's output is matched against the template's expected
   tool-call format and, if matched, rewritten into OpenAI-compatible `tool_calls`.

This means that even XML-format models can produce structured `tool_calls` when
served with `--jinja` and a template that handles the roundtrip. Whether they do
depends on the quality of the template baked into the GGUF.

---

## 4. How llmctl Handles Both

llmctl removes the need to think about format differences for most workflows.

### `--jinja` by default

All server launchers include `--jinja` in the generated llama-server command line.
You can verify this:

```bash
llmctl server status --verbose
```

The `Server command` line in the output will include `--jinja`.

### XML content parser

For models whose chat templates do not perform a full roundtrip (i.e. they still
emit XML in `content` even with `--jinja`), llmctl's HTTP router intercepts
responses before returning them to the client. It scans `content` for both
`<tool_call>` and `<function_call>` envelopes and rewrites the response into the
structured OpenAI format:

- `content` is set to `null`.
- `tool_calls` is populated with the parsed call.
- `finish_reason` is rewritten to `"tool_calls"`.
- The inner `arguments` object is re-serialised as a JSON string to match the
  OpenAI double-encoding convention.

This parser runs on every response, so XML-format models (Qwen2.5-Coder, Qwen3)
are transparent to any standard OpenAI client.

### Automatic `tool_choice` injection

When a request includes a non-empty `tools` array and `tool_choice` is absent,
llmctl injects `"tool_choice": "auto"` before forwarding to llama-server. Some
model templates require this field to activate tool-call mode; omitting it causes
the model to answer in chat mode even when tools are available.

### Usage tracking

llmctl sets `"stream_options": {"include_usage": true}` on all streaming requests.
This ensures the final chunk carries token counts, which are written to the local
metrics database regardless of whether the response contained tool calls.

---

## 5. Choosing a Model for Tool Use

| Need | Recommended model | Reason |
|------|------------------|--------|
| Best agentic tool use | Hermes-3-Llama-3.1-8B | Native OpenAI format, trained specifically for multi-step agentic tasks |
| Best coding + tool use | Llama-3.1-8B | Native OpenAI format, solid code quality, 4.9 GB |
| Best code quality (larger) | Qwen2.5-Coder-14B | XML format (parsed automatically by llmctl), best-in-class code generation |
| Reasoning + tools | Qwen3-8B | XML format, hybrid think/chat mode via `/no_think` suffix |
| Smallest capable | Phi-3.5-mini (2.2 GB) | Native OpenAI format, works on 4 GB machines |
| No tool use (chat only) | Any model | Tool format irrelevant; pick by capability and size |

### Native vs. parsed

"Native OpenAI format" means the model produces structured `tool_calls` directly
and no XML parsing step is needed. This is marginally more reliable because there
is no risk of the content parser matching the wrong tags.

"XML format (parsed automatically)" means llmctl's content parser is doing real
work on every tool response. For most workloads this is transparent, but if you
are building a client that bypasses llmctl and talks to llama-server directly, you
will need to implement your own XML parser.

### Context window and tool use reliability

Tool-heavy agentic tasks involve long tool-result chains that grow the context
quickly. Models with a context window under 16K tokens tend to lose track of
earlier tool calls as the context fills. Use `--ctx-size 32768` or higher for
multi-step tasks:

```bash
llmctl config set server.ctx_size 32768
```

---

## 6. Troubleshooting

### Tool calls appear as text in the response

**Symptom**: The assistant output contains a raw `<tool_call>` or
`<function_call>` string, or a JSON blob that looks like a function invocation,
rather than executing the tool.

**Diagnosis**: `--jinja` is missing from the server launch command, and the
content parser is not running (i.e. you are talking to llama-server directly, not
through llmctl's router).

**Fix**: Restart the server with `--jinja`. If using llmctl, run
`llmctl server restart` and verify with `llmctl server status --verbose` that
`--jinja` appears in the command line.

---

### Tool calls execute but results are wrong

**Symptom**: The tool executes, but the arguments are garbled or the wrong tool
is called.

**Diagnosis**: The XML content parser matched the wrong tags. This can happen if
the model emits partial or malformed XML, or if a model-specific tag variant is
not in the parser's allowlist.

**Fix**: Check the raw response body before parsing using the debug flag:

```bash
LLMCTL_DEBUG_TOOL_PARSE=1 llmctl server start
```

Inspect the logged raw `content` strings to identify the exact tag the model
is emitting and report it as a parser gap.

---

### Model ignores tools completely

**Symptom**: The model responds in chat mode to every request, even when
`tools` is populated and `tool_choice: "auto"` is set.

**Diagnosis**: Either `--jinja` is absent (most common), or the model is too
small to reliably perform tool use (generally anything under 3B parameters), or
the model was not trained for tool use.

**Fix**:
1. Confirm `--jinja` is present.
2. Check the model size. Models under 3B parameters have limited tool-use
   reliability regardless of format.
3. Try a model explicitly trained for tool use (Hermes-3, Llama-3.1-8B,
   Qwen2.5-Coder-7B).

---

### Inconsistent tool use

**Symptom**: The model sometimes calls tools and sometimes answers in chat mode
for the same class of request.

**Diagnosis**: The context window is too small. As the conversation grows, the
tool definitions in the system prompt get pushed out of the active context window.
The model no longer "sees" that tools exist and falls back to chat mode.

**Fix**: Increase the context size:

```bash
llmctl config set server.ctx_size 32768
llmctl server restart
```

32K tokens accommodates most multi-turn agentic sessions. For very long sessions
(code review of large files, extended multi-tool chains), use 65536 if your RAM
permits.

---

### 500 errors on tool requests

**Symptom**: Requests that include a `tools` array return HTTP 500.

**Diagnosis**: The backend does not support the OpenAI tool protocol. This can
happen when using llama-swap in cold-swap mode with a model that has not been
loaded yet, or when the llama-server version predates tool support.

**Fix**: Ensure you are running a llama.cpp build from mid-2024 or later.
Explicitly include `"tool_choice": "auto"` in the request body — some server
versions reject requests where `tool_choice` is absent when `tools` is present.
Check `llmctl server version` to confirm the binary version.

---

## 7. Testing Tool Use

Use this curl command to verify that a running server handles tool calls correctly
end-to-end:

```bash
curl -s http://localhost:8765/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "hermes-3-llama-3.1-8b",
    "messages": [{"role": "user", "content": "list files in /tmp"}],
    "tools": [{"type": "function", "function": {
      "name": "bash",
      "description": "run a shell command",
      "parameters": {"type": "object",
        "properties": {"command": {"type": "string"}},
        "required": ["command"]}
    }}],
    "tool_choice": "auto",
    "max_tokens": 200
  }' | jq '.choices[0].message.tool_calls'
```

**Expected output** — a structured `tool_calls` array:

```json
[
  {
    "id": "call_0",
    "type": "function",
    "function": {
      "name": "bash",
      "arguments": "{\"command\": \"ls /tmp\"}"
    }
  }
]
```

**Failure output** — `null` with text content:

```bash
curl -s ... | jq '.choices[0].message'
```

```json
{
  "role": "assistant",
  "content": "To list files in /tmp, you can run:\n\nls /tmp",
  "tool_calls": null
}
```

If you get `null`, `--jinja` is missing or the model was not served through
llmctl's router.

### Testing the XML parser in isolation

To confirm llmctl's content parser is active, send a synthetic XML response and
check that it is rewritten:

```bash
# Start with a Qwen model
llmctl config set server.model Qwen2.5-Coder-7B
llmctl server restart

curl -s http://localhost:8765/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5-coder-7b",
    "messages": [{"role": "user", "content": "what is 2+2? use the calculate tool"}],
    "tools": [{"type": "function", "function": {
      "name": "calculate",
      "description": "evaluate a math expression",
      "parameters": {"type": "object",
        "properties": {"expression": {"type": "string"}},
        "required": ["expression"]}
    }}],
    "tool_choice": "auto",
    "max_tokens": 100
  }' | jq '{tool_calls: .choices[0].message.tool_calls, content: .choices[0].message.content}'
```

Expected: `tool_calls` populated, `content` null. If `content` contains an XML
string, the router is not intercepting the response — check that you are connecting
to llmctl's port (default 8765) and not directly to llama-server.
