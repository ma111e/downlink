# Profiles

A profile is a curated, public view of the shared article pool. Feeds and articles are
fetched once and shared across the whole server; a profile selects a subset of feeds,
re-analyzes them with its own editorial config, and produces its own digests with its own
presentation. The same article can appear in two profiles with different summaries, scores,
and categories.

> "Profile" here means an editorial profile. LLM *providers* are a separate concept,
> configured in `config.json` and selected with `--provider` / `--provider-profile`. See
> [llm-providers.md](llm-providers.md).

## The default profile

The server always seeds a profile named `default`. It owns every enabled feed, and its
editorial config inherits the live `analysis` block from `config.json`. With no
`profiles.yml`, the server runs exactly as a single-tenant install: the default profile is
your current setup. Nothing about the published site changes until a second profile exists.

## profiles.yml

Profiles are defined in `profiles.yml` and applied to the database when the server starts
(path via `--profiles-file`, default `profiles.yml`). Edit the file and restart to apply.
This differs from `feeds.yml`, which is applied at runtime with `dlk feeds apply`. Copy the
bundled example:

```sh
cp profiles.example.yml profiles.yml
```

```yaml
profiles:
  - slug: neteng
    name: "Network Engineer Brief"
    description: "Infrastructure security news for network engineers."
    icon: "🖥️"
    layout: default
    theme: light
    enabled: true
    sort_order: 0
    topics: [news, threat-intel]   # feeds tagged with ANY of these
    editorial:
      persona: "You are a senior network engineer writing for infrastructure teams."
      writing_style: "Direct and operational. Lead with impact and remediation steps."
      audience: "network engineers responsible for patching and hardening"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `slug` | string | required | URL-safe id; the digest output subdirectory and the `--profile` value. |
| `name` | string | `slug` | Display name on the landing page and switcher. |
| `description` | string | | Shown on the landing page. |
| `icon` | string | | Emoji or short text shown next to the name. |
| `layout` | string | `default` | Template set (see [Presentation](#presentation)). |
| `theme` | string | `dark` | First-paint color palette (see [Presentation](#presentation)). |
| `enabled` | bool | `true` | Disabled profiles are skipped in generation, the landing page, and feeds. |
| `sort_order` | int | `0` | Order on the landing page and switcher. |
| `output_subdir` | string | `slug` | GitHub Pages subdirectory. The default profile uses `github_pages.output_dir`. |
| `topics` | list | | Select feeds carrying ANY of these topics. See [Feed selection](#feed-selection). |
| `feeds` | list | | Explicit include: feed URLs always in the pool. |
| `exclude_feeds` | list | | Explicit exclude: feed URLs always dropped. |
| `editorial` | object | | Per-profile editorial config; see below. |

## Feed selection

A profile's feed pool is resolved from its `topics` plus per-feed overrides:

    pool = (feeds with ANY of `topics`)  ∪  `feeds`  −  `exclude_feeds`   (enabled feeds only)

Tag feeds with `topics:` in [feeds.yml](feeds-and-scraping.md); a profile then selects by
topic, so adding a feed with a matching topic flows it into every matching profile on the
next `dlk feeds apply`, no profile edit needed. To label an existing `feeds.yml` in bulk, run
`dlk feeds backfill-topics -f feeds.yml` (the LLM derives each feed's topics from its
articles and writes them back). `feeds` and `exclude_feeds` reference feeds by URL (matched
to the catalog by host) for the odd feed you want pinned in or out.

A profile with **no topics and no explicit includes** gets **all enabled feeds**; that is
why the default profile (which sets neither) carries every feed. A profile that sets only
`exclude_feeds` means "everything except these".

## Editorial config

Every `editorial` field is optional. An omitted field inherits the global `config.json`
`analysis` value (and the built-in rubric, categories, and task prompts). The default
profile leaves `editorial` empty, so it tracks `config.json` live. See
[analysis-and-scoring.md](analysis-and-scoring.md) for what each knob does.

| Field | Type | Description |
|---|---|---|
| `provider` | string | Provider name for this profile's LLM steps. |
| `model` | string | Model override. |
| `persona` | string | Analysis system-message prefix. |
| `writing_style` | string | Style guide injected into the digest summary. |
| `audience` | string | Target reader, injected into analysis and summary prompts. |
| `glossary` | bool | Generate the plain-language glossary task. |
| `vibe_score` | bool | Use the legacy single-number score instead of the rubric. |
| `standard_synthesis` | bool | Generate the medium-length article summary. |
| `comprehensive_synthesis` | bool | Generate the full article summary. |
| `executive_summary` | bool | Generate the digest-level overview. |
| `categories` | list | Custom category set (replaces the default); each is `{name, description}`. |
| `rubric` | object | Custom importance model; see below. |
| `prompts` | object | Raw task-prompt overrides; see below. |

`rubric` overrides the importance model. Omitted sub-fields keep the defaults.

| Field | Type | Description |
|---|---|---|
| `weights` | map | Per-dimension weights, keyed `specificity`, `severity`, `breadth`, `novelty`, `actionability`, `credibility`. |
| `tiers` | object | Read-tier lower bounds: `{must, should, may}` on the 0-100 score. |
| `aggregator_score` | int | Fixed score forced for roundup/recap articles. |
| `promo_cap` | int | Cap for promotional articles (announcements, marketing, commercials). |
| `evergreen_cap` | int | Cap for pure-evergreen articles (specificity 0). |

`prompts` replaces task instructions verbatim. The output JSON schema and required keys are
not overridable, so result validation and corrective re-prompts still apply.

| Field | Type | Description |
|---|---|---|
| `tasks` | map | Task name → instruction. Keys: `categorize`, `tldr`, `plain_words`, `key_points`, `insights`, `referenced_reports`, `summaries`, `glossary`, `importance`. |
| `digest_summary` | string | Extra guidance appended to the digest-summary prompt. |
| `dedupe` | string | Extra guidance appended to the duplicate-grouping prompt. |

## Generating

```sh
dlk digest generate --profile neteng
```

The profile scopes the article window to its feed pool and analyzes each article with the
profile's editorial config, stored separately from other profiles. Omitting `--profile`
uses the default profile. To generate every profile, run the command once per slug. See
[digests.md](digests.md).

## Presentation

Two independent axes:

- **Layout** (`layout`): a full template set. The built-in layout is `default`.
  Ship your own by placing a template directory under the layouts dir (`--layouts-dir`,
  default `layouts/`); a custom layout may override only some pages and inherits the rest
  from `default`.
- **Theme** (`theme`): the first-paint color palette, one of `dark`, `light`, `contrast`,
  `mono`, `colorblind`, `pastel`. Readers can still switch themes in the page; the profile
  value is only the default.

When more than one profile is enabled, published pages get a floating profile switcher, and
the GitHub Pages root becomes a landing page listing the profiles (plus `profiles.json`).
Each profile publishes under its own subdirectory. A single-profile site keeps the flat
layout. See [github-pages.md](github-pages.md).

## Monitoring

Every digest generation records an LLM run tagged with its profile. The monitoring
dashboard ([deployment.md](deployment.md)) filters by profile with `?profile=<slug>`.
