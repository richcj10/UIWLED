#!/usr/bin/with-contenv sh
set -e

CONFIG_PATH=/data/options.json
export UIWLED_CONFIG="$CONFIG_PATH"

exec /usr/local/bin/uiwled
