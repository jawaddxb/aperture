# Aperture — Multi-stage Docker build
# Stage 1: Build Go binary
FROM golang:1.26-bookworm AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o aperture-server ./cmd/aperture-server

# Stage 2: Runtime with Chromium + Python (for Scrapling fallback)
FROM debian:bookworm-slim

# Install Chromium, Python, and dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    python3 \
    python3-pip \
    python3-venv \
    ca-certificates \
    fonts-liberation \
    libnss3 \
    libatk-bridge2.0-0 \
    libdrm2 \
    libxkbcommon0 \
    libgbm1 \
    libasound2 \
    && rm -rf /var/lib/apt/lists/*

# Install Scrapling for anti-bot fallback
RUN python3 -m venv /opt/scrapling && \
    /opt/scrapling/bin/pip install --no-cache-dir "scrapling[all]" && \
    /opt/scrapling/bin/scrapling install

# Copy binary and static assets
WORKDIR /app
COPY --from=builder /build/aperture-server .
COPY --from=builder /build/internal/api/static ./internal/api/static
COPY --from=builder /build/tools ./tools

# Default config (override with volume mount or env vars)
ENV APERTURE_SERVER_HOST=0.0.0.0
ENV APERTURE_SERVER_PORT=8080
ENV APERTURE_BROWSER_CHROMIUM_PATH=/usr/bin/chromium
ENV APERTURE_BROWSER_POOL_SIZE=3
ENV APERTURE_STEALTH_ENABLED=true
ENV PATH="/opt/scrapling/bin:${PATH}"

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD curl -sf http://localhost:8080/health || exit 1

ENTRYPOINT ["./aperture-server"]
