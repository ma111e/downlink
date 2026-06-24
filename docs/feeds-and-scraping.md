# Feeds and scraping

A feed is a source (RSS, Atom, or an HTML link-list page) plus rules for turning each item
into a full article. You describe feeds in `feeds.yml` and reconcile them into the database
with `dlk feeds apply -f feeds.yml`.

Feeds are global: every feed is fetched once and its articles are shared. A
[profile](profiles.md) selects a subset of feeds for its own digests.

## feeds.yml

The file is a list under `feeds:`, with an optional `default_selectors:` block applied to
feeds that do not set their own. Everything scraping-related lives under the `scraper:`
block; only `url`, `title`, `note`, and `enabled` stay at the top level.

```yaml
feeds:
  - url: https://example.com/rss.xml
    title: Example Blog
    enabled: true
    scraper:
      type: rss
      selectors:
        article: div.article-body
        cutoff: footer
```

### Top-level fields

| Field | Required | Description |
|---|---|---|
| `url` | yes | Feed URL. |
| `enabled` | yes | Whether the feed is fetched. |
| `title` | no | Display name. Auto-detected when empty. |
| `note` | no | Free-form note. |
| `topics` | no | Labels a [profile](profiles.md) selects feeds by, e.g. `[news, threat-intel]`. Many per feed. Fill them in bulk with `dlk feeds backfill-topics -f feeds.yml` (LLM, writes the file back). |
| `scraper` | yes | Scraping configuration. See below. |

### `scraper:` fields

| Field | Required | Description |
|---|---|---|
| `type` | yes | `rss`, `atom`, or `html` (link-list page). |
| `scraping` | no | Scraping mode: empty (static), `dynamic`, `full_browser`, or `none`. See below. |
| `selectors` | no | CSS selectors for article extraction (see below). |
| `headers` | no | Custom HTTP headers applied to this feed's fetch and article requests. |
| `triggers` | no | Full-browser load/fail selectors (see below). |
| type-specific | no | Options for the chosen `type` (e.g. html's `links_selector` / `url_filter`). |

### Selectors

The `scraper.selectors:` block accepts three CSS selectors:

| Selector | Description |
|---|---|
| `article` | Element wrapping the article body. |
| `cutoff` | Content from this element onward is dropped (footers, share bars). |
| `blacklist` | Elements removed from the extracted body (ads, promos). |

`scraper.triggers` configures full-browser scraping:

```yaml
scraper:
  type: rss
  scraping: full_browser
  triggers:
    loaded:                       # wait until these selectors appear
      - article
    failed:                       # bail early if these appear (anti-bot walls)
      - .cf-browser-verification
  selectors:
    article: .article-content
    cutoff: .share-buttons
```

Custom `headers` take precedence over the scraper's default spoofed headers, so they are
the place for `Authorization` or `X-Api-Key` on gated feeds.

### HTML link-list feeds (`type: html`)

Some blogs publish an index page that lists links to posts instead of an RSS/Atom feed.
The `html` type fetches that page, turns matched anchors into items, then fetches and
extracts each linked article with `selectors`.

```yaml
- url: https://blog.example.com/posts
  title: Linklist Blog
  enabled: true
  scraper:
    type: html
    scraping: static
    links_selector: "ul.posts li a"   # CSS selector for the post anchors (required)
    url_filter: "/posts/"             # keep only hrefs containing this substring (optional)
    selectors:
      article: div.post-content
```

| Option | Required | Description |
|---|---|---|
| `links_selector` | yes | CSS selector matching the post link anchors on the index page. |
| `url_filter` | no | Drop matched anchors whose resolved href does not contain this substring. |

A links-only page carries no dates, so every item is timestamped at fetch time:
`--from`/`--to` filtering and last-N ordering are no-ops for `html` feeds. De-duplication
still works — it keys on the link, not the date.

## Scraping modes

Each feed picks one mode. They escalate from cheapest to heaviest.

| Mode | Backend | When to use |
|---|---|---|
| static (empty) | direct HTTP fetch | Server-rendered article pages. The default. |
| `dynamic` | Lightpanda | Pages that need JavaScript to render the body. |
| `full_browser` | Solimen | Heavy JS, lazy loading, or anti-bot walls that need a real browser. Most expensive. |
| `none` | / | Skip fetching; use the feed item's own content. For feeds that already carry full text. |

`dynamic` requires a running Lightpanda container; `full_browser` requires Solimen. The
server can start both for you with `--auto-start-lightpanda` and `--auto-start-solimen`
(both need Docker). Solimen's address is set with `--solimen-addr` or `solimen_addr` in
config. See [deployment.md](deployment.md).

## Building a feed config

Finding the right mode, headers, and selectors is the bulk of the work. Downlink gives you
two paths, both under `dlk feeds`.

### Automatic

`dlk feeds autoconfig <rss-url>` runs an autonomous LLM agent: it probes and locks the
scraping mode and headers, then ranks and tests article selectors and prints a finished
feed config to paste into `feeds.yml`. Nothing is registered automatically.

```sh
dlk feeds autoconfig https://example.com/rss.xml
```

Flags: `--header/-H` (seed a header, repeatable), `--provider/-p`, `--model/-m`,
`--max-steps`, `--yes/-y`, `--verbose/-v`. The full agent design is documented in
[feed-autoconfig.md](feed-autoconfig.md).

### Manual

The same primitives the agent uses are exposed as subcommands, cheapest first:

| Command | Purpose |
|---|---|
| `feeds inspect <rss-url>` | Fetch the feed, diagnose it, list sample article links, and scaffold a starter config. |
| `feeds probe-headers <feed-url>` | Try header combinations (Referer, desktop UA, RSS Accept) and report which unblock a blocked feed. |
| `feeds fetch-article <url> --mode <m>` | Print an article page's HTML so you can spot the body selector. |
| `feeds probe-modes <url>` | Try each scraping mode and report the cheapest that yields full content. `--article` judges by what a selector extracts. |
| `feeds test-selector --url <url> --article <css>` | Score a selector against one or more articles. Pass `--url` repeatedly to check stability. |

`fetch-article`, `probe-modes`, and `test-selector` take `--mode/-m`
(`static`/`dynamic`/`full_browser`) and `--header/-H`. A selector scoring 0.8 or higher
across several articles is reliable.

The interactive `feed-config-builder` skill drives these commands end to end if you want a
guided session.

## Debugging a feed

When a feed errors during `feeds refresh`, run `dlk feeds diagnose <feed>`. It reports the
HTTP status, content type, a guess at the body type (rss/atom/json-feed/html/empty), any
parse error, the byte offset of invalid UTF-8, and the path to the saved raw body. Add
`--raw` to print the body itself.
