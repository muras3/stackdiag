# stackdiag — Test Strategy

## Design Philosophy

stackdiag is a failure-diagnosis tool. **Failure tests are more important than success tests.**

Three pillars of quality assurance:
1. **Test Pyramid** — Small tests are the majority, Large tests are minimal
2. **Failure Matrix** — Comprehensive coverage of layer × failure pattern combinations
3. **Cross-validation with reference tools** — Comparison against dig/openssl/curl

## Test Pyramid (Google-style)

```
        ╱╲
       ╱ L ╲      Large: pre-built binary + Docker canary
      ╱──────╲     5-10 tests
     ╱   M    ╲   Medium: httptest, localhost listener, TLS fixture
    ╱──────────╲   5-10 tests per layer
   ╱     S      ╲ Small: parser, classifier, renderer, exit code
  ╱──────────────╲ Majority of tests
```

## Test Types

| Type | Purpose | Introduction |
|------|---------|-------------|
| Unit (Small) | Pure logic verification. Fake injection | Phase 0+ |
| Integration (Medium) | localhost I/O. httptest, net.Listen | Phase 1+ |
| E2E (Large) | Pre-built binary stdout/stderr/exit code | Phase 3 |
| Contract | JSON schema backward compatibility verification | Phase 0+ |
| Failure-matrix | Layer × failure pattern coverage | Phase 1+ |
| Fuzz | Panic/hang detection for URL/argument parsers | Phase 0+ |
| Golden file | Snapshot comparison of renderer output | Phase 0+ |
| Race | Concurrency bug detection via `go test -race` | Phase 2+ |
| Benchmark | Allocation/latency tracking on hot paths | Phase 2+ |
| Regression | Reproducer test for each bug fix | Ongoing |
| Smoke | Minimal operational check on release binary | Phase 3 |
| Acceptance | Cross-validation against real tools (dig/openssl/curl) | Phase 3 |

## Faking Strategy

Each layer injects dependencies via interfaces, swapping in fakes during tests.

| Layer | Interface | Fake Approach |
|-------|-----------|--------------|
| DNS | `Resolver` | fake: return fixed IPs / return error / wait for context deadline |
| TCP | `Dialer` | fake: immediate success / localhost `net.Listen` / immediate error |
| TLS | `TLSHandshaker` | fake: fixed result / local TLS server with fixture certs |
| HTTP | `HTTPDoer` | fake: fixed response / `httptest.Server` |
| Time | `Clock` | fake: fixed time (prevents flaky timing tests) |

## Failure Matrix Testing

Inspired by Netflix chaos engineering. Implemented in stackdiag as "deterministic fault injection."

### Failure Patterns by Layer

#### DNS

| Pattern | Test Method | Expected Result |
|---------|------------|----------------|
| NXDOMAIN | fake resolver: return error code | `dns.status=fail`, `error.code=DNS_NXDOMAIN` |
| Timeout | fake resolver: wait for context deadline | `dns.status=fail`, `error.code=DNS_TIMEOUT` |
| SERVFAIL | fake resolver: return SERVFAIL error | `dns.status=fail`, `error.code=DNS_SERVFAIL` |

#### TCP

| Pattern | Test Method | Expected Result |
|---------|------------|----------------|
| Connection refused | connect to closed localhost port | `tcp.status=fail`, `error.code=TCP_REFUSED` |
| Timeout | fake dialer: wait for deadline | `tcp.status=fail`, `error.code=TCP_TIMEOUT` |
| Reset | listener accept → immediate RST | `tcp.status=fail`, `error.code=TCP_RESET` |

#### TLS

| Pattern | Test Method | Expected Result |
|---------|------------|----------------|
| Expired cert | fixture certificate (expired) | `tls.status=fail`, `error.code=TLS_CERT_EXPIRED` |
| Hostname mismatch | fixture with SAN mismatch | `tls.status=fail`, `error.code=TLS_HOSTNAME_MISMATCH` |
| Untrusted CA | self-signed with unknown CA | `tls.status=fail`, `error.code=TLS_UNTRUSTED_CHAIN` |
| Cert expiring soon | fixture with <30 days remaining | `tls.status=warn`, `error.code=TLS_CERT_EXPIRING_SOON` |

#### HTTP

| Pattern | Test Method | Expected Result |
|---------|------------|----------------|
| 4xx (401, 403, 404) | httptest handler | `http.status=fail`, `error.code=HTTP_4XX` |
| 5xx (500, 502, 503) | httptest handler | `http.status=fail`, `error.code=HTTP_5XX` |
| Timeout | handler sleep exceeds deadline | `http.status=fail`, `error.code=HTTP_TIMEOUT` |
| Reset mid-response | hijack → partial write → close | `http.status=fail` |

### Partial Failure Scenarios (stackdiag core tests)

| Scenario | dns | tcp | tls | http | exit |
|----------|-----|-----|-----|------|------|
| DNS failure | fail | skip | skip | skip | 10 |
| DNS ok → TCP failure | ok | fail | skip | skip | 20 |
| TCP ok → TLS failure | ok | ok | fail | skip | 30 |
| TLS ok → HTTP failure | ok | ok | ok | fail | 40 |
| TLS warn + HTTP failure | ok | ok | warn | fail | 40 |
| All success | ok | ok | ok | ok | 0 |

### Timeout Propagation Tests

- Total budget 2s → DNS consumes 1.5s → TCP onward gets remaining 0.5s
- Budget exhausted → subsequent layers are `skip`
- Reported timeout stage = root cause (not the last observed symptom)

## Contract Tests (Stripe-style)

