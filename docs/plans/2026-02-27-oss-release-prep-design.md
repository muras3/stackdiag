# OSS Release Prep Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix Codex-identified issues and create a clean public `main` branch for OSS release.

**Architecture:** Two phases — fix content issues on current main, then create an orphan branch with clean history and curated files.

**Tech Stack:** Git, Go, Markdown

---

## Phase 1: Codex Issue Fixes

### Task 1: Sync schema.md with implementation

**Files:**
- Modify: `docs/schema.md`

**Step 1:** Add `status_text` to HTTP observations table.

**Step 2:** Add `tls_scan` observation structure (when `--tls-scan` flag is used).

**Step 3:** Add `TLS_DEPRECATED_VERSION_ENABLED` to TLS error codes table.

**Step 4:** Add `--count` output structure (CountResult with attempts and statistics).

**Step 5:** Add `--tls-scan` and `--count` flags to behavioral rules.

**Step 6:** Verify schema.md is complete by cross-referencing `internal/core/types.go`.

**Step 7:** Commit: `docs: sync schema.md with v0.1 implementation`

### Task 2: Structured JSON for argument/target errors

**Files:**
- Modify: `cmd/stackdiag/main.go`
- Modify: `internal/render/json/renderer.go` (if needed)
- Test: `test/e2e/e2e_test.go`

**Step 1:** Write E2E test: `--json` with invalid target should output JSON to stdout with error code `INVALID_TARGET`.

**Step 2:** Write E2E test: `--json` with invalid args should output JSON to stdout with error code `INVALID_ARGS`.

**Step 3:** Run tests to verify they fail.

**Step 4:** Modify main.go to output structured JSON error on argument/target parse failures when `--json` is set.

**Step 5:** Run tests to verify they pass.

**Step 6:** Run full test suite: `make test && make e2e`

**Step 7:** Commit: `fix: output structured JSON for argument/target errors`

### Task 3: Sync README with v0.1 features

**Files:**
- Modify: `README.md`

**Step 1:** Add `--tls-scan`, `--count`, `--bearer-env`, `--basic-env` to Options section.

**Step 2:** Add usage examples for new features in Quick Start.

**Step 3:** Update JSON example to include `status_text` in HTTP observations.

**Step 4:** Commit: `docs: sync README with v0.1 features`

### Task 4: Add OSS hygiene files

**Files:**
- Create: `SECURITY.md`
- Create: `CODE_OF_CONDUCT.md`

**Step 1:** Create `SECURITY.md` with responsible disclosure policy.

**Step 2:** Create `CODE_OF_CONDUCT.md` (Contributor Covenant v2.1).

**Step 3:** Commit: `docs: add SECURITY.md and CODE_OF_CONDUCT.md`

## Phase 2: Clean Public Branch

### Task 5: Create clean orphan branch

**Step 1:** Create `dev/archive` branch from current `main` (preserves full history).

**Step 2:** Create orphan branch `release/v0.1.0` (no history).

**Step 3:** Copy all files from main, excluding:
- `docs/plans/`
- `docs/reports/`
- `docs/agent-teams-guide.md`
- `docs/development-guide.md`
- `docs/design-decisions.md`
- `docs/test-strategy.md`
- `scripts/agent-*.sh`
- `CLAUDE.md`

**Step 4:** Commit: `Initial release v0.1.0`

**Step 5:** Verify: `make build && make test && make e2e && make lint`

### Task 6: Replace main with clean branch

**Step 1:** Rename current `main` → `dev/archive` (already done in T5).

**Step 2:** Rename `release/v0.1.0` → `main`.

**Step 3:** Verify final state: clean 1-commit main, archive branch preserved.

**Step 4:** Force push new main to origin (requires user confirmation).
