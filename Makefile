.PHONY: docs

default: help

## all : Builds the program for both architecture
all: ui server cli

## proto: Generate grpc files
proto:
	protoc --go_out=./pkg/ --go-grpc_out=./pkg/ -I=./pkg/pb/ ./pkg/pb/*

## cli : Builds the cli from ./cmd/cli
cli:
	go build ./cmd/cli

## ui : Builds the ui from ./cmd/ui
ui:
	go build ./cmd/ui

## server : Builds the server from ./cmd/server
server:
	go build ./cmd/server

## solimen-image : Build and increment solimen Docker image version
solimen-image:
	@VERSION=$$(cat docker/solimen/.version); \
	MAJOR=$$(echo $$VERSION | cut -d. -f1); \
	MINOR=$$(echo $$VERSION | cut -d. -f2); \
	PATCH=$$(echo $$VERSION | cut -d. -f3); \
	NEW_VERSION=$$MAJOR.$$MINOR.$$((PATCH + 1)); \
	echo "Building solimen:$$NEW_VERSION..."; \
	docker build -t ghcr.io/ma111e/solimen:latest -t ghcr.io/ma111e/solimen:$$NEW_VERSION --build-arg VERSION=$$NEW_VERSION -f ./docker/solimen/Dockerfile . && \
	echo $$NEW_VERSION > docker/solimen/.version && \
	echo "✓ Built and tagged as $$NEW_VERSION"

## solimen-push : Push solimen Docker image to registry
solimen-push:
	@VERSION=$$(cat docker/solimen/.version); \
	echo "Pushing ghcr.io/ma111e/solimen:latest and :$$VERSION..."; \
	docker push ghcr.io/ma111e/solimen:latest && \
	docker push ghcr.io/ma111e/solimen:$$VERSION && \
	echo "✓ Pushed solimen:$$VERSION"

## solimen-build-push : Build, increment version, and push solimen Docker image
solimen-build-push: solimen-image solimen-push

## help : Shows this help
help: Makefile
	@printf ">] DOWNLINK\n\n"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@printf ""
