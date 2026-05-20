# Codex Model Fetching

This document describes how downlink CLI fetches available models for OpenAI Codex using stored server credentials.

## Overview

When you use `dlk model` to select an OpenAI Codex provider, or run `dlk analysis run --provider openai-codex`, the CLI fetches available models directly from OpenAI's Codex API using stored OAuth credentials from the server.

## Credential Management

Codex OAuth credentials are stored on the server in the provider configuration. The credential pool manages:
- Multiple credentials per provider (credential rotation)
- Automatic access token refresh when expiring
- Priority-based credential selection (lower priority = used first)
- Credential health tracking (ok, auth-failed, rate-limited)

## Implementation Details

### API Fetch Using Stored Credentials

**File:** `cmd/dlk/codex_models.go` → `fetchCodexModelsFromAPI()`

Makes a direct HTTP request to OpenAI's Codex models endpoint:

```
GET https://chatgpt.com/backend-api/codex/models?client_version=1.0.0
Authorization: Bearer <access_token>
```

**Token Source:**
- Fetched from the openai-codex provider's stored credentials on the server
- Uses the highest-priority (lowest numeric priority value) credential

**Response Processing:**
- Parses JSON response containing a `models` array
- **Filters out hidden models** (visibility = "hide" or "hidden")
- **Does NOT filter on `supported_in_api`** — this field indicates public API availability, not OAuth-backed Codex availability
- **Sorts by priority** (lower priority value = appears first)
- Returns slug/model names in priority order

If no credentials are stored, the user is prompted to enter a model name manually.

## Integration with CLI

### Model Selection Flow

**File:** `cmd/dlk/llms.go` → `resolveModelInteractive()`

When the user runs:
```bash
dlk model
```

And selects an OpenAI Codex provider, the CLI:

1. Checks if provider type is "openai-codex"
2. Retrieves the provider config from the server (includes stored credentials)
3. Uses the first credential's access token to call `getCodexModelIDs()`
4. Fetches available models from the Codex API
5. If no credentials stored, prompts user to authenticate
6. Presents available models in an interactive picker
7. Saves the selection to analysis config

### Authentication

To add Codex credentials:

```bash
dlk model auth login
# Follow prompts to authenticate with OpenAI Codex
# Credentials are stored securely on the server
```

To list stored credentials:

```bash
dlk model auth list
```

To remove a credential:

```bash
dlk model auth remove <credential-id>
```

To set credential priority:

```bash
dlk model auth priority <credential-id> <priority>
```

## Usage

### With Stored Credentials

```bash
# First, authenticate (one time setup)
dlk model auth login
# Follow the device code flow to authenticate with Codex

# Then use model selector
dlk model
# Select "openai-codex" provider
# CLI fetches available models using stored credentials and lets you pick one
```

### Without Credentials

```bash
dlk model
# Select "openai-codex" provider
# CLI shows error: "No Codex credentials stored. Run 'dlk model auth login' to authenticate."
# You can still enter a model name manually when prompted
```

## Troubleshooting

### Error: "No Codex credentials stored"

This means no OAuth credentials have been stored for any openai-codex provider. Fix:
```bash
dlk model auth login
# Complete the device code flow
```

### Error: "Could not fetch Codex models from API"

This means the API call failed. Check:
1. Credentials are still valid: `dlk model auth list`
2. Network connectivity to `chatgpt.com`
3. Try authenticating again: `dlk model auth login`

You can still proceed by entering a model name manually when prompted.

### How do I know what models are available?

Use the ChatGPT web interface or check the Codex documentation. Once you authenticate, running `dlk model` will show you the exact list available through your account.
