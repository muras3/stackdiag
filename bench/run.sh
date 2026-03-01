#!/bin/bash
set -euo pipefail

# run.sh — Orchestrate the full benchmark suite.
# Usage: ./run.sh [--dry-run]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${SCRIPT_DIR}"

DRY_RUN=false
if [ "${1:-}" = "--dry-run" ]; then
  DRY_RUN=true
fi

SCENARIOS_FILE="scenarios.json"
RESULTS_BASE="results"
RUN_TIMESTAMP="$(date +%Y-%m-%dT%H%M%S)"
RESULTS_DIR="${RESULTS_BASE}/${RUN_TIMESTAMP}"

# Activate venv if present (for tiktoken / anthropic dependencies)
if [ -f "${SCRIPT_DIR}/.venv/bin/activate" ]; then
  # shellcheck disable=SC1091
  source "${SCRIPT_DIR}/.venv/bin/activate"
fi

# Read scenario names, targets, and extra_flags from scenarios.json.
# Uses python3 for JSON parsing (portable, no jq dependency).
# Output format: name\ttarget\textra_flags (space-separated)
read_scenarios() {
  SCENARIOS_FILE="${SCENARIOS_FILE}" python3 -c "
import os, json
with open(os.environ['SCENARIOS_FILE']) as f:
    scenarios = json.load(f)
for s in scenarios:
    flags = ' '.join(s.get('extra_flags', []))
    print(s['name'] + '\t' + s['target'] + '\t' + flags)
"
}

log() {
  echo "[bench] $*" >&2
}

run_cmd() {
  if [ "${DRY_RUN}" = true ]; then
    echo "[dry-run] $*" >&2
  else
    "$@"
  fi
}

# --- Main ---

log "Reading scenarios from ${SCENARIOS_FILE}..."
SCENARIO_LIST="$(read_scenarios)"
SCENARIO_COUNT="$(echo "${SCENARIO_LIST}" | wc -l | tr -d ' ')"
log "Found ${SCENARIO_COUNT} scenarios"

log "Generating TLS certificates..."
run_cmd bash docker/certs/generate.sh

log "Starting Docker services..."
run_cmd docker compose up -d --build

if [ "${DRY_RUN}" = false ]; then
  log "Waiting for services to be ready..."
  sleep 5
fi

log "Running capture for each scenario..."
while IFS=$'\t' read -r NAME TARGET EXTRA_FLAGS; do
  log "  -> ${NAME}: ${TARGET} ${EXTRA_FLAGS}"
  if [ -n "${EXTRA_FLAGS}" ]; then
    # shellcheck disable=SC2086
    run_cmd docker compose exec -T runner ./capture.sh "${NAME}" "${TARGET}" /bench/results ${EXTRA_FLAGS} </dev/null
  else
    run_cmd docker compose exec -T runner ./capture.sh "${NAME}" "${TARGET}" /bench/results </dev/null
  fi
done <<< "${SCENARIO_LIST}"

log "Copying results to ${RESULTS_DIR}..."
mkdir -p "${RESULTS_DIR}"
run_cmd docker compose cp runner:/bench/results/. "${RESULTS_DIR}/"

log "Counting tokens..."
while IFS=$'\t' read -r NAME _TARGET; do
  if [ "${DRY_RUN}" = true ]; then
    echo "[dry-run] python3 count_tokens.py ${RESULTS_DIR}/${NAME}/stdiag.json > ${RESULTS_DIR}/${NAME}/stdiag.tokens.json" >&2
    echo "[dry-run] python3 count_tokens.py ${RESULTS_DIR}/${NAME}/manual.txt > ${RESULTS_DIR}/${NAME}/manual.tokens.json" >&2
  else
    python3 count_tokens.py "${RESULTS_DIR}/${NAME}/stdiag.json" \
      > "${RESULTS_DIR}/${NAME}/stdiag.tokens.json"
    python3 count_tokens.py "${RESULTS_DIR}/${NAME}/manual.txt" \
      > "${RESULTS_DIR}/${NAME}/manual.tokens.json"
  fi
done <<< "${SCENARIO_LIST}"

log "Running evaluation..."
if [ "${DRY_RUN}" = true ]; then
  run_cmd python3 evaluate.py --scenarios "${SCENARIOS_FILE}" --results-dir "${RESULTS_DIR}/"
else
  python3 evaluate.py --scenarios "${SCENARIOS_FILE}" --results-dir "${RESULTS_DIR}/" \
    | tee "${RESULTS_DIR}/evaluation.json"
fi

log "Tearing down Docker services..."
run_cmd docker compose down

# Update 'latest' symlink (never overwrites past runs)
ln -sfn "${RUN_TIMESTAMP}" "${RESULTS_BASE}/latest"

log "Results saved to ${RESULTS_DIR}"
log "Done."
