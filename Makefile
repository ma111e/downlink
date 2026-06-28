.PHONY: docs server cli dev-digest preview assets

default: help

COMMIT := $(shell git rev-parse --short HEAD)
LDFLAGS := -X github.com/ma111e/downlink/pkg/version.Commit=$(COMMIT)

## all : Builds the server and cli
all: server cli

## proto: Generate grpc files
proto:
	protoc --go_out=./pkg/ --go-grpc_out=./pkg/ -I=./pkg/pb/ ./pkg/pb/*

## assets : Build the web assets (CSS/JS) embedded into the server via go:embed
assets:
	cd web && npm ci && npm run build

# First-run guard: build assets only if they're missing (fresh clone). Rebuild
# explicitly with `make assets` after editing anything under web/.
cmd/server/internal/notification/assets/digest.css:
	cd web && npm install && npm run build

## cli : Builds the cli from ./cmd/dlk
cli:
	go build -ldflags "$(LDFLAGS)" ./cmd/dlk

## server : Builds the server from ./cmd/server (builds web assets on first run)
server: cmd/server/internal/notification/assets/digest.css
	go build -ldflags "$(LDFLAGS)" ./cmd/server

## dev-digest : Live-reload preview of the digest templates + assets (needs air and node)
dev-digest:
	@command -v air >/dev/null 2>&1 || { echo "air not found. Install with: go install github.com/air-verse/air@latest"; exit 1; }
	@command -v npm >/dev/null 2>&1 || { echo "npm not found. Install Node.js to build web assets."; exit 1; }
	cd web && npm install
	cd web && npm run build
	@( cd web && npm run watch >/dev/null 2>&1 & )
	@( sleep 2 && xdg-open http://localhost:8099 >/dev/null 2>&1 & )
	air -c .air.toml

## preview : Render the digest sample pages to tmp/preview/ as static HTML (for screenshots)
preview: cmd/server/internal/notification/assets/digest.css
	go run ./cmd/server dev digest --export tmp/preview

## help : Shows this help
help: Makefile
	@printf ">] DOWNLINK\n\n"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@printf ""
