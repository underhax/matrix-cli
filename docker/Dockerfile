FROM golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4 AS builder

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

FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f

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
