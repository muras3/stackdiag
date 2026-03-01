#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="${1:-$(date +%Y%m%d-%H%M%S)}"
RUN_DIR="${ROOT_DIR}/.agent-logs/${RUN_ID}"
LATEST_FILE="${ROOT_DIR}/.agent-logs/latest-run"

mkdir -p "${RUN_DIR}"
touch "${RUN_DIR}/implementation.log" "${RUN_DIR}/security-review.log" "${RUN_DIR}/code-review.log"
printf '%s\n' "${RUN_DIR}" >"${LATEST_FILE}"

if command -v tmux >/dev/null 2>&1; then
  if tmux has-session -t stackdiag-agents 2>/dev/null; then
    tmux kill-session -t stackdiag-agents
  fi
  tmux new-session -d -s stackdiag-agents "tail -F '${RUN_DIR}/implementation.log'"
  tmux split-window -h -t stackdiag-agents "tail -F '${RUN_DIR}/security-review.log'"
  tmux split-window -v -t stackdiag-agents "tail -F '${RUN_DIR}/code-review.log'"
  tmux select-layout -t stackdiag-agents tiled
  echo "Dashboard started."
  echo "Attach with: tmux attach -t stackdiag-agents"
else
  echo "tmux not found. Tail logs manually:"
  echo "tail -F '${RUN_DIR}/implementation.log'"
  echo "tail -F '${RUN_DIR}/security-review.log'"
  echo "tail -F '${RUN_DIR}/code-review.log'"
fi

echo "Run directory: ${RUN_DIR}"
