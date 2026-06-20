# Multi-stage build for Render. Build context is the repo root.
#
# Unlike the Railway image, Render only needs HTTP (port 8080) — no Quack
# TCP proxy port is required. Admin operations use the SURVEY_ADMIN_TOKEN
# via HTTP endpoints rather than Quack IPC.
#
# Runtime is debian:bookworm-slim (glibc) because duckdb-go-bindings/v2
# embeds a prebuilt libduckdb.so for linux-amd64 / linux-arm64 and extracts
# it to a temp dir at startup. Alpine (musl) would mismatch the libc and
# fail to dlopen.

# --- Build ---
FROM golang:1.24-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/survey ./cmd/survey

# --- Runtime ---
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/db/survey

COPY --from=build /out/survey /usr/local/bin/survey

# Image-level defaults. Render sets PORT for HTTP, but we use a fixed port
# and configure the Health Check Path and HTTP Port in render.yaml.
ENV SURVEY_HTTP_ADDR=0.0.0.0:8080 \
    SURVEY_DB_PATH=/var/db/survey/votes.duckdb \
    SURVEY_SITE_URL=https://pollmd.ssp.sh

WORKDIR /var/db/survey

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/survey"]
