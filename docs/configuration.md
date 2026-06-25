# Configuration

Downlink reads configuration from these files plus environment variables:

- **`config.json`** server-side configuration: LLM providers, the active analysis
  provider, and notification settings. Loaded by the server at startup.
- **`feeds.yml`** the list of feeds and their per-feed scraping rules. Applied to the
  database with `dlk feeds apply -f feeds.yml`. See [feeds-and-scraping.md](feeds-and-scraping.md).
- **`profiles.yml`** optional editorial profiles. Applied at server startup; restart to
  apply. Without it the server runs as a single default profile. See
  [profiles.md](profiles.md).
- **`.env`** runtime overrides via `DOWNLINK_*` variables, loaded automatically.

Copy the bundled examples to start:

```sh
cp config.example.json config.json
cp feeds.example.yml feeds.yml
cp profiles.example.yml profiles.yml   # optional, only for multiple profiles
cp .env.example .env
```

## Precedence

For settings that exist in more than one place (the server flags and the GitHub Pages
block), the order from highest to lowest is:

```
CLI flag  →  environment variable / .env  →  config.json  →  built-in default
```

Provider definitions, the analysis block, and the Discord block live only in
`config.json`. The server flags and the GitHub Pages block can also come from env vars
or flags.

## config.json

Top-level fields:

| Field | Type | Default | Description |
|---|---|---|---|
| `db_path` | string | `./downlink.db` | SQLite database file path. |
| `solimen_addr` | string | `http://localhost:5011` | Address of the Solimen full-browser scraper. |
| `providers` | array | / | LLM provider entries. See [llm-providers.md](llm-providers.md). |
| `analysis` | object | / | Analysis settings (active provider, persona, scoring). |
| `notifications` | object | / | Discord and GitHub Pages settings. |

A provider's `name` is required; the server refuses to start if any provider entry is
missing it.

### providers[]

Each entry configures one LLM backend.

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | / | Unique identifier for this entry (required). |
| `provider_type` | string | / | `anthropic`, `openai`, `mistral`, `ollama`, `llamacpp`, `openai-codex`, or `claude-code`. |
| `model_name` | string | / | Model to use. |
| `enabled` | bool | `false` | Whether the entry is usable. |
| `base_url` | string | / | Endpoint URL. Required for `llamacpp` and OpenAI-compatible endpoints; optional for `anthropic`/`openai` (provider default used when empty). |
| `api_key` | string | / | API key. Not used by `ollama`/`llamacpp` (local) or the OAuth provider types. |
| `max_retries` | int | / | Retries on a failed request. |
| `timeout_minutes` | int | `20` | Per-request timeout. |
| `credentials` | array | / | OAuth credential pool for `openai-codex`/`claude-code`; populated by `dlk model creds login`, not edited by hand. |

See [llm-providers.md](llm-providers.md) for which fields each provider type needs.

### analysis