Treat JSON schema as a public API and automatically verify backward compatibility.

Verification items:
- All field presence and types
- Possible values of `status`: `ok`, `warn`, `fail`, `skip`
- `error` is null or `{code, message}`
- `schema_version` exists
- No field deletions (compare against previous version golden file)

Golden files alone are insufficient. **Write separate semantic contract assertions.**

## Fuzz Tests (Cloudflare-style)

Uses Go native fuzzing.

Targets:
- URL/target parser (scheme, host, port, path, IPv6, IDNA)
- CLI argument parser
- Error code classification logic

Assertions:
- No panics
- No hangs (with timeout)
- Bounded memory usage
- Correct error code classification when errors are returned

Seed corpus is checked in at `testdata/fuzz/`.

## Real Environment Testing (Acceptance Testing)

### Two-tier Structure

#### Tier 1: Docker Compose Canary (deterministic, reproducible)

```
docker-compose.yml
├── dns-server (CoreDNS)        # Controlled DNS
├── ok-server                    # 200 OK, fast response
├── slow-server                  # Header delay (TTFB verification)
├── selfsigned-server            # Self-signed certificate
├── expired-server               # Expired certificate
├── wrong-san-server             # SAN mismatch
└── error-server                 # 503 etc.
```

Runs identically locally and in CI via `make acceptance`.

#### Tier 2: Cross-validation with Reference Tools

| Item | Comparison Target | Comparison Method |
|------|------------------|-------------------|
| DNS resolution | `dig` | Exact match (IP addresses) |
| TLS certificate | `openssl s_client` | Exact match (fingerprint, SAN, issuer, expiry) |
| HTTP status | `curl -w` | Exact match |
| DNS timing | `dig` | Tolerance (±200ms or ratio ≤2.5x) |
| TCP timing | `curl` connect time | Tolerance |
| TTFB | `curl` time_starttransfer | Tolerance (align measurement definitions first) |

**Important:** Timing definitions differ between tools. Align definitions before comparison.

### Acceptance Test Manifest

```yaml
- name: canary-ok
  url: https://ok.stackdiag-test.local
  dns:
    compare_with: dig
    exact_answer: true
  tls:
    compare_with: openssl
    compare_fields: [leaf_sha256, subject, not_after]
  http:
    compare_with: curl
    status_exact: true
    ttfb_tolerance_ms: 200
  retries: 3
  require_consensus: 2

- name: canary-expired-cert
  url: https://expired.stackdiag-test.local
  tls:
    expected_status: fail
    expected_error_code: TLS_CERT_EXPIRED
  retries: 1
```

### Acceptance Test Flow

1. `stackdiag --json <url>` → capture JSON
2. Run `dig` / `openssl` / `curl` against the same target
3. Normalize and compare
   - Facts (IP, cert, status) → strict match
   - Timing → tolerance (`abs(delta) <= N ms` OR `ratio <= 2.5`)
4. Retry 3 times, pass on 2/3 consensus
5. On failure, classify: `tool_bug` / `timing_drift` / `infra_flake`

### Flakiness Mitigation

- Separate correctness (IP, cert, status) from performance (timing)
- Retry + consensus (2/3 pass)
- Use own canary infrastructure for blocking decisions
- Public endpoints for smoke only (never used for CI pass/fail)
- Record environment context (resolved IP, protocol, ALPN, timestamp)

## Golden File Tests

- Manage renderer output via `testdata/*.golden`
- Scenarios: `success`, `dns_fail`, `tls_warn`, `http_500`, `timeout`, `partial_failure`
- Regenerate with `-update` flag
- Normalize timing values during test (`XXms` substitution) for deterministic comparison

## Phased Test Plan

### Phase 0: Core types + CLI skeleton + JSON renderer

- Small tests: parser, validation, defaults, error types, JSON shape
- Contract tests: JSON schema field presence and type verification (start here)
- Golden tests: JSON output
- Fuzz tests: URL/CLI argument parser (start here)

### Phase 1: DNS → TCP → TLS → HTTP layers

- Medium tests: localhost fixtures (httptest, net.Listen, local TLS)
- **Failure matrix**: write failure tests first and more than success tests
- Timeout/deadline propagation tests
- Partial failure scenarios (DNS ok → TCP fail, etc.)
- Benchmark: per-layer overhead and allocation measurement

### Phase 2: Runner + Table renderer + Exit codes

- Race tests: `go test -race` for runner/timeout concurrency
- Exit code contract tests
- Golden tests: table output (timing value normalization)
- Local E2E: pre-built binary + fixtures

### Phase 3: E2E + CI/CD + Acceptance

- CI full pyramid: PR=Small+Medium, Nightly=full stack+fuzz+acceptance
- Docker Compose canary environment setup
- Acceptance harness: YAML-driven + cross-validation
- Smoke tests: minimal operational check on release binary
- Cross-platform: linux/darwin × amd64/arm64

## CI Integration

| Trigger | Test Scope |
|---------|-----------|
| PR | `make test-race` (Small + Medium) + `make build` |
| push main | Above + `make e2e` + Docker canary acceptance |
| Nightly | Full stack + fuzz (long-running) + public endpoint smoke + benchmark trend |
| tag `v*` | Full stack + acceptance + smoke + goreleaser |

## Quality Evidence for Publication

1. Test pyramid ratio (Small:Medium:Large)
2. Failure matrix coverage table
3. Schema contract test existence
4. Fuzz seed corpus and CI execution logs
5. `go test -race` always passes in CI
6. Every bug fix has a regression test
7. Cross-validation results against reference tools
