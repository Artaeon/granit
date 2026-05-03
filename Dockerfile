# syntax=docker/dockerfile:1.7
# ── Stage 1: build the SvelteKit SPA ─────────────────────────────────
# Pinned to Node 22 LTS — pnpm 9 + the svelte-adapter-static toolchain
# we use have been smoke-tested on 22. Bump intentionally, not silently.
FROM node:22-alpine AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# `pnpm build` writes the static output into ../internal/serveapi/dist
# (see svelte.config.js). The Go build below picks it up via go:embed.
RUN pnpm build

# ── Stage 2: build the Go binary ─────────────────────────────────────
# go.mod requires 1.25.0 — keep this exact to avoid drift.
FROM golang:1.25-alpine AS go
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Bring in the SPA built above so go:embed has something to embed.
COPY --from=web /src/internal/serveapi/dist ./internal/serveapi/dist
ARG VERSION=docker
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/granit ./cmd/granit/

# ── Stage 3: runtime ─────────────────────────────────────────────────
# Alpine for the `git` binary granit web --sync shells out to. If you
# don't need git auto-sync, swap to gcr.io/distroless/static for a
# ~12 MB final image.
FROM alpine:3.20
RUN apk add --no-cache git ca-certificates tzdata && \
    addgroup -S granit && adduser -S granit -G granit
WORKDIR /vault
COPY --from=go /out/granit /usr/local/bin/granit
USER granit
EXPOSE 8787
# /vault is the bind-mount target. --sync is opt-in via env (see
# docker-compose.example.yml) so a missing remote doesn't error on
# first boot. PORT env is honoured by `granit web` directly.
ENV PORT=8787
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q -O- http://127.0.0.1:${PORT}/api/v1/health || exit 1
ENTRYPOINT ["granit"]
CMD ["web", "--addr", "0.0.0.0:8787", "/vault"]
