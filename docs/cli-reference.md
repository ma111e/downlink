# CLI reference

`dlk` is the command-line client. It talks to a running server over gRPC, except for the
`publish` commands, which connect to GitHub directly.

## Global flags

| Flag | Default | Description |
|---|---|---|
| `--address` | `localhost` | gRPC server address. |
| `--port` | `50051` | gRPC server port. |
| `--json` | `false` | Emit JSON instead of formatted tables. |

`dlk` also reads `.env` from the working directory.

## article

Browse and update stored articles.

| Command | Description |
|---|---|
| `article list` | List articles. Flags: `--unread`, `--bookmarked`, `--category <name>`, `--from`, `--to`, `--between`. |
| `article get [id]` | Show one article. `--markdown` renders the body as styled prose. Omit the id to pick interactively. |
| `article update [id]` | Mark read state or bookmark: `--read`, `--unread`, `--bookmark`, `--unbookmark`. |

Time-window flags (`--from`/`--to`/`--between`) accept `now`, an RFC3339 timestamp, a
date (`2025-01-01`), or a relative duration (`7d`, `2h`, `30m`).

## feeds

Manage feed sources. See [feeds-and-scraping.md](feeds-and-scraping.md) for the config
schema and the selector-building subcommands.

| Command | Description |
|---|---|
| `feeds list` | List registered feeds. |
| `feeds add` | Register a feed. Run with no `--url` for an interactive wizard, or pass `--url`, `--name`, `--type`, `--scraping`, `--selector-article`, `--selector-cutoff`, `--selector-blacklist`. |
| `feeds refresh [feed\|all]` | Pull new articles. Accepts a feed id, a normalized name, or `all`. Flags: `--from`, `--to`, `--between`, `--last-n`, `--overwrite`, `--restore`, `--dry-run`, `--debug`. |
| `feeds apply -f <file>` | Reconcile the database to a feeds YAML file: create/update listed feeds, disable the rest (articles kept). `--dry-run` previews. |
| `feeds export` | Write the database's feeds back to YAML. `-o <file>` (default stdout), `--enabled-only`. |
| `feeds backfill-topics -f <file>` | LLM-fill `topics:` in a feeds YAML file and write it back (for [profile](profiles.md) selection). Labels feeds without topics by default; `--overwrite` redoes all. `--dry-run`, `-y`, `-p`/`-m`. |
| `feeds delete` | Delete feeds by `-f <file>`, `-t <title>`, `-i <id>`, or interactively. `--dry-run` previews. Deleting a feed removes its articles. |
| `feeds diagnose <feed>` | Probe one feed and report the raw HTTP response, parse errors, and UTF-8 issues. `--raw` also prints the saved body. |

Selector-building subcommands (`inspect`, `fetch-article`, `test-selector`,
`probe-modes`, `probe-headers`, `autoconfig`) are covered in
[feeds-and-scraping.md](feeds-and-scraping.md).

## analysis

Run and inspect LLM analysis of articles. See
[analysis-and-scoring.md](analysis-and-scoring.md).

| Command | Description |
|---|---|
| `analysis run [article\|feed\|all]` | Analyze a single article (streams progress) or enqueue a batch by feed/time window. Flags: `--provider/-p`, `--model/-m`, `--provider-profile`, `--select-model`, `--from`, `--to`, `--between`, `--all-time`, `--key-points-only`, `--dry-run`. |
| `analysis list [article-id]` | List all analyses for an article. |
| `analysis get <analysis-id>` | Show one analysis. `--markdown` for styled prose. |
| `analysis config show` | Show the analysis configuration. |

Batch analysis requires a time window (`--from`/`--to`/`--between`) or `--all-time`.
Enqueued jobs are processed by the queue; watch them with `queue status`.

## model

Configure LLM providers and pick the active analysis model. See
[llm-providers.md](llm-providers.md).

