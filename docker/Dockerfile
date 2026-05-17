# =============================================================================
# Build Stage - Compile Go binary
# =============================================================================
FROM golang:alpine AS builder

# Install git and bash (git: required for go mod download; bash: for build scripts)
RUN apk add --no-cache git bash

ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID
ARG OFFICIALSITE=https://wthr.top

WORKDIR /build

# Copy go.mod first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X 'main.Version=${VERSION}' -X 'main.CommitID=${COMMIT_ID}' -X 'main.BuildDate=${BUILD_DATE}' -X 'main.OfficialSite=${OFFICIALSITE}'" \
    -o /build/binary/wthr ./src

# =============================================================================
# Runtime Stage - Minimal Alpine image
# =============================================================================
FROM alpine:latest

# ARGs for build-time values (set by docker build --build-arg)
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID
ARG LICENSE=MIT

# Static Labels (NON-NEGOTIABLE per AI.md PART 27)
LABEL maintainer="Weather Service Contributors <noreply@github.com>" \
      org.opencontainers.image.vendor="casapps" \
      org.opencontainers.image.authors="casapps" \
      org.opencontainers.image.title="wthr" \
      org.opencontainers.image.base.name="wthr" \
      org.opencontainers.image.description="Containerized version of wthr" \
      org.opencontainers.image.url="https://github.com/casapps/wthr" \
      org.opencontainers.image.source="https://github.com/casapps/wthr" \
      org.opencontainers.image.documentation="https://github.com/casapps/wthr" \
      org.opencontainers.image.vcs-type="Git" \
      com.github.containers.toolbox="false"

# Dynamic Labels (from ARGs)
LABEL org.opencontainers.image.licenses="${LICENSE}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.schema-version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT_ID}"

# Install required packages
# NOTE: Tor binary installed but NOT configured here - binary handles all Tor setup (see PART 32)
RUN apk add --no-cache \
    git \
    curl \
    bash \
    tini \
    tor

# NO directory creation - binary handles ALL setup based on env vars
# This allows users to override CONFIG_DIR, DATA_DIR, etc.
# Docker volume mounts auto-create mount points

# Copy binary from builder stage (multi-stage build)
COPY --from=builder /build/binary/wthr /usr/local/bin/wthr

# Copy BUILD-TIME overlay (entrypoint.sh) from docker/file_system/ into image
# Per AI.md PART 27: file_system/ is BUILD-TIME overlay, NOT runtime volumes
COPY docker/file_system/ /

# Copy Dockerfile to image (for reference and backup)
COPY docker/Dockerfile /root/Dockerfile

# Make all binaries/scripts in /usr/local/bin executable (755)
RUN chmod 755 /usr/local/bin/*

# Set environment
# Note: Tor available (binary installed) but server binary controls Tor startup (see PART 32)
ENV MODE=development \
    TZ=America/New_York

# Expose internal port (always 80)
EXPOSE 80

# Stop signal for graceful shutdown
STOPSIGNAL SIGRTMIN+3

# Health check (long start period for services that need initialization)
HEALTHCHECK --start-period=10m --interval=5m --timeout=15s --retries=3 \
    CMD /usr/local/bin/wthr --status || exit 1

# Use tini as init with signal propagation
# -p SIGTERM: propagate SIGTERM to child processes
ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]
