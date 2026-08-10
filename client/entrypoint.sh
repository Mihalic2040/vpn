#!/bin/sh
set -eu

: "${WG_PRIVATE_KEY:?WG_PRIVATE_KEY is required}"
: "${WG_ADDRESS:?WG_ADDRESS is required, for example 10.8.0.2/32}"
: "${WG_SERVER_PUBLIC_KEY:?WG_SERVER_PUBLIC_KEY is required}"
: "${WG_PRESHARED_KEY:?WG_PRESHARED_KEY is required}"
: "${WG_ENDPOINT:?WG_ENDPOINT is required, for example vpn.example.com:51820}"
: "${WG_ALLOWED_IPS:?WG_ALLOWED_IPS is required, for example 10.8.0.0/24}"
: "${TARGET_HOST:?TARGET_HOST is required, for example 10.8.0.3}"
: "${TARGET_PORT:?TARGET_PORT is required, for example 8080}"

PORT="${PORT:-8080}"
WG_PERSISTENT_KEEPALIVE="${WG_PERSISTENT_KEEPALIVE:-25}"

case "$PORT" in
    *[!0-9]*|'')
        echo "PORT must be a numeric TCP port" >&2
        exit 1
        ;;
esac

case "$TARGET_PORT" in
    *[!0-9]*|'')
        echo "TARGET_PORT must be a numeric TCP port" >&2
        exit 1
        ;;
esac

CONFIG_FILE=/tmp/wireproxy.conf
umask 077

{
    printf '%s\n' '[Interface]'
    printf 'Address = %s\n' "$WG_ADDRESS"
    printf 'PrivateKey = %s\n' "$WG_PRIVATE_KEY"
    printf '%s\n' '' '[Peer]'
    printf 'PublicKey = %s\n' "$WG_SERVER_PUBLIC_KEY"
    printf 'PresharedKey = %s\n' "$WG_PRESHARED_KEY"
    printf 'Endpoint = %s\n' "$WG_ENDPOINT"
    printf 'AllowedIPs = %s\n' "$WG_ALLOWED_IPS"
    printf 'PersistentKeepalive = %s\n' "$WG_PERSISTENT_KEEPALIVE"
    printf '%s\n' '' '[TCPClientTunnel]'
    printf 'BindAddress = 0.0.0.0:%s\n' "$PORT"
    printf 'Target = %s:%s\n' "$TARGET_HOST" "$TARGET_PORT"
} > "$CONFIG_FILE"

wireproxy -n -c "$CONFIG_FILE"
exec wireproxy -c "$CONFIG_FILE"
