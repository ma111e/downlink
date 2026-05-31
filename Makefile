.PHONY: docs

default: help

## all : Builds the program for both architecture
all: ui server cli

## proto: Generate grpc files
proto:
	protoc --go_out=./pkg/ --go-grpc_out=./pkg/ -I=./pkg/pb/ ./pkg/pb/*

## cli : Builds the cli from ./cmd/dlk
cli:
	go build ./cmd/dlk

## ui : Builds the ui from ./cmd/ui
ui:
	go build ./cmd/ui

## server : Builds the server from ./cmd/server
server:
	go build ./cmd/server


## help : Shows this help
help: Makefile
	@printf ">] DOWNLINK\n\n"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@printf ""
