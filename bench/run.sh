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
RESULTS_DIR="results"

# Read scenario names and targets from scenarios.json.
# Uses python3 for JSON parsing (portable, no jq dependency).
read_scenarios() {
  python3 -c "
import json
with open('${SCENARIOS_FILE}') as f:
    scenarios = json.load(f)
for s in scenarios:
    print(s['name'] + '\t' + s['target'])
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

log "Building stdiag binary..."
run_cmd go build -o docker/runner/stdiag ../cmd/stackdiag/

log "Starting Docker services..."
run_cmd docker compose up -d --build

if [ "${DRY_RUN}" = false ]; then
  log "Waiting for services to be ready..."
  sleep 5
fi

log "Running capture for each scenario..."
while IFS=$'\t' read -r NAME TARGET; do
  log "  -> ${NAME}: ${TARGET}"
  run_cmd docker compose exec runner ./capture.sh "${NAME}" "${TARGET}" /bench/results
done <<< "${SCENARIO_LIST}"

log "Copying results from container..."
rm -rf "${RESULTS_DIR}"
run_cmd docker compose cp runner:/bench/results "${RESULTS_DIR}/"

log "Counting tokens..."
while IFS=$'\t' read -r NAME _TARGET; do
  if [ "${DRY_RUN}" = true ]; then
    echo "[dry-run] python3 count_tokens.py ${RESULTS_DIR}/${NAME}/stdiag.json > ${RESULTS_DIR}/${NAME}/stdiag_tokens.json" >&2
    echo "[dry-run] python3 count_tokens.py ${RESULTS_DIR}/${NAME}/manual.txt > ${RESULTS_DIR}/${NAME}/manual_tokens.json" >&2
  else
    python3 count_tokens.py "${RESULTS_DIR}/${NAME}/stdiag.json" \
      > "${RESULTS_DIR}/${NAME}/stdiag_tokens.json"
    python3 count_tokens.py "${RESULTS_DIR}/${NAME}/manual.txt" \
      > "${RESULTS_DIR}/${NAME}/manual_tokens.json"
  fi
done <<< "${SCENARIO_LIST}"

log "Running evaluation..."
run_cmd python3 evaluate.py --scenarios "${SCENARIOS_FILE}" --results-dir "${RESULTS_DIR}/"

log "Tearing down Docker services..."
run_cmd docker compose down

log "Done."
