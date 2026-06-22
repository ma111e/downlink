# Downlink

[![CI](https://github.com/ma111e/downlink/actions/workflows/ci.yml/badge.svg)](https://github.com/ma111e/downlink/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](go.mod)
[![Docker Image](https://img.shields.io/github/v/tag/ma111e/downlink?logo=docker&logoColor=white&label=ghcr.io%2Fma111e%2Fdownlink&color=2496ED)](https://github.com/ma111e/downlink/pkgs/container/downlink)

A feed aggregator and content-analysis platform for security news. Downlink
collects RSS/Atom feeds, scrapes full article content, runs each article through
an LLM for rubric-based scoring and summarization, and assembles ranked digests
that it can publish to GitHub Pages and Discord.

## Features

- **Multi-provider LLMs** Claude, ChatGPT Codex, Mistral, vLLM, Ollama, and llama.cpp
  (any OpenAI-compatible endpoint)
- **Scoring** articles are either vibe-scored by the LLM or scored across six dimensions
  (specificity, severity, breadth, novelty, actionability, credibility) instead of a single
  opaque number. See [`pkg/scoring`](pkg/scoring).
- **Flexible scraping** plain RSS, dynamic scraping (Lightpanda), and full-browser
  scraping ([Solimen](https://github.com/ma111e/solimen)) with per-feed CSS selectors, triggers, blacklists, and headers.
- **Digest generation** categorized, importance-ranked digests with deduplication.
- **Profiles** run several curated views over one feed pool, each with its own feed
  selection, editorial config, digests, layout, and theme. See [docs/profiles.md](docs/profiles.md).
- **Glossary mode** optional plain-language explanations and a jargon glossary per article,
  toggled on the digest page.
- **Publishing** push static digests to GitHub Pages and notify a Discord webhook.
- **Interfaces** gRPC API (default `:50051`) and the `dlk` command-line client.

## Architecture

| Component         | Path          | Description                                                         |
| ----------------- | ------------- | ------------------------------------------------------------------- |
| Server            | `cmd/server`  | gRPC services + feed manager.   |
| CLI (`dlk`)       | `cmd/dlk`     | gRPC client for articles, feeds, analysis, digests, config, queue management.  |
| Shared packages   | `pkg/`        | Scoring, LLM gateway/providers, codex auth, models, protobufs.      |

The server exposes gRPC services for articles, analysis, feeds, digests, queue management,
config, and auth.

## Install

With Go 1.25+:

```sh
go install github.com/ma111e/downlink/cmd/dlk@latest # CLI
go install github.com/ma111e/downlink/cmd/server@latest # server
```

Or build from source:

```sh
make all # builds server + cli
make server # build just the server
make cli # build just dlk
```

Or with Docker:

```sh
docker build -t downlink .
docker run --rm -p 50051:50051 \
  -v "$PWD/config.json:/app/config.json" \
  -v "$PWD/feeds.yml:/app/feeds.yml" \
  downlink
```

A `docker-compose.yml` is provided that also wires up the optional Solimen
full-browser scraper.

## Quickstart

**1. Build the binaries** (see [Install](#install)):

```sh
make all # produces ./server and ./dlk
```

**2. Create your config** and enable at least one LLM provider (analysis needs one;
fetching feeds does not):

```sh
cp config.example.json config.json
$EDITOR config.json # enable a provider + fill in its api_key
```

**3. Write a demo `feeds.yml`** with a few public security feeds:

```yaml
feeds:
  - url: https://cert.gov.ua/api/articles/rss
    title: Cert-UA
    enabled: true
    scraper:
      type: rss
      scraping: dynamic # "dynamic" (Lightpanda) or "full_browser" (Solimen); omit for static RSS
      selectors:
        article: div.article-item__content # CSS selector for the article body
  - url: https://www.bleepingcomputer.com/feed/
    title: Bleeping Computer
    enabled: true
    scraper:
      type: rss
      scraping: full_browser
      triggers:
        loaded:
          - article .article_section
      selectors:
        article: div.articleBody
  - url: https://feeds.feedburner.com/TheHackersNews
    title: The Hacker News
    enabled: true
    scraper:
      type: rss
      scraping: full_browser
      triggers:
        loaded:
          - div.main-box
      selectors:
        article: '#articlebody'
        cutoff: .stophere
```

> The quickstart mixes scraping modes: `scraping: dynamic` needs **Lightpanda** and
> `scraping: full_browser` needs **Solimen**. The `--auto-start-*` flags in step 4
> launch both in Docker. For a no-dependencies first run, keep only
> `scraper: { type: rss }` and drop the `scraping`, `triggers`, and `selectors`
> keys to use plain RSS.

**4. Start the server** (in one terminal). The quickstart feeds use both scrapers, so start
them too (requires Docker):

```sh
./server --auto-start-lightpanda --auto-start-solimen
```

**5. Apply the feeds.** `dlk feeds apply` reconciles the database to match the file. Feeds in the file are created or updated, and feeds no longer listed are disabled
(their articles are kept). Preview first with `--dry-run`:

```sh
./dlk feeds apply -f feeds.yml --dry-run # show what would change
./dlk feeds apply -f feeds.yml # apply it
```

**6. Fetch and generate a digest.** `digest generate` analyzes any not-yet-scored
articles with your LLM provider, then assembles the ranked digest:

```sh
./dlk feeds refresh all # pull the latest articles
./dlk digest generate   # analyze + assemble the ranked digest
```

**7. View the result:**

```sh
./dlk digest list # list generated digests
./dlk digest get  # pick one and view it (add --markdown for prose)
```

`./dlk feeds export -o feeds.yml` does the reverse of step 5: it writes the feeds
currently in the database back out to a YAML file.

## Documentation

Full guides live in [docs/](docs/README.md): getting started, the configuration and CLI
references, feeds and scraping, analysis and scoring, profiles, LLM providers, digests,
publishing, and deployment.

## Configuration

Downlink reads configuration from these sources. Copy the bundled examples and
fill in your values:

```sh
cp config.example.json config.json # LLM providers, analysis, notifications
cp feeds.example.yml feeds.yml # feed sources and per-feed scraping rules
cp profiles.example.yml profiles.yml # optional: multiple editorial profiles
cp .env.example .env # runtime/env overrides
```

- **`config.json`** LLM provider definitions (name, type, model, base URL, API key),
  the active analysis provider and persona/writing style, and notification settings
  (Discord webhook, GitHub Pages repo/token/branch). See
  [config.example.json](config.example.json).
- **`feeds.yml`** the list of feeds with per-feed scraping strategy, CSS selectors,
  triggers, blacklists, and custom HTTP headers. See [feeds.example.yml](feeds.example.yml).
- **`profiles.yml`** optional editorial profiles, each with its own feed subset, editorial
  config, and presentation. Applied at startup. See [docs/profiles.md](docs/profiles.md).
- **`.env`** every server flag has a `DOWNLINK_*` env var equivalent
  (loaded automatically via Viper). See [.env.example](.env.example). Precedence: CLI flag -> env variables/`.env`
  -> `config.json` -> default.

## Publishing digests

Downlink can publish generated digests to GitHub Pages and announce them on Discord.
See [docs/github-pages.md](docs/github-pages.md) for the full setup, including
`--init-gh-pages` and the required token scopes.

## Deployment

A sample systemd unit is provided at [etc/downlink.service](etc/downlink.service)
for running the server under `/opt/downlink`.

## License

[MIT](LICENSE) © 2026 ma111e

---

Contributions are not being accepted at this time.
