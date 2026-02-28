# Benchmark Harness

Measures **stdiag** diagnostic output against manual tool equivalents (dig, openssl, curl) across controlled failure scenarios, comparing token efficiency and diagnostic accuracy.

## Prerequisites

- Docker (with `docker compose`)
- Python 3.8+
- `pip install -r requirements.txt`
- Go toolchain (to build stdiag)

## Usage

```bash
# From repo root
make bench

# Or directly
cd bench && ./run.sh

# Preview without executing
cd bench && ./run.sh --dry-run
```

## What it does

1. Builds the `stdiag` binary
2. Spins up Docker containers simulating DNS, TLS, and HTTP failure scenarios
3. Runs `stdiag --json` and equivalent manual commands (`dig`, `openssl s_client`, `curl`) for each scenario
4. Counts tokens (Claude + GPT-4o) for both outputs
5. Evaluates diagnostic accuracy against ground-truth labels
6. Prints a summary comparison table

## Output

Results are written to `bench/results/` (git-ignored):

```
results/<scenario_name>/
  stdiag.json          # stdiag JSON output
  manual.txt           # concatenated manual tool output
  stdiag_tokens.json   # token counts for stdiag output
  manual_tokens.json   # token counts for manual output
```

## Scenarios

Defined in `scenarios.json`. Each scenario specifies a target URL, expected ground-truth diagnosis, and required evidence fields. See the file for the full list.
