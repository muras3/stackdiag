# stackdiag — Design Decisions Log

Log of decisions agreed upon through discussions between Claude and Codex.

## Decision 1: Go (not Rust)

**Agreement:** Claude & Codex

- stdlib `net/tls/http` can be used directly for probe requirements
- Cross-compilation is easy and reliable
- Go has a large community in the CLI/ops tool space
- Rust's advantages (small binary size, type safety) don't justify development speed in this category

## Decision 2: layers as object (not array)

**Proposal:** Codex → array, Claude → object
**Conclusion:** object (Codex agreed)

- `result.layers.dns.status` is more natural than `result.layers.find(l => l.name == "dns").status`
- jq: `.layers.tls.timing` vs `.layers[] | select(.name=="tls") | .timing`
- Layer order is fixed by schema contract (no need to embed in data)
- Future dynamic/repeating phases will be addressed with a separate field

## Decision 3: error (not issue)

**Proposal:** Codex → issue, Claude → error
**Conclusion:** error (Codex agreed)

- Universal convention in API design
- severity is handled by the `status` field
- `status: "ok"` + `error: null` = healthy
- `status: "warn"` + `error: {...}` = warning
- `status: "fail"` + `error: {...}` = failure

## Decision 4: trace_id / attempt not included in MVP

**Proposal:** Codex → include, Claude → YAGNI
**Conclusion:** exclude (Codex agreed)

- No requirements for retry, batch, or distributed correlation
- Add when implementing `--count N`

## Decision 5: observations as layer-local + minimal top-level

**Proposal:** Codex → remove dotted keys, Claude → move inside layers + minimal top-level
**Conclusion:** Claude's proposal (Codex agreed)

- Layer-specific observations stored as typed fields within that layer
- Top-level `observations` only for cross-layer derived facts (can be empty object in MVP)

## Decision 6: MCP not needed (in MVP)

**Agreement:** Claude & Codex

- probe is completely stateless
- No requirement where MCP's persistent process model would be beneficial
- Extract internals as pure request→response API; design allows MCP adapter wrapper in future

## Decision 7: Screenshot-worthy human-friendly output

**Agreement:** Claude & Codex

- Colors: green✓ / yellow⚠ / red✗
- Monospace alignment
- `NO_COLOR` / non-TTY ASCII fallback
- Timing bar deferred to v0.2 (skipped in MVP)

## Decision 8: Agent-friendly = stable decision interface

**Agreement:** Claude & Codex

- `--json` is not a "bonus feature" but the product surface itself used by agents
- Explicitly state contract with schema_version
- Error codes must be structured (free text forbidden)
- Strict separation: stdout=data, stderr=logs

## Decision 9: Build order follows natural stack order

**Proposal:** Codex → TCP first, Claude → DNS first
**Conclusion:** DNS → TCP → TLS → HTTP (Codex agreed)

- Matches execution order; simpler mental model
- DNS is easiest to fake; contract flow validation happens early

## Decision 10: Makefile ensures local/CI parity

**Agreement:** Claude & Codex

- CI calls `make lint`, `make test-race`, `make build` directly
- Separate `fmt` and `fmt-check` (CI is strict)
- If it passes locally, it passes in CI

## Decision 11: Maximum 2 subagents in parallel

**Proposal:** Codex → start with 2
**Conclusion:** Max 2 parallel (Claude agreed)

- Review bandwidth is the bottleneck
- 4 parallel before pattern stability creates large merge friction
- Operations: TCP+TLS parallel after DNS completes

## Decision 12: Team composition

**Decision:** Human (final approver)

- Architect: Claude (design decisions require Codex consensus)
- Test Designer: Claude
- General Implementer: Claude
- Core Tech Implementer: Codex (TLS/HTTP measurement)
- Code Reviewer: Codex (all code)
- UI/UX Lead: Claude

## Decision 13: v0.1 scope expansion — integrated full-feature release

**Proposal:** Claude → auth only in v0.1, Codex → MinVersion+auth only in v0.1
**Conclusion:** All features integrated into v0.1 (Human decision)

- Include all of the following in v0.1:
  - MinVersion: explicitly set `tls.VersionTLS12` (both TLS layer and HTTP Transport)
  - `--bearer-env ENV_VAR` (fetch Bearer token from environment variable)
  - `--basic-env ENV_VAR` (fetch Basic auth from environment variable)
  - `--tls-scan` (TLS 1.0/1.1/1.2/1.3 version scan)
  - `--count N` + p50/p95/loss statistics (repeated measurement and statistical aggregation)
- Rationale:
  - Schema contract (additions only, type changes forbidden) means we should design optimal structure in v0.1 before users exist
  - Deferring to v0.2 forces design within backward-compatibility constraints
  - Auth hardening is core to identity as a security diagnostic tool
  - Default behavior unchanged; all features are opt-in, preserving simplicity
- Auth conflict rules:
  - Simultaneous `--header Authorization` with `--bearer-env`/`--basic-env` is error (INVALID_ARGS, exit 1)
  - Simultaneous `--bearer-env` and `--basic-env` is also error
  - Unset, empty, or whitespace-only environment variables are also error
  - Header name matching is case-insensitive
