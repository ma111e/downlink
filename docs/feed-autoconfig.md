# Feed autoconfig

`autoconfig` takes a URL and produces a complete feed config: scraping mode, HTTP
headers, topic labels, and the article-content CSS selectors. It runs the same
probes as the manual `dlk feeds` primitives, deciding mode and headers
deterministically, and uses an LLM only to derive topics and pick selectors.

The argument can be an RSS/Atom feed, an HTML index page (a link-list of posts),
or a plain web page. For a web page the CLI first discovers the page's feeds and
lets you pick one (`resolveFeedURL` in [cmd/dlk/feeds_build.go](../cmd/dlk/feeds_build.go)).

## Entry points

- CLI: `dlk feeds autoconfig <url>` ([cmd/dlk/feeds_build.go](../cmd/dlk/feeds_build.go))
- gRPC: `FeedsService.AutoConfigFeed`, server-streaming ([cmd/server/internal/services/feeds.go](../cmd/server/internal/services/feeds.go))
- Agent loop: `runAutoConfig` ([cmd/server/internal/services/feed_autoconfig.go](../cmd/server/internal/services/feed_autoconfig.go))

The CLI opens the stream, prints each step, and prints the final YAML. It
registers nothing. Paste the output into `feeds.yml` and run
`dlk feeds apply -f feeds.yml`, or pass `--update <file>` to merge it into an
existing feeds file (`mergeAutoConfig`), which prints a diff when the feed is
already present.

Flags:

