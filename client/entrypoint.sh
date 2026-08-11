#!/bin/sh
set -eu

CONFIG_FILE=/tmp/wireproxy.conf
umask 077

configgen > "$CONFIG_FILE"
chmod 0600 "$CONFIG_FILE"

wireproxy -n -c "$CONFIG_FILE"
exec wireproxy -c "$CONFIG_FILE"
