FROM golang:1.27.1-alpine3.24@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS builder

ARG VERSION=dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X github.com/zoster81/scripthold/filetoolsserver.Version=${VERSION}" \
    -o /out/scripthold \
    ./cmd/scripthold

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 mcp \
    && adduser -S -D -H -u 10001 -G mcp mcp \
    && mkdir -p /data /tmp/scripthold \
    && chown -R 10001:10001 /data /tmp/scripthold

COPY --from=builder --chown=10001:10001 /out/scripthold /usr/local/bin/scripthold

USER 10001:10001
WORKDIR /data
ENV HOME=/tmp/scripthold \
    TMPDIR=/tmp/scripthold

STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/scripthold"]
