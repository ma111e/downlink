.PHONY: docs server cli dev-digest

default: help

COMMIT := $(shell git rev-parse --short HEAD)
LDFLAGS := -X github.com/ma111e/downlink/pkg/version.Commit=$(COMMIT)

## all : Builds the server and cli
all: server cli

## proto: Generate grpc files
proto:
	protoc --go_out=./pkg/ --go-grpc_out=./pkg/ -I=./pkg/pb/ ./pkg/pb/*

## cli : Builds the cli from ./cmd/dlk
cli:
	go build -ldflags "$(LDFLAGS)" ./cmd/dlk

## server : Builds the server from ./cmd/server
server:
	go build -ldflags "$(LDFLAGS)" ./cmd/server

## dev-digest : Live-reload preview of the digest templates (needs air: go install github.com/air-verse/air@latest)
dev-digest:
	@command -v air >/dev/null 2>&1 || { echo "air not found. Install with: go install github.com/air-verse/air@latest"; exit 1; }
	@( sleep 2 && xdg-open http://localhost:8099 >/dev/null 2>&1 & )
	air -c .air.toml

## help : Shows this help
help: Makefile
	@printf ">] DOWNLINK\n\n"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@printf ""