| Command | Description |
|---|---|
| `model` | Interactively select the active provider and model. |
| `model list` | List configured provider entries. |
| `model add` | Add a provider entry interactively. |
| `model update` | Update entries. Flags: `--provider/-p`, `--model/-m`, `--api-key/-k`, `--url/-u`, `--enabled/-e`, `--all/-a`, `--file/-f`. With no targeting flag, runs interactively. |
| `model remove` | Remove a provider entry interactively. |
| `model set-persona` | Edit the analysis persona prompt. |
| `model creds login [provider-type]` | OAuth device-code login for `openai-codex` / `claude-code`. Pass the provider type to skip the picker. |
| `model creds list` | List stored OAuth credentials. |
| `model creds remove` | Remove an OAuth credential. |
| `model creds priority` | Reorder the OAuth credential pool. |

## digest

Generate and view digests. See [digests.md](digests.md).

| Command | Description |
|---|---|
| `digest list` | List digests. `--limit <n>`. |
| `digest get [id]` | Show a digest summary and its articles. `--markdown` for styled prose. |
| `digest generate` | Build a new digest from a time window (default: last 24h). `--profile <slug>` generates for one [profile](profiles.md); many other flags, see [digests.md](digests.md). |
| `digest articles [id]` | List the articles in a digest. |
| `digest list --themes` | List available layouts and exit. |

Profiles are defined in `profiles.yml` and applied at server startup (no `dlk` command);
see [profiles.md](profiles.md).

## config

| Command | Description |
|---|---|
| `config show` | Print the server's live configuration. |
| `config edit` | Open the configuration in `$EDITOR` (default `vi`) and save changes back. |

## glossary

Inspect and curate the global jargon glossary. The glossary is built during
`digest generate --glossary`; see [analysis-and-scoring.md](analysis-and-scoring.md#glossary-mode).

| Command | Description |
|---|---|
| `glossary list` | List glossary entries. `--limit <n>` (0 = all). |
| `glossary override <term> <definition>` | Set a curated definition that wins over the generated one and survives regeneration. Terms match case-insensitively and ignore all punctuation and spacing (`cobalt strike` == `cobalt-strike`, `wscript.exe` == `wscript-exe`). |

## queue

Control the analysis queue.

| Command | Description |
|---|---|
| `queue status` | Live TUI monitor. Keys: `s`/`x`/`c` to start/stop/clear, `q` to quit. With `--json`, prints a status snapshot. |
| `queue start` | Start processing. |
| `queue stop` | Pause processing. |
| `queue clear` | Empty the queue (does not stop active work). |

## publish

Set up and maintain the GitHub Pages archive. These commands connect to GitHub directly
with a token and do not need a running server, except `add`, `republish`, and
`republish-all`, which fetch digests from the server. See [github-pages.md](github-pages.md).

Persistent flags (all subcommands): `--repo`, `--branch`, `--token`, `--output-dir`,
`--configure-pages`, `--clone-dir`, `--commit-author`, `--commit-email`, `--theme`,
`--window-days` (days of digests to retain in the manifest and feeds; `0` uses the default
of 30). The token can also come from `DOWNLINK_GH_PAGES_TOKEN`; `--theme` from
`DOWNLINK_GH_PAGES_THEME`; `--window-days` from `DOWNLINK_GH_PAGES_WINDOW_DAYS`.

| Command | Description |
|---|---|
| `publish init` | Create the branch if absent and seed the manifest and index pages. Idempotent; existing files are kept. |
| `publish reinit` | Erase the branch and local clone, then recreate from scratch. Destructive; prompts for confirmation. |
| `publish add [digest-id]` | Fetch a digest from the server, render it, and push it to the archive. `--no-wait`. |
| `publish remove [title]` | Remove a digest (matched by title) from the archive and republish. `--no-wait`. |
| `publish republish [id-or-title]` | Remove and re-add one digest with the current templates. `--no-wait`. |
| `publish republish-all` | Re-render every published digest. `--dry-run`, `--no-wait`. |
| `publish republish-index` | Re-render just the archive index pages. `--dry-run`, `--no-wait`. |
