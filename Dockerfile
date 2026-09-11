# syntax=docker/dockerfile:1

# ============================================================================
#  3x-ui launcher for Back4app Containers (free tier)
#  Runs the official 3x-ui panel behind a small path-based reverse proxy on
#  the PORT environment variable that Back4app provides (default 3000).
# ============================================================================

# ---------- Stage 1: build the tiny Go launcher ----------
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /launcher .

# ---------- Stage 2: runtime ----------
# IMPORTANT: the official 3x-ui binary is built against glibc, so the runtime
# image MUST be Debian-based (an Alpine/musl image would crash it).
FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates wget tar \
    && rm -rf /var/lib/apt/lists/*

# Official 3x-ui release, pinned to an exact version + SHA-256 checksum
# (verified at build time instead of blindly pulling "latest").
ARG XUI_VERSION=v3.7.0
ARG XUI_SHA256=0f8dd7baef3458f6591574e24814f322cf7f5e1e27f0a594683745e50be84ec5
RUN wget -q "https://github.com/MHSanaei/3x-ui/releases/download/${XUI_VERSION}/x-ui-linux-amd64.tar.gz" -O /tmp/x-ui.tar.gz \
 && echo "${XUI_SHA256}  /tmp/x-ui.tar.gz" | sha256sum -c - \
 && mkdir -p /app \
 && tar -xzf /tmp/x-ui.tar.gz -C /app \
 && rm -f /tmp/x-ui.tar.gz \
 && chmod +x /app/x-ui/x-ui /app/x-ui/bin/xray-linux-amd64

RUN mkdir -p /data && chmod -R a+rwX /app /data

COPY --from=build /launcher /launcher

ENV XUI_DIR=/app/x-ui \
    XUI_DB_FOLDER=/data \
    XUI_LOG_FOLDER=/data/logs \
    XUI_PORT=20530 \
    SUB_PORT=2096 \
    VLESS_PORT=20868 \
    WS_PREFIX=/ws/ \
    ADMIN_PATH=/panel/ \
    SUB_PREFIXES=/sub/ /json/ /clash/ /assets/

WORKDIR /app/x-ui
EXPOSE 3000
ENTRYPOINT ["/launcher"]
