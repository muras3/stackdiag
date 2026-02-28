#!/bin/bash
# Self-verification script for bench Docker infrastructure.
# Run from inside the runner container, or from host via:
#   docker compose -f bench/docker-compose.yml exec runner bash /bench/verify.sh
set -euo pipefail

PASS=0
FAIL=0

check() {
  local desc=$1 expected=$2
  shift 2
  local output
  if output=$("$@" 2>&1); then
    if echo "$output" | grep -q "$expected"; then
      echo "  PASS: $desc"
      ((PASS++))
    else
      echo "  FAIL: $desc (expected '$expected' in output)"
      echo "        got: $(echo "$output" | head -3)"
      ((FAIL++))
    fi
  else
    if echo "$output" | grep -q "$expected"; then
      echo "  PASS: $desc"
      ((PASS++))
    else
      echo "  FAIL: $desc (command failed, expected '$expected')"
      echo "        got: $(echo "$output" | head -3)"
      ((FAIL++))
    fi
  fi
}

echo "=== Bench Infrastructure Verification ==="
echo ""

echo "[DNS]"
check "DNS resolves nginx-healthy.bench.local" "172.28.0.10" \
  dig +short nginx-healthy.bench.local @172.28.0.100
check "DNS NXDOMAIN for nonexistent.bench.local" "NXDOMAIN" \
  dig nonexistent.bench.local @172.28.0.100

echo ""
echo "[HTTPS - healthy]"
check "nginx-healthy returns 200" "200" \
  curl -sk -o /dev/null -w '%{http_code}' https://nginx-healthy

echo ""
echo "[HTTPS - error codes]"
check "nginx-503 returns 503" "503" \
  curl -sk -o /dev/null -w '%{http_code}' https://nginx-503
check "nginx-403 returns 403" "403" \
  curl -sk -o /dev/null -w '%{http_code}' https://nginx-403
check "nginx-500 returns 500" "500" \
  curl -sk -o /dev/null -w '%{http_code}' https://nginx-500

echo ""
echo "[TLS - failure scenarios]"
check "nginx-expired cert error" "certificate" \
  curl -s --cacert /dev/null https://nginx-expired
check "nginx-wronghost cert error" "certificate" \
  curl -s --resolve nginx-wronghost:443:172.28.0.15 https://nginx-wronghost
check "nginx-selfsigned cert error" "certificate" \
  curl -s https://nginx-selfsigned

echo ""
echo "[TLS - version]"
check "nginx-tls12 supports TLS 1.2" "TLSv1.2" \
  openssl s_client -connect nginx-tls12:443 -tls1_2 </dev/null

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] || exit 1
