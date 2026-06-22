# syntax=docker/dockerfile:1

# ----- build stage (CGO required for SQLite via mattn/go-sqlite3) -----
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache build-base git

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=dev
ARG DATE=unknown
ENV CGO_ENABLED=1
ENV LDFLAGS="-s -w \
      -X github.com/ma111e/downlink/pkg/version.Version=${VERSION} \
      -X github.com/ma111e/downlink/pkg/version.Commit=${COMMIT} \
      -X github.com/ma111e/downlink/pkg/version.Date=${DATE}"
RUN go build -ldflags "${LDFLAGS}" -o /out/server ./cmd/server \
 && go build -ldflags "${LDFLAGS}" -o /out/dlk ./cmd/dlk

# ----- runtime stage -----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && adduser -D -u 10001 downlink

WORKDIR /app
COPY --from=builder /out/server /usr/local/bin/server
COPY --from=builder /out/dlk /usr/local/bin/dlk

# Mount config.json / feeds.yml / .env into /app at runtime.
USER downlink

# gRPC API.
EXPOSE 50051

ENTRYPOINT ["server"]
