FROM golang:1.27.1-trixie@sha256:433790e515d27dc6003e847e644cc0af956985cf315c1c58a3b73ee2dd305183 AS builder

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

FROM debian:trixie-slim@sha256:d7e12182ce18b85b93007c1dedf31f2d29e01ccf3182cc4017c709b6259bc132

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
        netcat-openbsd \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 10001 -s /bin/bash bot

RUN mkdir -p /home/bot/.ssh /home/bot/keys /home/bot/.config/matrix-cli \
    && chown -R bot:bot /home/bot

COPY --from=builder /matrix-cli /usr/local/bin/matrix-cli
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh

EXPOSE 2222
USER 10001
ENTRYPOINT ["/entrypoint.sh"]
