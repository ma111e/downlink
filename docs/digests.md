# Digests

A digest is a ranked, categorized summary of articles over a time window. Generating one
runs the full pipeline: fetch the window's articles, analyze any that lack a score,
deduplicate near-identical stories, rank by importance, write a summary, and store the
result. When publishing is configured it is also pushed out (see
[github-pages.md](github-pages.md)).

## Generating

```sh
dlk digest generate                 # last 24 hours (default)
dlk digest generate --from 7d       # last 7 days
dlk digest generate --day yesterday # one UTC day
```

Time selection:

| Flag | Description |
|---|---|
| `--from`, `--to`, `--between` | Time window. Accept `now`, a date, an RFC3339 timestamp, or a relative duration (`7d`, `2h`). Default window is the last 24h. |
| `--day` | A single midnight-to-midnight UTC day (`YYYY-MM-DD`, `today`, `yesterday`). Mutually exclusive with the window flags. |

Pipeline control:

| Flag | Description |
|---|---|
| `--refresh-feeds` | Refresh all feeds over the same window before generating. |
| `--dry-run` | List the articles that would be included, generate nothing. |
| `--skip-analysis` | Skip LLM article analysis. |
| `--skip-duplicates` | Skip LLM duplicate detection. |
| `--skip-llm` | Skip the LLM analysis and duplicate-detection steps. |
| `--one-shot` | Analyze missing articles with one combined prompt instead of the multi-step chain. |
| `--exclude-digested` | Exclude articles already in a previous digest. |
| `--reanalyze` | Re-analyze every article in the window. |
| `--reanalyze-on-model-change` | Re-analyze only articles last analyzed by a different model. |

Model and scoring:

| Flag | Description |
|---|---|
| `--provider/-p`, `--model/-m` | Override the LLM for this run (all steps). |
| `--select-model` | Pick a model interactively. |
| `--vibe-score` | Use legacy single-number scoring for this run. See [analysis-and-scoring.md](analysis-and-scoring.md). |
| `--glossary` | Generate plain-language explanations and a jargon glossary for this run. See [analysis-and-scoring.md](analysis-and-scoring.md). |
| `--standard-synthesis` | Also generate the Standard article summary for this run (Brief is always generated). Use `=false` to force off. |
| `--comprehensive-synthesis` | Also generate the Comprehensive article summary for this run. Use `=false` to force off. |
| `--executive-summary` | Generate the digest-level executive summary for this run (off by default). Use `=false` to force off. |

Output and publishing:

| Flag | Description |
|---|---|
| `--theme` | HTML theme (default `dark`). See themes below. |
| `--gh-pages` / `--no-gh-pages` | Force GitHub Pages publishing on or off for this run, overriding server config. |
| `--test` / `--test-digest-id <id>` | Send a stored digest to the notification channels without generating a new one. |

Generation streams per-stage and per-article progress. Press Ctrl-C once to ask the server
to stop at the next stage boundary; twice to force exit.

## Viewing

| Command | Description |
|---|---|
| `dlk digest list` | List digests (`--limit <n>`). |
| `dlk digest get [id]` | Show the summary and articles. `--markdown` renders styled prose. Omit the id to pick interactively. |
| `dlk digest articles [id]` | List just the articles in a digest. |

## Themes

Themes style the published HTML. List them with `dlk digest --list-themes`.

| Theme | Description |
|---|---|
| `dark` | Dark navy/charcoal (default). |
| `light` | Warm cream background, dark text. |
| `contrast` | Maximum-contrast black and white. |
| `mono` | Grayscale, no chroma. |
| `colorblind` | Light, colorblind-safe (Okabe-Ito). |
| `pastel` | Soft pastel cream/mint/coral, dark text. |

Pick a theme per run with `dlk digest generate --theme <name>`, or set a default for
published pages with `github_pages.theme` in config. The archive can be re-rendered into a
different theme later with `dlk publish republish-all --theme <name>`.

When a digest has glossary content, the page nav shows a **Glossary** switch next to the
theme picker. Toggling it reveals each article's plain-language explanation and jargon
glossary. See [analysis-and-scoring.md](analysis-and-scoring.md#glossary-mode).
