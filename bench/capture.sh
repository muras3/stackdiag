#!/bin/bash
set -euo pipefail

# capture.sh — Run stdiag and manual commands for a single scenario.
# Usage: ./capture.sh <scenario_name> <target> <results_dir> [extra_flags...]

if [ $# -lt 3 ]; then
  echo "Usage: $0 <scenario_name> <target> <results_dir> [extra_flags...]" >&2
  exit 1
fi

SCENARIO_NAME="$1"
TARGET="$2"
RESULTS_DIR="$3"
shift 3
EXTRA_FLAGS=("$@")

OUTDIR="${RESULTS_DIR}/${SCENARIO_NAME}"
mkdir -p "${OUTDIR}"

# --- stdiag capture ---
# Always succeed: we want the JSON output even on non-zero exit codes.
stdiag --json "${TARGET}" "${EXTRA_FLAGS[@]+"${EXTRA_FLAGS[@]}"}" > "${OUTDIR}/stdiag.json" 2>/dev/null || true

# --- Manual commands capture ---

# Parse host and port from target URL.
# Supported schemes: https://, http://, tcp://
SCHEME="${TARGET%%://*}"
HOSTPORT="${TARGET#*://}"
HOST="${HOSTPORT%%:*}"
# Strip trailing path from host if present
HOST="${HOST%%/*}"

# Determine port (default based on scheme)
case "${SCHEME}" in
  https) DEFAULT_PORT=443 ;;
  http)  DEFAULT_PORT=80 ;;
  tcp)   DEFAULT_PORT=80 ;;
  *)     DEFAULT_PORT=443 ;;
esac

# Extract explicit port if present (host:port/path)
if [[ "${HOSTPORT}" == *":"* ]]; then
  PORT_AND_PATH="${HOSTPORT#*:}"
  PORT="${PORT_AND_PATH%%/*}"
else
  PORT="${DEFAULT_PORT}"
fi

# Parse extra flags for manual command adjustments
COUNT=1
TLS_SCAN=false
DNS_SERVER=""
for i in "${!EXTRA_FLAGS[@]}"; do
  case "${EXTRA_FLAGS[$i]}" in
    --count)
      COUNT="${EXTRA_FLAGS[$((i+1))]}"
      ;;
    --tls-scan)
      TLS_SCAN=true
      ;;
    --dns-server)
      DNS_SERVER="${EXTRA_FLAGS[$((i+1))]}"
      ;;
  esac
done

run_manual_once() {
  local iteration="$1"
  if [ "${COUNT}" -gt 1 ]; then
    echo "=== Attempt ${iteration}/${COUNT} ==="
    echo ""
  fi

  # DNS: use custom server if specified
  if [ -n "${DNS_SERVER}" ]; then
    echo "=== dig @${DNS_SERVER} ${HOST} ==="
    dig "@${DNS_SERVER}" "${HOST}" 2>&1 || true
  else
    echo "=== dig ${HOST} ==="
    dig "${HOST}" 2>&1 || true
  fi
  echo ""

  # Reachability check (ping)
  echo "=== ping -c 3 -W 3 ${HOST} ==="
  ping -c 3 -W 3 "${HOST}" 2>&1 || true
  echo ""

  if [ "${SCHEME}" = "tcp" ]; then
    echo "=== nc -zv -w 5 ${HOST} ${PORT} ==="
    if nc -zv -w 5 "${HOST}" "${PORT}" 2>&1; then
      echo "Connection to ${HOST} ${PORT} port [tcp/*] succeeded!"
    else
      echo "nc: connect to ${HOST} port ${PORT} (tcp) failed (exit code $?)"
    fi
  else
    if [ "${SCHEME}" = "https" ]; then
      echo "=== openssl s_client -connect ${HOST}:${PORT} ==="
      openssl s_client -connect "${HOST}:${PORT}" -servername "${HOST}" < /dev/null 2>&1 || true
      echo ""

      # TLS scan: probe each version manually
      if [ "${TLS_SCAN}" = true ]; then
        for tlsver in tls1 tls1_1 tls1_2 tls1_3; do
          echo "=== openssl s_client -${tlsver} -connect ${HOST}:${PORT} ==="
          openssl s_client -"${tlsver}" -connect "${HOST}:${PORT}" -servername "${HOST}" < /dev/null 2>&1 || true
          echo ""
        done
      fi
    fi

    echo "=== curl -sv --max-redirs 0 --connect-timeout 10 ${TARGET} ==="
    curl -sv --max-redirs 0 --connect-timeout 10 "${TARGET}" 2>&1 || true
  fi
}

{
  for ((i=1; i<=COUNT; i++)); do
    run_manual_once "${i}"
  done
} > "${OUTDIR}/manual.txt"

echo "Captured: ${SCENARIO_NAME} -> ${OUTDIR}" >&2
