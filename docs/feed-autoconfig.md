# Feed autoconfig

`autoconfig` takes a bare RSS/Atom URL and produces a complete feed config:
scraping mode, HTTP headers, and the article-content CSS selectors. It runs the
same probes as the manual `dlk feeds` primitives and uses an LLM only to pick
selectors.

## Entry points

- CLI: `dlk feeds autoconfig <rss-url>` ([cmd/dlk/feeds_build.go:58](../cmd/dlk/feeds_build.go#L58))
- gRPC: `FeedsService.AutoConfigFeed`, server-streaming ([feeds.go:204](../cmd/server/internal/services/feeds.go#L204))
- Agent loop: `runAutoConfig` ([feed_autoconfig.go:108](../cmd/server/internal/services/feed_autoconfig.go#L108))

The CLI opens the stream, prints each step, and prints the final YAML. It
registers and writes nothing. Paste the output into `feeds.yml` and run
`dlk feeds apply`.

Flags:

| Flag | Meaning |
|------|---------|
| `-H, --header "Key: Value"` | Seed header, repeatable. Starting point for header probing. |
| `-p, --provider` | LLM provider (type or configured provider name). |
| `-m, --model` | Model override. |
| `--max-steps` | Cap on agent turns. `0` uses the server default (16). |

## Phases

`runAutoConfig` runs three phases. The first two are deterministic and decide
everything that hits the target server expensively; the LLM only sees phase 3,
with mode and headers frozen. The model cannot change mode or headers, fetch new
URLs, or repeat an identical tool call.

### 1. Lock headers

`lockHeaders` ([feed_autoconfig.go:222](../cmd/server/internal/services/feed_autoconfig.go#L222)):

1. Inspect the feed with the seed headers. If it parses, use them and return the
   sample article links.
2. If blocked, probe a fixed set once each: `{Referer}`, then
   `{Referer, desktop UA}`. The `Referer` is `scheme://host/` from the feed URL.
3. First combination that parses wins. If none unblocks the feed, fail.

If the feed parses but has no sample links, fail: nothing to inspect.

### 2. Lock scraping mode

`lockMode` ([feed_autoconfig.go:247](../cmd/server/internal/services/feed_autoconfig.go#L247)).
Tried cheapest first: `static -> dynamic -> full_browser`. For each, get
selector candidates for the first sample article and check the top candidate's
char count:

- Top candidate ≥ 2000 chars (`autoconfigUsableChars`): that mode wins, and its
  candidates seed the loop.
- Otherwise keep the best-by-length mode and continue.
- If none clears 2000, use the best-by-length mode anyway (likely low final
  confidence).

2000 is the shared `minUsableChars` threshold
([scrapers/usability.go:11](../cmd/server/internal/scrapers/usability.go#L11)):
shorter content is a stub, paywall teaser, or unrendered JS shell.

### 3. Selector discovery (LLM loop)

`writeSeed` ([feed_autoconfig.go:276](../cmd/server/internal/services/feed_autoconfig.go#L276))
primes the transcript with the task, the fixed mode, the locked header names,
the feed type, the sample links, and the ranked candidates.

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

`finishAutoConfig` ([feed_autoconfig.go:291](../cmd/server/internal/services/feed_autoconfig.go#L291))
requires a non-empty `article`. It re-tests against up to 3 samples for the
final confidence, assembles a `FeedConfig` (locked mode and headers, selectors,
URL/type, `enabled: true`), and marshals to YAML. `Scraping` is empty when the
mode is `static`. Running out of steps without finishing fails.

## Scoring

`ScoreSelector` ([scrapers/usability.go:51](../cmd/server/internal/scrapers/usability.go#L51)):

```
score = usableRatio * (0.5 + 0.5 * consistency)
consistency = 1 - min(cv, 1)
```

`usableRatio` is the fraction of samples that matched with ≥ 500 chars. `cv` is
the coefficient of variation (stddev / mean) of the usable lengths, so a
selector that grabs 4000 chars on one page and 600 on another is penalized.
Score ≥ 0.8 is `Reliable`. The CLI colours it green ≥ 0.8, yellow ≥ 0.5, red
below.

## Streaming events

`AutoConfigFeedEvent`:

- `STEP`: one per probe or tool call, with step number, tool name, and a short
  detail. The counter is shared across all three phases.
- `DONE`: config YAML, summary/rationale (the model's `thought`), confidence.
- `ERROR`: failure detail.

## Tool backing

`managerTools` ([feed_autoconfig.go:455](../cmd/server/internal/services/feed_autoconfig.go#L455))
delegates to `manager.Manager`:

| Agent tool | Manager call |
|------------|--------------|
| `inspectFeed` | `InspectFeedURL` |
| `suggestSelectors` | `SuggestSelectors` |
| `testSelector` | `InspectArticle` per URL, then `ScoreSelector` |

`autoconfigTools` and `autoconfigGenerate` are injected into `runAutoConfig`, so
the loop is tested against fakes
([feed_autoconfig_test.go](../cmd/server/internal/services/feed_autoconfig_test.go)).

## Failure modes

- No LLM gateway: "autoconfig unavailable: no LLM gateway configured". Model is
  resolved via `ResolveLLM` from the flags or defaults.
- Feed unreachable/blocked after probing: "feed is blocked and no tried header
  set unblocked it".
- No sample links: "feed has no sample article links to inspect".
- No convergence: "agent did not converge within N steps".

## Manual builder commands

The same workflow by hand, via the other `dlk feeds` subcommands
([cmd/dlk/feeds_build.go](../cmd/dlk/feeds_build.go)). Use when you want control
or autoconfig fails to converge. None registers or writes anything; all support
`--json` and `-H`.

| Command | Phase it maps to |
|---------|------------------|
| `probe-headers` | 1, lock headers |
| `probe-modes` | 2, lock mode |
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

Usable means: with a selector, matched with ≥ 500 chars; without one, non-empty
HTML and no error.

### probe-headers `<feed-url>`

When a feed 403s or returns an anti-bot/HTML page, try common header
combinations and report which return a valid feed. Combinations: `default`,
`+Referer`, `+Referer +UA`, `+RSS Accept`. Recommends the first that parses.

- `-H, --header`: extra header applied to every combination.

`probe-modes`/`probe-headers` test and report every combination; autoconfig's
lock phases stop at the first that works.
