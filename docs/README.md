# Downlink documentation

Downlink is a feed aggregator and content-analysis platform for security news. It collects
RSS/Atom feeds, scrapes full article content, scores and summarizes each article with an
LLM, and assembles ranked digests it can publish to GitHub Pages and Discord.

Start with [getting-started.md](getting-started.md). The project
[README](../README.md) covers the overview and install.

## Guides

- **[Getting started](getting-started.md)** install, configure, and produce a first digest.
- **[Configuration](configuration.md)** the `config.json` and environment-variable
  reference, and how the three config sources combine.
- **[CLI reference](cli-reference.md)** every `dlk` command and flag.

## Feeds and analysis

- **[Feeds and scraping](feeds-and-scraping.md)** the feed schema, the four scraping
  modes, and the tools for building a feed config.
- **[Feed autoconfig](feed-autoconfig.md)** the autonomous agent that discovers a feed's
  scraping mode, headers, and selectors on its own.
- **[Analysis and scoring](analysis-and-scoring.md)** the six-dimension rubric, tiers, and
  the persona/writing-style knobs.
- **[LLM providers](llm-providers.md)** supported provider types, setup, OAuth, and
  concurrency.

## Output

- **[Digests](digests.md)** generating, controlling, and viewing digests, plus themes.
- **[GitHub Pages publishing](github-pages.md)** publish digests as a static archive and
  notify Discord.

## Operations

- **[Deployment](deployment.md)** Docker, systemd, ports, and scheduling.
