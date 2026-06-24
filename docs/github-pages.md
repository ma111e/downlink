# GitHub Pages Publishing

Downlink can automatically publish each digest as a self-contained HTML page to a GitHub Pages repository. After every successful digest generation, it:

1. Clones (or pulls) your Pages repo locally.
2. Writes the digest HTML file (e.g. `digests/downlink-digest-2026-04-24_1200.html`) and a companion swipe/triage page (`digests/downlink-swipe-2026-04-24_1200.html`).
3. Regenerates `digests/index.html` and `digests/manifest.json` listing all digests, newest first.
4. Writes a root `index.html` that renders the digest archive and reads files from the configured output directory.
5. Commits and pushes the changed files.
6. Optionally sends a Discord message with the link to the new page.

---

## 1. Create a GitHub Pages repository

If you don't have one, create a repository named `<your-username>.github.io` (for a user site) or any repo with GitHub Pages enabled on a branch.

In the repo's **Settings -> Pages**, set the source to the branch you'll push to (e.g. `main`, root `/`).

---

## 2. Create a Personal Access Token

Go to **GitHub -> Settings -> Developer settings -> Personal access tokens -> Fine-grained tokens** (or classic tokens).

For normal publishing, the token needs **Contents: Read and write** on the target repository.

If you enable `configure_pages`, the token also needs permission to manage GitHub Pages settings:

- Fine-grained token: **Pages: Read and write** and **Administration: Read and write** on the target repository.
- Classic token: `repo`.

> Keep the token out of version control. Use the environment variable `DOWNLINK_GH_PAGES_TOKEN` or the `token` config field (see below).

---

## 3. Configure `config.json`

Add a `github_pages` block inside `notifications`:

```json
{
  "notifications": {
    "discord": {
      "enabled": true,
      "webhook_url": "https://discord.com/api/webhooks/..."
    },
    "github_pages": {
      "enabled": true,
      "repo_url": "https://github.com/your-username/your-username.github.io.git",
      "branch": "main",
      "configure_pages": false,
      "output_dir": "digests",
      "base_url": "https://your-username.github.io",
      "discord_webhook_url": "https://discord.com/api/webhooks/..."
    }
  }
}
```

### All fields

