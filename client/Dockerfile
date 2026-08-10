FROM golang:1.26-bookworm AS wireproxy-builder

ARG WIREPROXY_VERSION=v1.1.2

RUN CGO_ENABLED=0 go install github.com/windtf/wireproxy/cmd/wireproxy@${WIREPROXY_VERSION}

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        dumb-init \
    && rm -rf /var/lib/apt/lists/*

COPY --from=wireproxy-builder /go/bin/wireproxy /usr/local/bin/wireproxy
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chmod 0755 /usr/local/bin/entrypoint.sh /usr/local/bin/wireproxy

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/entrypoint.sh"]
