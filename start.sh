#!/bin/bash
set -euo pipefail
./mofufans &
api_pid=$!
nginx -g 'daemon off;' &
web_pid=$!
trap 'kill "$api_pid" "$web_pid" 2>/dev/null || true; wait || true' EXIT
trap 'exit 0' TERM INT
set +e
wait -n "$api_pid" "$web_pid"
status=$?
exit "$status"
