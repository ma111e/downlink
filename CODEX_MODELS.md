# Codex Model Fetching Strategy

This document describes how downlink CLI fetches available models for OpenAI Codex with a robust, layered fallback strategy inspired by Hermes CLI.

## Overview

When you use `dlk model` to select an OpenAI Codex provider, or run `dlk analysis run --provider openai-codex`, the CLI uses a **4-layer fallback strategy** to fetch available models:

```
Layer 1: Live API fetch (requires OAuth token) ↓
Layer 2: Local config file (~/.downlink/config.toml) ↓
Layer 3: Local models cache (~/.downlink/models_cache.json) ↓
Layer 4: Hardcoded defaults
```

Each layer is tried in sequence. The first successful result is used, ensuring offline support and fast fallbacks if the API is unavailable.

## Implementation Details

### Layer 1: Live API Fetch

**File:** `cmd/dlk/codex_models.go` → `fetchCodexModelsFromAPI()`

Makes a direct HTTP request to OpenAI's Codex models endpoint:

```
GET https://chatgpt.com/backend-api/codex/models?client_version=1.0.0
Authorization: Bearer <access_token>
```

**OAuth Token Sources** (in order):
1. `OPENAI_ACCESS_TOKEN` environment variable
2. `CHATGPT_TOKEN` environment variable

**Response Processing:**
- Parses JSON response containing a `models` array
- **Filters out hidden models** (visibility = "hide" or "hidden")
- **Does NOT filter on `supported_in_api`** — this field indicates public API availability, not OAuth-backed Codex availability
- **Sorts by priority** (lower priority value = appears first)
- **Applies forward-compat synthesis** to add newer model versions if older compatible ones exist

### Layer 2: Local Config File

**File:** `~/.downlink/config.toml` (or `$DOWNLINK_HOME/config.toml`)

Simple TOML-like parsing looking for a `model` key:

```toml
model = "gpt-4o"
```

Returns this as the top-priority model choice if present.

### Layer 3: Local Models Cache

**File:** `~/.downlink/models_cache.json` (or `$DOWNLINK_HOME/models_cache.json`)

A JSON cache file written by prior successful API calls:

```json
{
  "models": [
    {
      "slug": "gpt-4o",
      "priority": 1,
      "visibility": "public",
      "supported_in_api": true,
      "display_name": "GPT-4 Optimized",
      "description": "Latest GPT-4 variant",
      "context_window_size": 128000
    }
  ]
}
```

**Processing:** Same filtering (hidden exclusion, priority sort, forward-compat synthesis) as the API layer.

### Layer 4: Hardcoded Defaults

**File:** `cmd/dlk/codex_models.go` → `defaultCodexModels`

Fallback when all other layers fail or return no models:

```go
var defaultCodexModels = []string{
	"gpt-4o",
	"gpt-4-turbo",
	"gpt-4",
	"gpt-3.5-turbo",
}
```

These are curated, commonly-available models that work with Codex.

## Forward-Compat Synthesis

**Function:** `addForwardCompatModels()`

Synthetically generates newer model versions if older compatible ones are present. This mirrors OpenAI Codex CLI's own forward-compat behavior.

**Example:**
- If the API returns `gpt-4`, and a template exists for `gpt-4.5`, the function adds `gpt-4.5` to the list automatically
- Maps are defined in `forwardCompat` (currently empty, can be extended as needed)

## Integration with CLI

### Model Selection Flow

**File:** `cmd/dlk/llms.go` → `resolveModelInteractive()`

When the user runs:
```bash
dlk model
```

And selects an OpenAI Codex provider, the CLI:

1. Checks if provider type is "openai-codex"
2. If yes, calls `getCodexModelIDs(accessToken)` to fetch via the 4-layer strategy
3. If no, falls back to server-provided models via `client.GetAvailableModelsForProvider()`
4. Presents available models in an interactive picker
5. Saves the selection to analysis config

### Environment Variables

Set one of these to enable live API fetching:

```bash
export OPENAI_ACCESS_TOKEN="<your-access-token>"
# or
export CHATGPT_TOKEN="<your-access-token>"
```

Without a token, the CLI falls back to Layer 2 (config) → Layer 3 (cache) → Layer 4 (defaults).

## Cache Management

The models cache is automatically updated whenever a successful API call is made. To manually refresh:

1. Ensure `OPENAI_ACCESS_TOKEN` or `CHATGPT_TOKEN` is set
2. Run `dlk model` and select your Codex provider
3. The cache file `~/.downlink/models_cache.json` will be updated with the latest models

## Offline Usage

If the API is unavailable and no cache exists, the CLI will:

1. Check for a configured model in `~/.downlink/config.toml`
2. Fall back to hardcoded defaults
3. Allow manual entry of a custom model name

This ensures the CLI remains functional even without network connectivity.

## Troubleshooting

### No models showing up

1. Verify OAuth token is valid: `echo $OPENAI_ACCESS_TOKEN`
2. Check if cache exists: `cat ~/.downlink/models_cache.json`
3. Try manual entry: select "Custom..." in the model picker and enter the model name

### Models appearing in wrong order

Models are sorted by `priority` (lower = first). If priority is the same, alphabetical order is used. Check the cache file to see the priority values.

### Adding new models

Edit `~/.downlink/models_cache.json` directly or set a valid OAuth token to refresh from the live API.