| Field | Required | Default | Description |
|---|---|---|---|
| `enabled` | yes | `false` | Enable or disable publishing. |
| `repo_url` | yes | / | HTTPS clone URL of the Pages repo. |
| `branch` | no | `main` | Branch to clone and push to. When `configure_pages` is true, this is also configured as the GitHub Pages source branch. |
| `configure_pages` | no | `false` | Configure the GitHub Pages source to `branch` at `/` before publishing. Requires extra token permissions. |
| `token` | no* | / | GitHub PAT. Prefer `DOWNLINK_GH_PAGES_TOKEN` env var instead. |
| `output_dir` | no | `digests` | Safe relative subdirectory inside the repo where digest files are written. Absolute paths, `.`, `..`, and parent traversal are rejected. |
| `layout` | no | `default` | Layout (template set) for the published pages when a profile sets none. See [digests.md](digests.md#layouts-and-themes). |
| `base_url` | no | / | Public URL of the site (e.g. `https://your-username.github.io`). Used to build links in Discord notifications. |
| `commit_author` | no | `downlink-bot` | Git commit author name. |
| `commit_email` | no | `downlink-bot@users.noreply.github.com` | Git commit author email. |
| `clone_dir` | no | `$TMPDIR/downlink-ghpages` | Local path where the repo is cloned. Persists across runs to avoid full re-clones. |
| `discord_webhook_url` | no | / | A **separate** Discord webhook to notify when a page is published. Distinct from the main digest webhook. |
| `publish_window_days` | no | `30` | Days of digests to keep in the manifest and feeds; older entries are pruned on publish. `0` uses the default. |

*\* `token` must be provided via config or environment variable for publishing to work.*

---

## 4. Provide the token

Set the environment variable before starting the server:

```sh
export DOWNLINK_GH_PAGES_TOKEN=github_pat_...
./server
```

Or put the token directly in `config.json` under `github_pages.token` - but be careful not to commit it.

---

## 5. CLI flag overrides

Every config field has a corresponding flag on the `server` command. Flags override config file values when explicitly set.

```
--gh-pages-enabled              Enable GitHub Pages publishing
--gh-pages-repo <url>           Repo clone URL
--gh-pages-branch <branch>      Branch to push to
--gh-pages-configure            Configure GitHub Pages source to the selected branch at /
--gh-pages-token <token>        GitHub PAT (prefer env var)
--gh-pages-output-dir <dir>     Subdirectory for digest files
--gh-pages-base-url <url>       Public base URL of the site
--gh-pages-commit-author <name> Commit author name
--gh-pages-commit-email <email> Commit author email
--gh-pages-clone-dir <path>     Local clone directory
--gh-pages-discord-webhook <url> Discord webhook for publish notifications
--gh-pages-window-days <n>      Days of digests to retain in the manifest (0 = 30)
```

Two more flags set up or reset the Pages structure, then exit without starting the
server:

```
--init-gh-pages    Create the branch if absent and seed the manifest and index
                   pages. Idempotent; existing files are kept.
--reinit-gh-pages  Erase the branch and local clone, then recreate from scratch.
                   Destructive; prompts for confirmation.
```

These mirror the `dlk publish init` / `dlk publish reinit` commands; use them when
the server holds the only configured token.

Example - enable for one run without touching `config.json`:

```sh
export DOWNLINK_GH_PAGES_TOKEN=github_pat_...
./server \
  --gh-pages-enabled \
  --gh-pages-repo https://github.com/you/you.github.io.git \
  --gh-pages-base-url https://you.github.io \
  --gh-pages-output-dir digests
```

---

## 6. What gets published

Each digest publishes under its [profile's](profiles.md) output subdirectory `<subdir>`.
For the default profile that is `output_dir` (or `digests` when empty); other profiles use
their `output_subdir`, the slug by default. On each digest generation:

- **`<subdir>/downlink-digest-YYYY-MM-DD_HHMM.html`** - self-contained HTML page for that digest, in the profile's theme (same file sent to Discord).
- **`<subdir>/downlink-swipe-YYYY-MM-DD_HHMM.html`** - self-contained mobile-friendly "swipe" triage view of the same digest, linked from the articles page. Shares its timestamp.
- **`<subdir>/manifest.json`** - machine-readable archive data for the profile's digest index, newest first. See [Manifest schema](#manifest-schema) below for every field.
- **`<subdir>/index.html`** - the profile's archive UI with latest-digest hero, search, filters, sort controls, log/grid/timeline layouts, keyboard navigation, and pinned digests in browser local storage.
- **`<subdir>/sources.html`** - the profile's feed list.
- **`<subdir>/reports.html`** - referenced reports aggregated across the profile's digests in the publish window. Linked from the digest, archive, and swipe page footers. Skipped when no report data is available.

The repo root depends on how many profiles are enabled:

- **One profile**: `index.html` is the root archive UI that loads `<subdir>/manifest.json`, as before.
- **More than one profile**: `index.html` is a landing page listing the profiles, alongside `profiles.json`; every published page also gets a floating profile switcher.

Publishing directly to the repo root is not supported. The browser-facing archive index
uses `manifest.json` directly; old manifests should be regenerated by publishing a new
digest or reinitializing the Pages structure.

### Manifest schema

Top-level `manifest.json` object:

| Field | Type | Description |
|---|---|---|
| `generated_at` | string | Timestamp when the manifest was last (re)generated. |
| `source_repo` | string | The Pages repository URL the manifest was written to. |
| `digests` | array | List of digest entries, newest first (see below). |

Each entry in `digests`:

| Field | Type | Description |
|---|---|---|
| `filename` | string | Digest HTML filename (e.g. `downlink-digest-2026-04-24_1200.html`). |
| `started_at` | string | When digest generation started. |
| `period_start` | string | Start of the time window the digest covers. Omitted when unset. |
| `time_window` | string | Human-readable description of the covered time window. |
| `article_count` | int | Total number of articles in the digest. |
| `must_count` | int | Articles tagged "Must Read". |
| `should_count` | int | Articles tagged "Should Read". |
| `may_count` | int | Articles tagged "May Read". |
| `opt_count` | int | Articles tagged "Optional". |
| `provider` | string | Analysis provider used for the digest summary. |
| `model` | string | Primary model name. |
| `models` | array | All unique model names across the summary and per-article analysis. Omitted when empty. |
| `title` | string | Digest title. Omitted when unset. |
| `headlines` | array | Headline strings shown in the archive index. |
| `headline_priorities` | array | Per-headline priority labels, aligned with `headlines`. Omitted when empty. |
| `summary` | string | Digest summary (markdown). |

---

## 7. Discord publish notification

The `discord_webhook_url` field is distinct from the main `notifications.discord.webhook_url`. It posts a one-line message when a page goes live:

```
📰 New digest published to GitHub Pages: https://you.github.io/digests/downlink-digest-2026-04-24_1200.html
```

If `base_url` is not set, the message omits the URL. Failure to send this notification is logged as a warning and does not affect the publish itself.

---

## 8. How the local clone works

On the first publish, Downlink clones the repository into `clone_dir` (shallow, depth 1). On subsequent runs it pulls the latest from origin before writing and committing. This means:

- The clone persists between server restarts - no full re-clone each time.
- If two concurrent digest generations race, the second push may be rejected as non-fast-forward; Downlink will pull and retry once automatically.
- If you need to force a fresh clone, delete the `clone_dir` directory.

---

## 9. Automatic publishing

Publishing is wired into the server-side digest pipeline, so no extra configuration is needed beyond `github_pages.enabled`. Any digest generated against the server - via `dlk digest generate` or the gRPC `DigestService` - is published automatically when `github_pages.enabled` is `true` in the server's `config.json`. To publish on a schedule, run `dlk digest generate` from cron (or any external scheduler) against the running server.
