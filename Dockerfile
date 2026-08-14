FROM golang:1.26.6-alpine3.24 AS builder

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

FROM alpine:3.24.1

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