- `--count N` JSON design:
  - `attempts` array with raw data from each run (same structure as existing LayerResult)
  - `statistics` section with per-layer aggregation (p50_ms, p95_ms, success_count, fail_count, skip_count, sample_count, loss_ratio)
  - Without `--count`, maintain existing flat structure (backward compatible)
  - Discriminator is presence of `count` field
  - Exit code is worst case across N runs
- `--tls-scan` JSON design:
  - `tls_scan` object inside `observations`
  - `attempts` array (version, supported, duration_ms, error)
  - supported_versions, deprecated_versions_enabled (`[]string`)
  - On deprecated detection: `status=warn`, `error.code=TLS_DEPRECATED_VERSION_ENABLED`
  - Without `--tls-scan`, `tls_scan` field absent
- Deferred to Phase 2:
  - `--header-file` (read headers from file)
  - `--netrc` (auth from .netrc)
  - Rule Engine, MCP adapter

## Decision 14: Reachability layer addition (ICMP only)

**Agreement:** Claude & Codex

- Added Reachability as the 2nd layer: DNS → **Reachability** → TCP → TLS → HTTP
- ICMP only — no TCP fallback to avoid responsibility overlap with the TCP layer
- Exit code: 15 (between DNS 10 and TCP 20)
- When ICMP permission is denied: `probe_method: "none"`, `reachable: null`, `skip_reason: "permission_denied"`, `status: "skip"`
- Permission denied triggers `skip`, not `fail` — lack of ICMP privileges should not block downstream diagnosis
- Rejected alternatives:
  - TCP SYN fallback: duplicates TCP layer semantics
  - Always-skip on non-root: loses information on platforms where unprivileged ICMP works (macOS, some Linux configs)

## Decision 15: Error code audit principle

**Agreement:** Claude & Codex

- Contract error codes (`error.code`) must be distinguishable via Go types or errno values only
- Conditions that depend on Go internal string matching are demoted to best-effort hints in `observations`
- This principle was applied retroactively to v0.1 codes:
  - `DNS_SERVFAIL`, `DNS_REFUSED`, `DNS_NO_ANSWER` → removed from contract codes, moved to `dns_error_hint`
  - `TCP_RESET` → removed (not reliably produced at connect stage)
- `TLS_PROTOCOL_ERROR` explicitly documented as best-effort classification
- Rationale: Go stdlib may change internal error strings without notice; contract codes must survive Go version upgrades

## Decision 16: dns_error_hint pattern (contract vs observation separation)

**Agreement:** Claude & Codex

- DNS error codes reduced to 3 contract codes: `DNS_NXDOMAIN`, `DNS_TIMEOUT`, `DNS_ERROR`
- `dns_error_hint` field added to DNS observations: `"servfail"`, `"refused"`, `"no_answer"`, or `null`
- Consumers must not branch on `dns_error_hint` — it is informational only
- This pattern (contract code + observation hint) may be reused for other layers if similar situations arise
- Rationale: `net.DNSError.IsNotFound` and `.IsTimeout` are stable type-level checks; SERVFAIL/REFUSED/NO_ANSWER require string matching against `Err` field content that Go does not guarantee

## Decision 17: TLS observations expansion (cert_verified 2-phase trust model)

**Agreement:** Claude & Codex

- 7 new TLS observation fields: `cert_verified`, `cert_subject`, `cert_san`, `cert_issuer`, `cert_not_after`, `cert_not_before`, `cert_chain`
- `cert_verified` distinguishes "certificate was retrieved" from "certificate was validated by the system trust store"
  - `true`: normal TLS handshake succeeded with system trust
  - `false`: certificate was obtained via `InsecureSkipVerify` reconnection (2-phase handshake)
- 2-phase reconnection conditions: only on x509 errors, maximum 1 retry, within existing context deadline
- `cert_san`: empty array `[]` is valid (certificates without SANs exist); `null` is not used
- `cert_chain`: summary format only (`subject`, `issuer`, `not_after`). Full PEM is prohibited (token explosion + information leak risk)
- `TLS_NO_CERTIFICATES` added as contract code (existed in implementation but was missing from v0.1 schema)
- `TLS_HOSTNAME_MISMATCH` and `TLS_UNTRUSTED_CHAIN` implementation changed from string matching to `errors.As` type assertions
- Known constraint: 2-phase reconnection under load balancers may hit a different backend

## Decision 18: 5-layer architecture rationale

**Agreement:** Claude & Codex

- v0.2 moves from 4 layers (DNS → TCP → TLS → HTTP) to 5 layers (DNS → Reachability → TCP → TLS → HTTP)
- Motivated by real-world incident analysis where 4-layer model had diagnostic gaps:
  - Cloudflare BGP leak (2024): DNS resolved correctly but packets were routed to wrong AS — TCP timeout gave no reachability signal
  - Route leak incidents: host was unreachable at IP level but TCP timeout was ambiguous (firewall drop vs routing failure)
  - HTTP/2 Rapid Reset (CVE-2023-44487): connection-level issue was masked by HTTP layer errors — reachability data would have shown host was alive
- Adding the layer now (before v0.2 users exist) avoids future schema compatibility costs
- Schema contract means adding a layer post-publication requires a version bump; doing it in v0.2 is the last low-cost opportunity
- Exit code 15 fits naturally in the 10-20 gap between DNS and TCP
