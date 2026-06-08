# Getting started

This walks through a first run from a clean checkout. The project README has the same path
in condensed form; this page adds the details it skips.

## Install

With Go 1.25+:

```sh
go install github.com/ma111e/downlink/cmd/dlk@latest     # CLI
go install github.com/ma111e/downlink/cmd/server@latest  # server
```

Or build from source: `make all` produces `./server` and `./dlk`. See the README for the
Docker path.

## 1. Configure

```sh
cp config.example.json config.json
cp feeds.example.yml feeds.yml
```

Enable at least one LLM provider in `config.json` and fill in its key. Analysis needs a
provider; fetching feeds does not, so you can defer this if you only want to pull
articles. See [llm-providers.md](llm-providers.md) and [configuration.md](configuration.md).

Edit `feeds.yml` to point at the feeds you want. A feed needs at least `url`, `type`,
`enabled`, and (for non-RSS-complete sources) a way to scrape the body. See
[feeds-and-scraping.md](feeds-and-scraping.md).

## 2. Start the server

```sh
./server
```

It listens for gRPC on `:50051` and serves an Atom export on `:65261`. If your feeds use
`scraping: dynamic` or `full_browser`, start the scrapers too (needs Docker):

```sh
./server --auto-start-lightpanda --auto-start-solimen
```

The server logs `gRPC server started` once it is up. `dlk config show` against it confirms
the client can reach it.

## 3. Apply feeds

`dlk feeds apply` reconciles the database to your file: feeds in the file are created or
updated, and feeds no longer listed are disabled (their articles are kept). Preview first:

```sh
./dlk feeds apply -f feeds.yml --dry-run
./dlk feeds apply -f feeds.yml
```

## 4. Fetch and generate a digest

```sh
./dlk feeds refresh all   # pull the latest articles
./dlk digest generate     # analyze unscored articles, then assemble the ranked digest
```

`digest generate` covers the last 24 hours by default. See [digests.md](digests.md) for
windows and options.

## 5. View

```sh
./dlk digest list
./dlk digest get          # pick one; add --markdown for prose
```

The SQLite database lives at `db_path` (default `./downlink.db`). Deleting it resets all
stored feeds, articles, and digests.

## Next

- [configuration.md](configuration.md) full config reference.
- [cli-reference.md](cli-reference.md) every `dlk` command.
- [github-pages.md](github-pages.md) publish digests to a website.
- [deployment.md](deployment.md) run it as a service, and read articles via the Atom export on `:65261`.
