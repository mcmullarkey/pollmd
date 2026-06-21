# Root Dockerfile for Render manual Web Service.
# Build context is the repo root (/) so cmd/ and internal/ are accessible.
# Uses debian:bookworm-slim (glibc) because duckdb-go-bindings/v2 embeds
# a prebuilt libduckdb.so that requires glibc.

# --- Build ---
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/survey ./cmd/survey

# --- Runtime ---
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /var/db/survey /var/data
COPY --from=build /out/survey /usr/local/bin/survey
EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/survey"]