| Field | Type | Default | Description |
|---|---|---|---|
| `provider` | string | / | `name` of the provider entry used for analysis and digest summaries. |
| `auto_analyze` | bool | `false` | Enqueue articles for analysis automatically after each feed refresh. |
| `vibe_score` | bool | `false` | Use the legacy single-number importance prompt instead of the rubric. See [analysis-and-scoring.md](analysis-and-scoring.md). |
| `glossary` | bool | `false` | Generate a plain-language explanation and jargon glossary per article, shown on the digest page behind a "Glossary" nav switch. Adds one LLM round-trip per article; re-analyze existing articles to populate it. Override per run with `dlk digest generate --glossary` (or `--glossary=false` to force off). |
| `standard_synthesis` | bool | `false` | Also generate the **Standard** article summary (a medium, multi-paragraph synthesis), shown as an extra tab in the digest. The **Brief** summary is always generated. Override per run with `dlk digest generate --standard-synthesis` (or `=false` to force off). |
| `comprehensive_synthesis` | bool | `false` | Also generate the **Comprehensive** article summary (a long, structured analysis), shown as an extra tab in the digest. Override per run with `dlk digest generate --comprehensive-synthesis` (or `=false` to force off). |
| `executive_summary` | bool | `false` | Generate the digest-level executive summary (a title plus a thematic overview shown under "Executive Overview" and in the Discord embed). Override per run with `dlk digest generate --executive-summary` (or `=false` to force off). |
| `persona` | string | / | Prompt prefix injected before every analysis request. |
| `writing_style` | string | / | Style guide injected into the digest summary prompt. |
| `step_providers` | object | / | Per-step provider/model overrides. See [Per-step providers](#per-step-providers). |
| `worker_pool.max_workers` | int | `3` | Analysis worker pool size. Bounds how many articles are analyzed in parallel; actual LLM concurrency is still capped by `--max-concurrent-llm-requests`. |

#### Per-step providers

`step_providers` routes individual pipeline steps to a different provider or model than
`analysis.provider`, to trade cost against quality per step. Each key is a step name; each
value is `{provider, model}` (both optional). A `provider`-only entry keeps that step's
model from the named provider; a `model`-only entry keeps `analysis.provider` and swaps the
model. Steps with no entry use `analysis.provider`. This is global config only; profiles
inherit it but cannot override it.

```json
"analysis": {
  "provider": "vllm",
  "step_providers": {
    "importance": { "provider": "Claude", "model": "claude-sonnet-4-6" },
    "digest_summary": { "model": "claude-opus-4-8" }
  }
}
```

Per-article task steps: `categorize`, `tldr`, `plain_words`, `key_points`, `insights`,
`referenced_reports`, `summaries`, `glossary`, `importance`. Digest-level steps: `dedupe`,
`digest_summary`, `glossary_entities`, `glossary_context`. Switching provider mid-article
resets the conversation and re-sends the article text on the next step.

### notifications.discord

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Send digest notifications to Discord. |
| `webhook_url` | string | / | Discord webhook URL. |

### notifications.github_pages

Publishing digests to GitHub Pages has its own guide with the full field table, token
scopes, and workflow: [github-pages.md](github-pages.md).

## Environment variables

Every server flag has a `DOWNLINK_*` equivalent (loaded from `.env` or the environment).
These override `config.json` and are themselves overridden by an explicit CLI flag.

| Variable | Flag | Default | Description |
|---|---|---|---|
| `DOWNLINK_HOST` | `--host` | `localhost` | gRPC bind host. |
| `DOWNLINK_PORT` | `--port` | `50051` | gRPC port. |
| `DOWNLINK_TLS` | `--tls` | `false` | Enable TLS. |
| `DOWNLINK_CERT_FILE` | `--cert-file` | / | TLS certificate path (required with TLS). |
| `DOWNLINK_KEY_FILE` | `--key-file` | / | TLS key path (required with TLS). |
| `DOWNLINK_REFRESH` | `--refresh` | `false` | Refresh all feeds on startup. |
| `DOWNLINK_LOG_LEVEL` | `--log-level` | `info` | `trace`, `debug`, `info`, `warn`, `error`. |
| `DOWNLINK_TRACE_DIR` | `--trace-dir` | `/tmp/downlink-trace-<ts>` | Where content traces go; only active at `trace` level. |
| `DOWNLINK_AUTO_START_LIGHTPANDA` | `--auto-start-lightpanda` | `false` | Start the Lightpanda container if absent. |
| `DOWNLINK_AUTO_START_SOLIMEN` | `--auto-start-solimen` | `false` | Start the Solimen container if absent. |
| `DOWNLINK_SOLIMEN_ADDR` | `--solimen-addr` | `http://localhost:5011` | Solimen address for `full_browser` scraping. |
| `DOWNLINK_PROFILES_FILE` | `--profiles-file` | `profiles.yml` | Profiles catalog applied at startup; skipped if absent. See [profiles.md](profiles.md). |
| `DOWNLINK_LAYOUTS_DIR` | `--layouts-dir` | `layouts` | Directory of on-disk custom layouts; used if it exists. |
| `DOWNLINK_MAX_CONCURRENT_LLM_REQUESTS` | `--max-concurrent-llm-requests` | `1` | Cap on concurrent LLM calls across all paths. |
| `DOWNLINK_AUTO_ANALYZE` | `--auto-analyze` | `false` | Same as `analysis.auto_analyze`. |
| `DOWNLINK_VIBE_SCORE` | `--vibe-score` | `false` | Same as `analysis.vibe_score`; overrides config. |
| `DOWNLINK_GLOSSARY` | `--glossary` | `false` | Same as `analysis.glossary`; overrides config. |
| `DOWNLINK_ADMIN_PORT` | `--admin-port` | `65262` | Localhost port for the LLM monitoring dashboard. See [deployment.md](deployment.md). |
| `DOWNLINK_LLM_MONITOR_RETENTION` | `--llm-monitor-retention` | `100` | Most-recent digest runs whose LLM conversations are kept (`0` disables pruning). |
| `DOWNLINK_FEED_MONITOR_RETENTION` | `--feed-monitor-retention` | `100` | Most-recent feed-refresh runs whose history is kept (`0` disables pruning). |

The `DOWNLINK_GH_PAGES_*` variables map to the `--gh-pages-*` flags and the
`github_pages` config block; they are documented in [github-pages.md](github-pages.md).

> `trace` log level writes raw LLM prompts/responses and scraped bodies to disk. Use it
> for debugging only, and treat the trace directory as sensitive.

## Changing config at runtime

`dlk config show` prints the server's live config; `dlk config edit` opens it in
`$EDITOR` and saves changes back. Provider and analysis settings can also be changed with
`dlk model` commands. See [cli-reference.md](cli-reference.md).
