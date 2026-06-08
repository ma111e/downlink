# Analysis and scoring

Analysis runs each article through the configured LLM to produce a summary, key points,
and an importance score. Scores drive how articles are ranked and grouped in digests.

## Running analysis

- `dlk analysis run <article-id>` analyzes one article and streams progress.
- `dlk analysis run [feed|all] --from 7d` enqueues a batch for the queue to process.
- Set `analysis.auto_analyze` (or `--auto-analyze`) so each feed refresh enqueues new
  articles automatically.

Analysis also happens implicitly during `dlk digest generate`, which analyzes any
not-yet-scored article in the window before assembling the digest. See
[digests.md](digests.md) and [cli-reference.md](cli-reference.md).

## The rubric

By default the LLM does not pick a final score. It rates six narrow dimensions on a 0 to 4
scale, and Downlink combines them deterministically. Because the raw dimensions are stored
with the article, scores can be recomputed later without re-running the LLM.

| Dimension | 0 | 4 | Weight |
|---|---|---|---|
| Specificity | generic/evergreen concept | single concrete, recent event | 20% |
| Severity | informational | active exploitation, critical patch, major breach | 25% |
| Breadth | niche product | ubiquitous software or whole sector | 20% |
| Novelty | rehash of known facts | genuinely new disclosure | 10% |
| Actionability | nothing to do | clear defensive action, patch, IOCs | 15% |
| Credibility | unsourced blogspam | primary source, vendor advisory, named researcher | 10% |

The weighted average of the six dimensions is scaled to a 0 to 100 score, then two
overrides apply:

- **Aggregators** (roundups, weekly recaps, link digests) are forced to **40**, whatever
  the dimensions say.
- **Pure-evergreen** articles (Specificity 0) are capped at **60**.

## Tiers

The 0 to 100 score maps to a read tier used to group articles in a digest:

| Tier | Score |
|---|---|
| Must Read | 90 and up |
| Should Read | 75 to 89 |
| May Read | 60 to 74 |
| Optional | 1 to 59 |
| Unscored | 0 |

## Vibe scoring

The legacy mode asks the LLM for a single importance number directly instead of the
rubric. Enable it with `analysis.vibe_score` in config, the server's `--vibe-score` flag,
or per run with `dlk digest generate --vibe-score`. The rubric is the default and is
recommended; vibe scores cannot be recomputed without re-running the LLM.

## Persona and writing style

Two `analysis` config fields shape LLM output:

- **`persona`** a prompt prefix injected before every analysis request (for example,
  "Be concise and decisive"). Edit it with `dlk model set-persona`.
- **`writing_style`** a style guide injected into the digest summary prompt (voice,
  tense, attribution rules).

Both are set in `config.json`; see [configuration.md](configuration.md).
