# Codex Model Fetching

This document describes how downlink CLI fetches available models for OpenAI Codex.

## Overview

When you use `dlk model` to select an OpenAI Codex provider, or run `dlk analysis run --provider openai-codex`, the CLI fetches available models directly from OpenAI's Codex API.

## Implementation Details

### Direct API Fetch

**File:** `cmd/dlk/codex_models.go` → `fetchCodexModelsFromAPI()`

Makes a direct HTTP request to OpenAI's Codex models endpoint:

```
GET https://chatgpt.com/backend-api/codex/models?client_version=1.0.0
Authorization: Bearer <access_token>
```

**OAuth Token Sources** (checked in order):
1. `OPENAI_ACCESS_TOKEN` environment variable
2. `CHATGPT_TOKEN` environment variable

**Response Processing:**
- Parses JSON response containing a `models` array
- **Filters out hidden models** (visibility = "hide" or "hidden")
- **Does NOT filter on `supported_in_api`** — this field indicates public API availability, not OAuth-backed Codex availability
- **Sorts by priority** (lower priority value = appears first)
- Returns slug/model names in priority order

If the API call fails or no token is available, the user is prompted to enter a model name manually.

## Integration with CLI

### Model Selection Flow

**File:** `cmd/dlk/llms.go` → `resolveModelInteractive()`

When the user runs:
```bash
dlk model
```

And selects an OpenAI Codex provider, the CLI:

1. Checks if provider type is "openai-codex"
2. If yes, looks for `OPENAI_ACCESS_TOKEN` or `CHATGPT_TOKEN` env vars
3. If token found, calls `getCodexModelIDs(accessToken)` to fetch from API
4. If token not found or API fails, prompts user to enter model name manually
5. For other providers, uses server-provided models via `client.GetAvailableModelsForProvider()`
6. Presents available models in an interactive picker
7. Saves the selection to analysis config

### Environment Variables

Set one of these to enable live API fetching:

```bash
export OPENAI_ACCESS_TOKEN="<your-access-token>"
# or
export CHATGPT_TOKEN="<your-access-token>"
```

Without a valid token, you'll be prompted to enter the model name manually (e.g., `gpt-4o`).

## Usage

### With OAuth Token

```bash
export OPENAI_ACCESS_TOKEN="your-token-here"
dlk model
# Select "openai-codex" provider
# CLI fetches available models and lets you pick one
```

### Without Token

```bash
dlk model
# Select "openai-codex" provider
# CLI prompts you to enter model name manually
# Enter: gpt-4o
```

## Troubleshooting

### Error: "Could not fetch Codex models from API"

This means the API call failed. Check:
1. OAuth token is valid: `echo $OPENAI_ACCESS_TOKEN`
2. Token is still fresh (they expire)
3. Network connectivity to `chatgpt.com`

You can still proceed by entering a model name manually when prompted.

### How do I know what models are available?

Use the ChatGPT web interface or check the Codex CLI documentation. Once you have a token, running `dlk model` will show you the exact list available through your account.
