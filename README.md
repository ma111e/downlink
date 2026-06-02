# DOWNLINK

[![CI](https://github.com/ma111e/downlink/actions/workflows/ci.yml/badge.svg)](https://github.com/ma111e/downlink/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](go.mod)

A feed aggregator and content-analysis platform for security news. DOWNLINK
collects RSS/Atom feeds, scrapes full article content, runs each article through
an LLM for rubric-based scoring and summarization, and assembles ranked digests
that it can publish to GitHub Pages and Discord.

```
feeds → scrape → LLM analysis & scoring → digest → publish (GitHub Pages / Discord)
```

## Features

- **Multi-provider LLMs** — Anthropic Claude, OpenAI, vLLM, Ollama, and llama.cpp
  (any OpenAI-compatible endpoint). Configure several and pick one per run.
- **Rubric scoring** — articles are scored on six dimensions (specificity, severity,
  breadth, novelty, actionability, credibility) rather than a single opaque number.
  See [`pkg/scoring`](pkg/scoring).
- **Flexible scraping** — plain RSS, dynamic scraping (Lightpanda), and full-browser
  scraping (Solimen) with per-feed CSS selectors, triggers, blacklists, and headers.
- **Digest generation** — categorized, importance-ranked digests with deduplication.
- **Publishing** — push static digests to GitHub Pages and notify a Discord webhook.
- **Interfaces** — gRPC API (default `:50051`), an Atom feed export (`:65261`), and the
  `dlk` command-line client.
- **Storage** — embedded SQLite via GORM; no external database required.

## Architecture

| Component         | Path          | Description                                                         |
| ----------------- | ------------- | ------------------------------------------------------------------- |
| Server            | `cmd/server`  | gRPC services + feed manager + Atom feed export. The core daemon.   |
| CLI (`dlk`)       | `cmd/dlk`     | gRPC client for articles, feeds, analysis, digests, config, queue.  |
| Shared packages   | `pkg/`        | Scoring, LLM gateway/providers, codex auth, models, protobufs.      |

The server exposes gRPC services for articles, analysis, feeds, digests, queue,
config, and auth. The Atom feed server publishes analyzed articles at
`http://localhost:65261`.

## Install

With Go 1.25+:

```sh
go install github.com/ma111e/downlink/cmd/dlk@latest      # CLI
go install github.com/ma111e/downlink/cmd/server@latest   # server
```

Or build from source:

```sh
make all        # builds server + cli
make server     # build just the server (embeds the git commit)
make cli        # build just dlk
```

Or with Docker:

```sh
docker build -t downlink .
docker run --rm -p 50051:50051 -p 65261:65261 \
  -v "$PWD/config.json:/app/config.json" \
  -v "$PWD/feeds.yml:/app/feeds.yml" \
  downlink
```

A `docker-compose.yml` is provided that also wires up the optional Solimen
full-browser scraper.

## Configuration

DOWNLINK reads configuration from three sources. Copy the bundled examples and
fill in your values:

```sh
cp config.example.json config.json   # LLM providers, analysis, notifications
cp feeds.example.yml   feeds.yml      # feed sources and per-feed scraping rules
cp .env.example        .env           # runtime/env overrides
```

- **`config.json`** — LLM provider definitions (name, type, model, base URL, API key),
  the active analysis provider and persona/writing style, and notification settings
  (Discord webhook, GitHub Pages repo/token/branch). See
  [config.example.json](config.example.json).
- **`feeds.yml`** — the list of feeds with per-feed scraping strategy, CSS selectors,
  triggers, blacklists, and custom HTTP headers. See [feeds.example.yml](feeds.example.yml).
- **`.env` / environment** — every server flag has a `DOWNLINK_*` env var equivalent
  (loaded via Viper). See [.env.example](.env.example). Precedence: CLI flag → env/`.env`
  → `config.json` → default.

> **Secrets:** `config.json`, `.env`, and `feeds.yml` are gitignored. Never commit
> real API keys, tokens, or webhook URLs.

## Running

Start the server:

```sh
./server --refresh                     # fetch all feeds on startup
./server --port 50051 --log-level info
```

Use the CLI against a running server:

```sh
dlk feed list
dlk article list
dlk analysis run
dlk digest generate
dlk --help
```

Check versions:

```sh
./server --version
dlk --version
```

## Publishing digests

DOWNLINK can publish generated digests to GitHub Pages and announce them on Discord.
See [docs/github-pages.md](docs/github-pages.md) for the full setup, including
`--init-gh-pages` and the required token scopes.

## Deployment

A sample systemd unit is provided at [etc/downlink.service](etc/downlink.service)
for running the server under `/opt/downlink`.

## License

[MIT](LICENSE) © 2026 ma111e

---

Contributions are not being accepted at this time.
