FROM golang:1.26-bookworm AS wireproxy-builder

ARG WIREPROXY_VERSION=v1.1.2

RUN CGO_ENABLED=0 go install github.com/windtf/wireproxy/cmd/wireproxy@${WIREPROXY_VERSION}

FROM golang:1.26-bookworm AS configgen-builder

WORKDIR /src

COPY go.mod ./
COPY configgen ./configgen

RUN go test ./... \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/configgen ./configgen

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        dumb-init \
    && rm -rf /var/lib/apt/lists/*

COPY --from=wireproxy-builder /go/bin/wireproxy /usr/local/bin/wireproxy
COPY --from=configgen-builder /out/configgen /usr/local/bin/configgen
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chmod 0755 /usr/local/bin/configgen /usr/local/bin/entrypoint.sh /usr/local/bin/wireproxy

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/entrypoint.sh"]
