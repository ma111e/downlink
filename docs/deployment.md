# Deployment

Downlink runs as a long-lived server with the `dlk` client (and any cron jobs) talking to
it over gRPC.

## Ports

| Port | Purpose |
|---|---|
| `50051` | gRPC API (the `dlk` client connects here). |
| `127.0.0.1:65262` | LLM monitoring dashboard (`--admin-port`); localhost only, no auth. |

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

If you run [profiles](profiles.md), ship `profiles.yml` next to the binary (and a `layouts/`
directory for any custom layout packs); both are read at startup, so restart after editing.

## Monitoring

The server serves a read-only LLM monitoring dashboard on `127.0.0.1:<--admin-port>`
(default `65262`): recent generation runs with token totals and per-run prompt/response
detail. Filter to one profile's runs with `?profile=<slug>`.

## Scheduling digests

The server does not generate digests on a timer. Drive it from cron (or any scheduler) by
running the client against the running server:

```cron
0 7 * * *  cd /opt/downlink && ./dlk digest generate --refresh-feeds
```

When `github_pages.enabled` is true, each generated digest is published automatically, so
a single cron entry keeps the site current. See [github-pages.md](github-pages.md).
