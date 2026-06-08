# Deployment

Downlink runs as a long-lived server with the `dlk` client (and any cron jobs) talking to
it over gRPC.

## Ports

| Port | Purpose |
|---|---|
| `50051` | gRPC API (the `dlk` client connects here). |
| `65261` | Atom feed export of analyzed articles. |

## Docker

A `Dockerfile` and `docker-compose.yml` are provided. Compose also wires up Solimen for
`full_browser` scraping:

```sh
docker compose up -d
```

The compose service mounts `./config.json` and `./feeds.yml` into the container, persists
data in a named volume, sets `DOWNLINK_HOST=0.0.0.0`, and points
`DOWNLINK_SOLIMEN_ADDR` at the Solimen container. To use the published image instead of
building, set `image: ghcr.io/ma111e/downlink:latest` on the `downlink` service.

For `dynamic` scraping you also need Lightpanda; add it alongside Solimen or let the
server start it with `--auto-start-lightpanda` where Docker is available.

## systemd

A sample unit is at [etc/downlink.service](../etc/downlink.service) for running the server
under `/opt/downlink`. It expects `config.json` and `feeds.yml` next to the binary. Adjust
paths and the `DOWNLINK_*` environment to taste, then:

```sh
sudo systemctl enable --now downlink
journalctl -u downlink -f
```

## Configuration in production

Prefer environment variables for secrets and host-specific values; they override
`config.json` and keep tokens out of files. The GitHub Pages token in particular should
come from `DOWNLINK_GH_PAGES_TOKEN`. See [configuration.md](configuration.md) for the full
variable list and precedence.

## Atom feed export

Alongside the gRPC API, the server exposes analyzed articles as Atom feeds over HTTP on
`:65261`, so you can read them in any feed reader:

| Path | Returns |
|---|---|
| `/` | HTML index linking every feed. |
| `/feeds/<feed-name>` | Atom XML of that feed's articles. The name is the normalized feed title (lowercase, spaces to hyphens). |

```sh
curl http://localhost:65261/                       # list feeds
curl http://localhost:65261/feeds/the-hacker-news  # one feed as Atom
```

The port is fixed. Expose it (or put it behind a reverse proxy) only if you want the
feeds reachable beyond the host.

## Scheduling digests

The server does not generate digests on a timer. Drive it from cron (or any scheduler) by
running the client against the running server:

```cron
0 7 * * *  cd /opt/downlink && ./dlk digest generate --refresh-feeds
```

When `github_pages.enabled` is true, each generated digest is published automatically, so
a single cron entry keeps the site current. See [github-pages.md](github-pages.md).
