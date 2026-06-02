.PHONY: docs server cli

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


## help : Shows this help
help: Makefile
	@printf ">] DOWNLINK\n\n"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@printf ""
