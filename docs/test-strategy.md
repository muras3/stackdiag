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

| Type | Purpose | Status |
|------|---------|--------|
| Unit (Small) | Pure logic verification. Fake injection | Implemented |
| Integration (Medium) | localhost I/O. httptest, net.Listen | Implemented |
| E2E (Large) | Pre-built binary stdout/stderr/exit code validation | Implemented |
| Contract | JSON schema backward compatibility verification | Implemented |
| Failure-matrix | Layer × failure pattern coverage | Implemented |
| Race | Concurrency bug detection via `go test -race` | Implemented |
| Fuzz | Panic/hang detection for URL/argument parsers | Planned |
| Golden file | Snapshot comparison of renderer output | Planned |
| Benchmark | Allocation/latency tracking on hot paths | Planned |
| Regression | Reproducer test for each bug fix | Ongoing |
| Smoke | Minimal operational check on release binary | Planned |
| Acceptance | Cross-validation against real tools (dig/openssl/curl) | Planned |

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
| SERVFAIL | fake resolver: return SERVFAIL-like error | `dns.status=fail`, `error.code=DNS_ERROR`, `observations.dns_error_hint="servfail"` |

#### TCP

| Pattern | Test Method | Expected Result |
|---------|------------|----------------|
| Connection refused | connect to closed localhost port | `tcp.status=fail`, `error.code=TCP_REFUSED` |
| Timeout | fake dialer: wait for deadline | `tcp.status=fail`, `error.code=TCP_TIMEOUT` |
| Reset | listener accept → immediate RST | `tcp.status=fail`, `error.code=TCP_ERROR` |

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
| 4xx (401, 403, 404, 429, others) | httptest handler | 401/403/404/429 use specific codes; other 4xx use `HTTP_4XX` |
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

## Fuzz Tests (Planned)

Will use Go native fuzzing.

Targets:
- URL/target parser (scheme, host, port, path, IPv6, IDNA)
- CLI argument parser
- Error code classification logic

Assertions:
- No panics
- No hangs (with timeout)
- Bounded memory usage
- Correct error code classification when errors are returned

## Real Environment Testing (Planned)

### Benchmark Infrastructure (Implemented)

Token efficiency benchmarks use Docker Compose with 18 services (CoreDNS, nginx variants, runner). See `bench/` for details. This infrastructure validates diagnostic correctness across 24 scenarios.

### Acceptance Testing (Planned)

Two-tier structure for future implementation:

#### Tier 1: Docker Compose Canary (deterministic, reproducible)

Controlled test servers for each failure mode. Planned for future implementation.

#### Tier 2: Cross-validation with Reference Tools

Compare stackdiag output against `dig`, `openssl s_client`, and `curl` for:
- Facts (IP, cert, status) → strict match
- Timing → tolerance (±200ms or ratio ≤2.5x)

Retry 3 times, pass on 2/3 consensus. Classify failures as `tool_bug` / `timing_drift` / `infra_flake`.

## Golden File Tests (Planned)

- Manage renderer output via `testdata/*.golden`
- Scenarios: `success`, `dns_fail`, `tls_warn`, `http_500`, `timeout`, `partial_failure`
- Regenerate with `-update` flag
- Normalize timing values during test (`XXms` substitution) for deterministic comparison

## CI Integration

### Current (v0.1.0)

Single workflow runs on both PR and push-to-main:

| Step | Command |
|------|---------|
| Lint | `make lint` (go vet + gofumpt) |
| Build | `make build` |
| Unit + Integration (race) | `make test-race` |
| E2E | `go test ./test/e2e/... -v -timeout 300s` |
| Binary size check | ≤ 7MB |

### Planned

| Trigger | Additional Scope |
|---------|-----------------|
| Nightly | Fuzz (long-running) + benchmark trend |
| tag `v*` | Acceptance + smoke + goreleaser |

## Quality Evidence

1. Test pyramid: Small (majority) + Medium (per-layer) + Large (92 E2E patterns)
2. Failure matrix coverage across all 5 layers
3. Schema contract tests (field presence, types, backward compatibility)
4. `go test -race` always passes in CI
5. Token efficiency benchmarks (24 scenarios, Docker-based)
