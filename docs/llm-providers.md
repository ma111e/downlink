# LLM providers

Analysis and digest summaries need one enabled LLM provider. Fetching feeds does not.
Providers are entries in the `providers` array of `config.json`; the `analysis.provider`
field names which entry is active. See [configuration.md](configuration.md).

## Supported types

| `provider_type` | Auth | Required fields | Notes |
|---|---|---|---|
| `anthropic` | API key | `api_key` | Claude. `base_url` optional. |
| `openai` | API key | `api_key`, `base_url` | Any OpenAI-compatible endpoint (OpenAI, vLLM, Nvidia, ...). `base_url` defaults to OpenAI when empty. |
| `mistral` | API key | `api_key` | Mistral. |
| `ollama` | none | `base_url` | Local. Defaults to `http://localhost:11434`. |
| `llamacpp` | none | `base_url` | Local llama.cpp server. |
| `openai-codex` | OAuth | credential pool | ChatGPT via device-code login. |
| `claude-code` | OAuth | credential pool | Claude via subscription login. |

OpenAI-compatible endpoints (vLLM, Nvidia, local gateways) all use `provider_type:
openai` with the appropriate `base_url`. Example from `config.example.json`:

```json
{
  "name": "vllm",
  "provider_type": "openai",
  "model_name": "YOUR_VLLM_MODEL_NAME",
  "enabled": true,
  "base_url": "https://your-vllm-endpoint.example.com/v1",
  "api_key": "sk-YOUR_VLLM_KEY_HERE"
}
```

## Managing providers

- `dlk model list` lists configured entries.
- `dlk model add` adds one interactively (prompts for type, name, key or base URL, model,
  timeout).
- `dlk model update` changes an entry; with flags (`-p`, `-m`, `-k`, `-u`, ...) it edits
  non-interactively.
- `dlk model remove` deletes an entry.
- `dlk model` (no subcommand) picks the active analysis provider and model.

You can also override the provider or model per run on `dlk analysis run` and
`dlk digest generate` with `--provider`/`--model`, or pick interactively with
`--select-model`. See [cli-reference.md](cli-reference.md).

## OAuth providers

`openai-codex` and `claude-code` authenticate with a device-code login instead of an API
key. Add the entry first, then log in:

```sh
dlk model add                 # choose openai-codex or claude-code
dlk model auth login          # device-code flow; stores the credential
```

Credentials are stored as a pool on the provider entry. Manage them with
`dlk model auth list`, `dlk model auth remove`, and `dlk model auth priority` (the pool is
tried in priority order). The OAuth managers persist credentials back into `config.json`.

## Concurrency

The server runs at most `--max-concurrent-llm-requests` LLM calls at once (default `1`),
enforced across every path: direct analysis, queued analysis, digest deduplication, and
digest summaries. Raise it to parallelize analysis when your provider and rate limits
allow. Set it with the flag or `DOWNLINK_MAX_CONCURRENT_LLM_REQUESTS`.