| Flag | Meaning |
|------|---------|
| `-H, --header "Key: Value"` | Seed header, repeatable. Starting point for header probing. |
| `-p, --provider` | LLM provider (type or configured provider name). |
| `-m, --model` | Model override. |
| `--max-steps` | Cap on agent turns. `0` uses the server default (16). |
| `--no-topics` | Skip topic extraction (don't propose feed topics). |
| `--update <file>` | Merge the result into this feeds YAML file instead of printing it. |
| `-y, --yes` | Skip the confirmation prompt. |
| `-v, --verbose` | Stream the raw LLM prompt and response for each agent turn. |

## Phases (RSS/Atom)

`runAutoConfig` decides everything that hits the target server expensively
before the LLM selector loop runs. The model only sees the selector phase, with
mode and headers frozen; it cannot change mode or headers, fetch new URLs, or
repeat an identical tool call.

### 1. Lock headers

`lockHeaders`:

1. Inspect the feed with the seed headers. If it parses, use them and return the
   sample article links.
2. If blocked, probe a fixed set once each: `{Referer}`, then
   `{Referer, desktop UA}`. The `Referer` is `scheme://host/` from the feed URL.
3. First combination that parses wins. If none unblocks the feed, fail.

If the feed parses but has no sample links, fail: nothing to inspect.

### 2. Extract topics

`extractTopics` (skipped with `--no-topics`): one LLM call derives 1 to 5 broad
topic labels for the feed from its title and a few sample entries, preferring the
topic vocabulary already in use across configured feeds so profiles select feeds
consistently. Best-effort: any error or unparseable reply yields no topics and
autoconfig proceeds.

### 3. Feed-content short-circuit

`feedContentScore`: if the feed's entries already carry full article bodies
(score `>= 0.8`, `feedContentModeThreshold`), autoconfig emits a `scraping: none`
config and stops. No page scraping and no selector LLM loop happen.

### 4. Lock scraping mode

`lockMode`, tried cheapest first: `static -> dynamic -> full_browser`. For each,
get selector candidates for the first sample article and check the top
candidate's char count:

- Top candidate `>= 1500` chars (`autoconfigUsableChars`): that mode wins, and
  its candidates seed the loop.
- Otherwise keep the best-by-length mode and continue.
- If none clears 1500, use the best-by-length mode anyway (likely low final
  confidence).

`autoconfigUsableChars` (1500) is the mode-probe bar; it sits a little below the
`minUsableChars` (2000) yardstick that `Usable`/`ScoreSelector` apply when scoring
final selectors ([cmd/server/internal/scrapers/usability.go](../cmd/server/internal/scrapers/usability.go)).

### 5. Selector discovery (LLM loop)

`discoverArticleSelectors`. `writeSeed` primes the transcript with the task, the
fixed mode, the locked header names, the feed type, the sample links, and the
ranked candidates.

The loop runs up to `max-steps` turns (default 16, `defaultAutoconfigSteps`).
Each turn:

1. Send system prompt + transcript to the LLM.
2. Parse the reply to one JSON action. `parseAction` extracts the first balanced
   `{...}`, so stray prose does not break it.
3. Duplicate-call guard, keyed on `action + args`: any repeat of a non-`finish`
   call is rejected with a note instead of re-hitting the server.
4. Dispatch.

Protocol (system prompt:
[feed_autoconfig_prompt.md](../cmd/server/internal/services/feed_autoconfig_prompt.md)).
Every message is a single JSON object, one action per turn:

```json
{"thought": "...", "action": "<tool>", "args": { ... }}
```

| Action | Purpose | Args |
|--------|---------|------|
| `test_selector` | Extract with candidate selectors across all samples and score it. Main tool. | `{article, cutoff?, blacklist?}` |
| `suggest_selectors` | Re-rank candidates for another sample article. | `{article_url}` |
| `finish` | Emit chosen selectors. | `config.selectors.{article, cutoff?, blacklist?}` |

The model may only use selectors from candidate output or confirmed by testing,
and must `finish` with the best it found even when nothing scores well (flagged
low-confidence).

## HTML index pages

When the URL is an HTML index page (`type: html`), `runHTMLAutoConfig` replaces
phases 1 to 4 with a link-list pre-phase, then shares the same selector loop:

1. `lockIndexHeaders`: probe the raw page (no RSS parse) for headers that return
   a usable response.
2. `lockIndexMode`: probe `static -> dynamic -> full_browser` for the first mode
   that yields at least 3 (`autoconfigMinLinks`) repeating post links.
3. Lock the top-ranked link list deterministically (no LLM): `links_selector`
   plus an inferred `url_filter` and per-post `date_selector`. A warning is
   emitted when no date selector is found (posts may lack publish dates).
4. `lockMode` against the first discovered post, then escalate to the heavier of
   the index mode and the article mode so one mode covers both fetches.
5. Topics (best-effort, from the first post's text) unless `--no-topics`.
6. The shared selector loop, with the link-list options carried into the final
   config's `scraper.` options.

## Finish

`finishAutoConfig` requires a non-empty `article` selector. It re-tests against
up to 3 samples for the final confidence, assembles a `FeedConfig` (locked mode
and headers, selectors, topics, URL/type, html link-list options, `enabled:
true`), and marshals to YAML. `scraping` is empty when the mode is `static`.
Running out of steps without finishing fails.

## Scoring

`ScoreSelector` ([cmd/server/internal/scrapers/usability.go](../cmd/server/internal/scrapers/usability.go)):

```
score = usableRatio * (0.5 + 0.5 * consistency)
consistency = 1 - min(cv, 1)
```

`usableRatio` is the fraction of samples that matched with `>= 2000` chars
(`minUsableChars`). `cv` is the coefficient of variation (stddev / mean) of the
usable lengths, so a selector that grabs 4000 chars on one page and 600 on
another is penalized. Score `>= 0.8` is `Reliable`. The CLI colours it green
`>= 0.8`, yellow `>= 0.5`, red below.

## Streaming events

`AutoConfigFeedEvent` kinds:

- `STEP`: one per probe or tool call, with step number, tool name, and a short
  detail. The counter is shared across every phase.
- `LLM_IO`: the raw prompt and response for one agent turn (only when `--verbose`).
- `DONE`: config YAML (including topics), summary/rationale (the model's
  `thought`), confidence.
- `ERROR`: failure detail.

## Tool backing

`managerTools` ([cmd/server/internal/services/feed_autoconfig.go](../cmd/server/internal/services/feed_autoconfig.go))
delegates to `manager.Manager`:

| Agent tool | Purpose |
|------------|---------|
| `inspectFeed` | Fetch + parse a feed; return sample links and per-entry content lengths. |
| `suggestSelectors` | Rank article-body selector candidates for a page in a mode. |
| `testSelector` | Extract with selectors across URLs, then `ScoreSelector`. |
| `articleText` | Scrape a page's main text (feed-content match and html topics). |
| `existingTopics` | Distinct topics already in use across configured feeds. |
| `inspectIndex` | Fetch an HTML index page (no RSS parse) and report usability. |
| `suggestLinkSelectors` | Rank repeating post-link structures on an index page. |

`autoconfigTools` and `autoconfigGenerate` are injected into `runAutoConfig`, so
the loop is tested against fakes
([feed_autoconfig_test.go](../cmd/server/internal/services/feed_autoconfig_test.go)).

## Failure modes

- No LLM gateway: "autoconfig unavailable: no LLM gateway configured". Model is
  resolved via `ResolveLLM` from the flags or defaults.
- Feed unreachable/blocked after probing: "feed is blocked and no tried header
  set unblocked it".
- No sample links: "feed has no sample article links to inspect".
- HTML index with no post list: "no repeating post-link list found".
- No convergence: "agent did not converge within N steps".

## Manual builder commands

The same workflow by hand, via the other `dlk feeds` subcommands
([cmd/dlk/feeds_build.go](../cmd/dlk/feeds_build.go)). Use when you want control
or autoconfig fails to converge. None registers or writes anything; all support
`--json` and `-H`.

| Command | Phase it maps to |
|---------|------------------|
| `probe-headers` | lock headers |
| `probe-modes` | lock mode |
| `inspect` / `fetch-article` | sample links + candidates |
| `test-selector` | the `test_selector` loop |

### inspect `<rss-url>`

Print a feed's diagnosis, up to 5 sample article links, and a starter config
(URL, detected title, type, headers).

### fetch-article `<article-url>`

Scrape one article and print its page HTML.

- `-m, --mode` (default `static`): `static` | `dynamic` | `full_browser`.
- `--full`: print the entire page HTML (otherwise capped).

### test-selector `--url <article-url> --article <css>`

Extract content from one or more articles with the given selectors and score the
result. Pass `--url` repeatedly to score stability across articles. Prints
per-URL match/char counts, the `ScoreSelector` score, and one extracted sample.

- `--url` (repeatable, required)
- `--article` (required), `--cutoff`, `--blacklist`
- `-m, --mode` (default `static`)

### probe-modes `<article-url>`

Try `static -> dynamic -> full_browser` against an article and report what each
yields, recommending the cheapest usable one. With `--article` it judges by the
content that selector extracts; otherwise by rendered HTML size.

- `--article`, `--cutoff`, `--blacklist`

Usable means: with a selector, matched with `>= 2000` chars; without one,
non-empty HTML and no error.

### probe-headers `<feed-url>`

When a feed 403s or returns an anti-bot/HTML page, try common header
combinations and report which return a valid feed. Combinations: `default`,
`+Referer`, `+Referer +UA`, `+RSS Accept`. Recommends the first that parses.

- `-H, --header`: extra header applied to every combination.

`probe-modes`/`probe-headers` test and report every combination; autoconfig's
lock phases stop at the first that works.
