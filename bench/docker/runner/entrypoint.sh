#!/bin/bash
set -euo pipefail

# Install bench CA into system trust store (mounted at runtime)
if [ -f /etc/bench/certs/ca.crt ]; then
  cp /etc/bench/certs/ca.crt /usr/local/share/ca-certificates/bench-ca.crt
  update-ca-certificates 2>/dev/null
fi

exec "$@"
