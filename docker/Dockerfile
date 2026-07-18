FROM golang:1.26.5-trixie@sha256:117e07f49461abb984fc8aef661432461ff43d06faa22c3b73af6a49ce325cb9 AS builder

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
        gcc \
        libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

ARG TARGETARCH
ARG APP_VERSION="dev"

RUN GOOS=linux GOARCH="${TARGETARCH}" CGO_ENABLED=1 \
    go build -tags goolm -trimpath -ldflags="-s -w -X 'main.AppVersion=${APP_VERSION}'" \
    -o /matrix-cli ./cmd/matrix-cli

FROM debian:trixie-slim@sha256:020c0d20b9880058cbe785a9db107156c3c75c2ac944a6aa7ab59f2add76a7bd

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
        netcat-openbsd \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 10001 -s /bin/bash bot

RUN mkdir -p /home/bot/.ssh /home/bot/keys /home/bot/data \
    && chown -R bot:bot /home/bot

COPY --from=builder /matrix-cli /usr/local/bin/matrix-cli
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh

EXPOSE 2222
USER bot
ENTRYPOINT ["/entrypoint.sh"]
