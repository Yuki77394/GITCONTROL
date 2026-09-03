# SWAGGYMUSIC GitHub Controller Bot — Dockerfile
# Multi-stage build: build the Go binary in a builder stage, then copy it
# into a minimal final image to reduce attack surface.

# ---------- Builder ----------
FROM golang:1.23-alpine AS builder

# Install git (needed by go modules) and ca-certificates.
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Cache module downloads: copy go.mod/go.sum first and download deps.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# Build a static binary with stripped debug info.
# CGO_ENABLED=0 ensures a static binary that runs in scratch/distroless.
# -ldflags="-s -w" strips symbol tables and DWARF info.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/swaggymusic-bot \
    ./cmd/bot

# ---------- Runtime ----------
FROM alpine:3.20

# Install ca-certificates so HTTPS to api.github.com works, tzdata so
# time operations use the configured timezone, and wget for the health
# check (alpine doesn't include it by default).
RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -u 10001 -h /app swaggymusic

WORKDIR /app

# Copy the binary from the builder stage.
COPY --from=builder /out/swaggymusic-bot /app/swaggymusic-bot

# Drop privileges.
USER swaggymusic

# Heroku assigns PORT at runtime via env var. The app reads PORT from the
# environment (default 8080 for local dev). We do NOT hardcode EXPOSE
# because Heroku ignores it and routes traffic to $PORT dynamically.
# For local Docker runs, EXPOSE 8080 is still useful as documentation.
ENV PORT=8080
EXPOSE 8080

# Health check: hit the /health endpoint (added for Heroku + general
# observability). Heroku ignores Docker HEALTHCHECK (it uses its own
# health checks based on the PORT binding), but this is useful for
# local Docker runs and other orchestrators.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${PORT}/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/swaggymusic-bot"]
