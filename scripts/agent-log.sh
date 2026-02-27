#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <implementation|security-review|code-review> <message...>" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LATEST_FILE="${ROOT_DIR}/.agent-logs/latest-run"
ROLE="$1"
shift

if [[ ! -f "${LATEST_FILE}" ]]; then
  echo "latest run file not found: ${LATEST_FILE}" >&2
  exit 1
fi

RUN_DIR="$(cat "${LATEST_FILE}")"
LOG_FILE="${RUN_DIR}/${ROLE}.log"

if [[ ! -e "${LOG_FILE}" ]]; then
  echo "unknown role or log file missing: ${ROLE}" >&2
  exit 1
fi

printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >>"${LOG_FILE}"
