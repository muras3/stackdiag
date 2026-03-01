# v0.2 Full Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement all v0.2 schema changes (5-layer architecture, expanded observations, error code audit), update OSS documentation, and expand benchmarks — maximizing parallelism with Agent Teams.

**Architecture:** 5-layer sequential probe (DNS→Reachability→TCP→TLS→HTTP) with stop-at-first-failure. Each layer is an independent Go package implementing `core.Layer` interface. Renderers (JSON/table) consume `core.Result`. Benchmark harness validates accuracy/recall/token-efficiency.

**Tech Stack:** Go 1.24 (stdlib only), Docker Compose (benchmarks), Python (evaluation scripts)

---

## Team Structure

```
                        ┌─────────┐
                        │  Lead   │
                        └────┬────┘
          ┌──────┬──────┬────┼────┬──────┬──────┬──────┐
          ▼      ▼      ▼    ▼    ▼      ▼      ▼      ▼
       [core] [reach] [tls] [dns] [http] [render] [bench] [docs]
                                    │tcp│
          │      │      │    │    │      │      │      │
          └──────┴──────┴────┼────┴──────┴──────┴──────┘
                             ▼
                    ┌────────┴────────┐
                    │   reviewer      │  ← Codex second-opinion
                    │   security      │  ← セキュリティ専任
                    │   uiux          │  ← table出力品質
                    └─────────────────┘
```

### Agent Assignments

| Agent | subagent_type | isolation | Role |
|-------|---------------|-----------|------|
| `core` | general-purpose | worktree | Phase 0: types.go, exitcode.go, runner.go |
| `reachability` | general-purpose | worktree | Reachability layer package (new) |
| `tls-enhance` | general-purpose | worktree | TLS observations + error fixes |
| `dns-tcp` | general-purpose | worktree | DNS observations/cleanup + TCP cleanup |
| `http-enhance` | general-purpose | worktree | HTTP response_headers |
| `render` | general-purpose | worktree | JSON + table renderer updates |
| `bench` | general-purpose | worktree | Benchmark expansion + Docker infra |
| `docs` | general-purpose | worktree | schema.md, design-decisions.md, README |
| `reviewer` | general-purpose | — | Codex second-opinion (background) |
| `security` | general-purpose | — | Security audit (background) |
| `uiux` | general-purpose | — | Table output quality review |

---

## Dependency Graph

```
Phase 0 ──────────────────────────────────────────────────
  Task 0: core infrastructure [core]

Phase 1 (parallel, after Phase 0) ────────────────────────
  Task 1: Reachability layer        [reachability]
  Task 2: TLS observations+fixes    [tls-enhance]
  Task 3: DNS observations+cleanup  [dns-tcp]
  Task 4: HTTP response_headers     [http-enhance]

Phase 2 (parallel, after Phase 1) ────────────────────────
  Task 5: Renderers update          [render]
  Task 6: main.go wiring            [core or lead]
  Task 7: Contract tests update     [render]

Phase 3 (parallel with Phase 1-2) ────────────────────────
  Task 8: Documentation             [docs]

Phase 4 (after Phase 2) ──────────────────────────────────
  Task 9: Benchmark expansion       [bench]
  Task 10: tcp_refused optimization [http-enhance]

Continuous ────────────────────────────────────────────────
  Task 11: Code review              [reviewer]
  Task 12: Security review          [security]
  Task 13: UI/UX review             [uiux]
```

---

## Task 0: Core Infrastructure

**Agent:** `core`
**Blocks:** Tasks 1-7
**Files:**
- Modify: `internal/core/types.go`
- Modify: `internal/core/types_test.go`
- Modify: `internal/exitcode/exitcode.go`
- Modify: `internal/exitcode/exitcode_test.go`
- Modify: `internal/runner/runner.go`
- Modify: `internal/runner/runner_test.go`

### Step 1: Update LayerOrder and schema version

**Test first** (`internal/core/types_test.go`):
```go
func TestLayerOrderV02(t *testing.T) {
    expected := []string{"dns", "reachability", "tcp", "tls", "http"}
    if !reflect.DeepEqual(core.LayerOrder, expected) {
        t.Errorf("LayerOrder = %v, want %v", core.LayerOrder, expected)
    }
}

func TestSchemaVersionV02(t *testing.T) {
    r := &core.Result{SchemaVersion: core.SchemaVersion}
    if r.SchemaVersion != "v0.2" {
        t.Errorf("SchemaVersion = %q, want %q", r.SchemaVersion, "v0.2")
    }
}
```

**Implementation** (`internal/core/types.go`):
```go
const SchemaVersion = "v0.2"

var LayerOrder = []string{"dns", "reachability", "tcp", "tls", "http"}
```

Update `Result.MarshalJSON()` to include `"reachability"` in the ordered keys.
Update `AttemptResult.MarshalJSON()` similarly.
Update `CountResult.MarshalJSON()` statistics ordering.

### Step 2: Update exit codes

**Test first** (`internal/exitcode/exitcode_test.go`):
```go
func TestReachabilityExitCode(t *testing.T) {
    r := &core.Result{
        Layers: map[string]*core.LayerResult{
            "dns":          {Status: core.StatusOK},
            "reachability": {Status: core.StatusFail, Error: &core.ProbeError{Code: "REACHABILITY_TIMEOUT"}},
            "tcp":          {Status: core.StatusSkip},
            "tls":          {Status: core.StatusSkip},
            "http":         {Status: core.StatusSkip},
        },
    }
    got := exitcode.FromResult(r)
    if got != 15 {
        t.Errorf("FromResult() = %d, want 15", got)
    }
}
```

**Implementation** (`internal/exitcode/exitcode.go`):
```go
var layerOrder = []string{"dns", "reachability", "tcp", "tls", "http"}

var layerCodes = map[string]int{
    "dns":          10,
    "reachability": 15,
    "tcp":          20,
    "tls":          30,
    "http":         40,
}
```

### Step 3: Update runner for 5 layers

**Test** (`internal/runner/runner_test.go`):
Add test that runner.RunOnce() produces result with all 5 layer keys.

**Implementation** (`internal/runner/runner.go`):
Update `ensureAllLayers()` (or equivalent logic) to use `core.LayerOrder` (5 layers).

### Step 4: Run all tests

Run: `make test`
Expected: ALL PASS (existing tests may need LayerOrder updates)

### Step 5: Fix broken tests

Update all existing tests that hardcode `[]string{"dns", "tcp", "tls", "http"}` to include `"reachability"`.

Search pattern: `grep -r '"dns", "tcp", "tls", "http"' internal/`

### Step 6: Commit

```bash
git add internal/core/ internal/exitcode/ internal/runner/
git commit -m "feat: v0.2 core infrastructure - 5-layer architecture

- Add reachability to LayerOrder (dns→reachability→tcp→tls→http)
- Update SchemaVersion to v0.2
- Add exit code 15 for reachability failures
- Update runner and JSON marshalers for 5 layers"
```

---

## Task 1: Reachability Layer

**Agent:** `reachability`
**Blocked by:** Task 0
**Files:**
- Create: `internal/layers/reachability/reachability.go`
- Create: `internal/layers/reachability/reachability_test.go`
- Modify: `internal/testkit/fakes.go` (add FakePinger)

### Step 1: Define Pinger interface and test

**Test first** (`internal/layers/reachability/reachability_test.go`):
```go
func TestReachabilityOK(t *testing.T) {
    pinger := &FakePinger{RTT: 5 * time.Millisecond}
    layer := reachability.New(pinger)

    pctx := &core.ProbeContext{
        Context:     context.Background(),
        Target:      core.Target{Host: "example.com"},
        ResolvedIPs: []string{"203.0.113.10"},
    }
    result := layer.Probe(pctx)

    if result.Status != core.StatusOK {
        t.Errorf("status = %q, want ok", result.Status)
    }
    if result.Observations["reachable"] != true {
        t.Error("expected reachable=true")
    }
    if result.Observations["probe_method"] != "icmp" {
        t.Error("expected probe_method=icmp")
    }
}

func TestReachabilityTimeout(t *testing.T) {
    pinger := &FakePinger{Err: context.DeadlineExceeded}
    layer := reachability.New(pinger)
    // ... assert status=fail, error.code=REACHABILITY_TIMEOUT
}

func TestReachabilityPermissionDenied(t *testing.T) {
    pinger := &FakePinger{Err: &net.OpError{Err: os.ErrPermission}}
    layer := reachability.New(pinger)
    // ... assert status=skip, skip_reason=permission_denied, reachable=null
}
```

### Step 2: Implement layer

**Implementation** (`internal/layers/reachability/reachability.go`):
```go
package reachability

type Pinger interface {
    Ping(ctx context.Context, addr string) (rtt time.Duration, err error)
}

type Layer struct {
    pinger Pinger
}

func (l *Layer) Name() string { return "reachability" }

func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
    // Use first resolved IP from DNS layer
    // ICMP ping
    // Permission denied → status=skip, skip_reason=permission_denied
    // Timeout → status=fail, REACHABILITY_TIMEOUT
    // Other error → status=fail, REACHABILITY_ERROR
    // Success → status=ok, reachable=true, rtt_ms
}
```

**ICMP implementation**: Use raw socket ICMP (requires root/setcap) or `golang.org/x/net/icmp`.
Decision: Use stdlib `net.Dial("ip4:icmp", addr)` for zero-dependency approach.
If permission denied, gracefully skip.

### Step 3: Run tests

Run: `go test ./internal/layers/reachability/ -v`
Expected: ALL PASS

### Step 4: Commit

```bash
git commit -m "feat: add reachability layer (ICMP probe)

- New internal/layers/reachability package
- ICMP-only design (no tcp_fallback)
- Graceful skip on permission denied
- Error codes: REACHABILITY_TIMEOUT, REACHABILITY_ERROR"
```

---

## Task 2: TLS Observations Expansion + Error Fixes

**Agent:** `tls-enhance`
**Blocked by:** Task 0
**Files:**
- Modify: `internal/layers/tls/tls.go`
- Modify: `internal/layers/tls/tls_test.go`

### Step 1: Add TLS observation tests

**Test first** (`internal/layers/tls/tls_test.go`):
```go
func TestTLSObservationsV02(t *testing.T) {
    // Handshake with valid cert
    // Assert observations contain:
    //   cert_verified: true
    //   cert_subject: "example.com"
    //   cert_san: ["example.com", "*.example.com"]
    //   cert_issuer: "Example CA"
    //   cert_not_after: "2027-01-01T00:00:00Z"
    //   cert_not_before: "2026-01-01T00:00:00Z"
}

func TestTLSCertSanEmpty(t *testing.T) {
    // Cert with no SANs → cert_san: [] (empty array, not null)
}

func TestTLSCertVerifiedFalseOnInsecure(t *testing.T) {
    // InsecureSkipVerify reconnection → cert_verified: false
}

func TestTLSCertChainSummary(t *testing.T) {
    // Assert cert_chain returns [{subject, issuer, not_after}, ...]
}
```

### Step 2: Implement new observations

**Implementation** (`internal/layers/tls/tls.go` — `buildObservations()`):
```go
func buildObservations(state *tls.ConnectionState, serverName string, verified bool) map[string]any {
    obs := map[string]any{
        "version":      tlsVersionString(state.Version),
        "cipher_suite": tls.CipherSuiteName(state.CipherSuite),
    }
    if len(state.PeerCertificates) > 0 {
        leaf := state.PeerCertificates[0]
        obs["cert_verified"] = verified
        obs["cert_subject"] = leaf.Subject.CommonName
        obs["cert_san"] = leaf.DNSNames // empty slice if no SANs
        if obs["cert_san"] == nil {
            obs["cert_san"] = []string{} // ensure [] not null
        }
        obs["cert_issuer"] = leaf.Issuer.CommonName
        obs["cert_not_after"] = leaf.NotAfter.UTC().Format(time.RFC3339)
        obs["cert_not_before"] = leaf.NotBefore.UTC().Format(time.RFC3339)
        obs["cert_days_until_expiry"] = int(time.Until(leaf.NotAfter).Hours() / 24)
        obs["cert_hostname_match"] = certMatchesHost(leaf, serverName)

        // cert_chain summary
        chain := make([]map[string]string, 0, len(state.PeerCertificates))
        for _, c := range state.PeerCertificates {
            chain = append(chain, map[string]string{
                "subject":   c.Subject.CommonName,
                "issuer":    c.Issuer.CommonName,
                "not_after": c.NotAfter.UTC().Format(time.RFC3339),
            })
        }
        obs["cert_chain"] = chain
    }
    return obs
}
```

### Step 3: Fix error classification to type-based

**Test first:**
```go
func TestTLSHostnameMismatchTypeAssertion(t *testing.T) {
    // Create x509.HostnameError directly
    // Assert classifyTLSError returns TLS_HOSTNAME_MISMATCH
}

func TestTLSUntrustedChainTypeAssertion(t *testing.T) {
    // Create x509.UnknownAuthorityError directly
    // Assert classifyTLSError returns TLS_UNTRUSTED_CHAIN
}
```

**Implementation** (`classifyTLSError()`):
```go
func classifyTLSError(err error) *core.ProbeError {
    var hostErr x509.HostnameError
    if errors.As(err, &hostErr) {
        return &core.ProbeError{Code: "TLS_HOSTNAME_MISMATCH", Message: err.Error()}
    }
    var unknownAuth x509.UnknownAuthorityError
    if errors.As(err, &unknownAuth) {
        return &core.ProbeError{Code: "TLS_UNTRUSTED_CHAIN", Message: err.Error()}
    }
    var certInvalid x509.CertificateInvalidError
    if errors.As(err, &certInvalid) {
        // Expired, not yet valid, etc.
        return classifyCertInvalidError(certInvalid, err)
    }
    // ... existing timeout/protocol checks
}
```

### Step 4: Add TLS_NO_CERTIFICATES

**Test:**
```go
func TestTLSNoCertificates(t *testing.T) {
    // Handshake returns ConnectionState with empty PeerCertificates
    // Assert error.code = TLS_NO_CERTIFICATES
}
```

### Step 5: Run tests and commit

Run: `go test ./internal/layers/tls/ -v -race`
Expected: ALL PASS

```bash
git commit -m "feat(tls): expand observations and fix error classification

- Add cert_verified, cert_subject, cert_san, cert_issuer, cert_not_after,
  cert_not_before, cert_chain observations
- cert_san returns [] for certs without SANs (not null)
- Fix TLS_HOSTNAME_MISMATCH: errors.As(x509.HostnameError)
- Fix TLS_UNTRUSTED_CHAIN: errors.As(x509.UnknownAuthorityError)
- Add TLS_NO_CERTIFICATES error code"
```

---

## Task 3: DNS Observations + Error Cleanup + TCP Cleanup

**Agent:** `dns-tcp`
**Blocked by:** Task 0
**Files:**
- Modify: `internal/layers/dns/dns.go`
- Modify: `internal/layers/dns/dns_test.go`
- Modify: `internal/layers/tcp/tcp.go`
- Modify: `internal/layers/tcp/tcp_test.go`

### Step 1: DNS — Remove SERVFAIL/REFUSED/NO_ANSWER, add dns_error_hint

**Test first** (`internal/layers/dns/dns_test.go`):
```go
func TestDNSServfailBecomesErrorWithHint(t *testing.T) {
    // dnsErr with "server misbehaving"
    // Assert error.code = DNS_ERROR (not DNS_SERVFAIL)
    // Assert observations.dns_error_hint = "servfail"
}

func TestDNSRefusedBecomesErrorWithHint(t *testing.T) {
    // dnsErr with "refused"
    // Assert error.code = DNS_ERROR
    // Assert observations.dns_error_hint = "refused"
}

func TestDNSNoAnswerBecomesErrorWithHint(t *testing.T) {
    // dnsErr with no records
    // Assert error.code = DNS_ERROR
    // Assert observations.dns_error_hint = "no_answer"
}
```

**Implementation** (`internal/layers/dns/dns.go`):
```go
func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
    obs := map[string]any{
        "query_name":      host,
        "dns_error_hint":  nil,  // always present
        "ttl":             nil,  // v0.2: placeholder until resolver supports it
        "resolver_address": nil, // v0.2: placeholder
    }
    // ... resolution logic ...
    // On error:
    probeErr, hint := classifyDNSError(err)
    obs["dns_error_hint"] = hint  // "servfail", "refused", "no_answer", or nil
}

func classifyDNSError(err error) (*core.ProbeError, any) {
    var dnsErr *net.DNSError
    if !errors.As(err, &dnsErr) {
        return &core.ProbeError{Code: "DNS_ERROR", Message: err.Error()}, nil
    }
    if dnsErr.IsNotFound {
        return &core.ProbeError{Code: "DNS_NXDOMAIN", Message: err.Error()}, nil
    }
    if dnsErr.IsTimeout {
        return &core.ProbeError{Code: "DNS_TIMEOUT", Message: err.Error()}, nil
    }
    // Best-effort hints (not contract codes)
    hint := inferDNSHint(dnsErr)
    return &core.ProbeError{Code: "DNS_ERROR", Message: err.Error()}, hint
}

func inferDNSHint(dnsErr *net.DNSError) any {
    msg := dnsErr.Err
    switch {
    case strings.Contains(msg, "server misbehaving"):
        return "servfail"
    case strings.Contains(msg, "refused"):
        return "refused"
    case strings.Contains(msg, "no answer") || strings.Contains(msg, "no such host"):
        return "no_answer"
    default:
        return nil
    }
}
```

### Step 2: TCP — Remove TCP_RESET

**Test first** (`internal/layers/tcp/tcp_test.go`):
```go
func TestTCPResetBecomesError(t *testing.T) {
    // ECONNRESET → TCP_ERROR (not TCP_RESET)
}
```

**Implementation** (`internal/layers/tcp/tcp.go`):
Remove the `syscall.ECONNRESET` case from `classifyTCPError()`. Let it fall through to `TCP_ERROR`.

### Step 3: Run tests and commit

Run: `make test`

```bash
git commit -m "feat(dns,tcp): v0.2 error code audit

DNS:
- Remove DNS_SERVFAIL/REFUSED/NO_ANSWER from contract codes
- Add dns_error_hint observation (best-effort: servfail/refused/no_answer)
- Add ttl, resolver_address placeholders

TCP:
- Remove TCP_RESET (connect-phase unreachable)"
```

---

## Task 4: HTTP Response Headers

**Agent:** `http-enhance`
**Blocked by:** Task 0
**Files:**
- Modify: `internal/layers/http/http.go`
- Modify: `internal/layers/http/http_test.go`

### Step 1: Add response_headers test

**Test first** (`internal/layers/http/http_test.go`):
```go
func TestHTTPResponseHeaders(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("X-Request-Id", "abc-123")
        w.Header().Set("Server", "nginx")
        w.Header().Set("Set-Cookie", "session=secret")  // should be redacted
        w.WriteHeader(200)
    })
    // ... setup test server ...
    // Assert observations["response_headers"] contains selected headers
    // Assert Set-Cookie is redacted
}

func TestHTTPResponseHeadersEmpty(t *testing.T) {
    // Server returns no interesting headers
    // Assert observations["response_headers"] == {} (empty object, not null)
}
```

### Step 2: Implement response_headers

**Implementation** (`internal/layers/http/http.go`):
```go
// Important response headers to include (allowlist)
var importantResponseHeaders = map[string]bool{
    "content-type":            true,
    "server":                  true,
    "x-request-id":            true,
    "x-correlation-id":        true,
    "retry-after":             true,
    "www-authenticate":        true,
    "location":                true,
    "x-ratelimit-limit":       true,
    "x-ratelimit-remaining":   true,
    "x-ratelimit-reset":       true,
    "strict-transport-security": true,
    "x-content-type-options":  true,
    "x-frame-options":         true,
    "cache-control":           true,
    "age":                     true,
    "cf-ray":                  true,
    "x-served-by":             true,
}

func buildResponseHeaders(header http.Header, redact bool) map[string]string {
    result := make(map[string]string)
    for name, values := range header {
        lower := strings.ToLower(name)
        if !importantResponseHeaders[lower] {
            continue
        }
        val := strings.Join(values, ", ")
        if redact && isSensitiveHeader(name) {
            val = "[REDACTED]"
        }
        result[name] = val
    }
    return result
}
```

### Step 3: Run tests and commit

```bash
git commit -m "feat(http): add response_headers observation

- Selected important headers (allowlist approach)
- Sensitive headers redacted by default
- Empty object when no important headers present"
```

---

## Task 5: Renderer Updates

**Agent:** `render`
**Blocked by:** Tasks 1-4
**Files:**
- Modify: `internal/render/json/renderer.go`
- Modify: `internal/render/json/renderer_test.go`
- Modify: `internal/render/json/contract_test.go`
- Modify: `internal/render/table/renderer.go`
- Modify: `internal/render/table/renderer_test.go`

### Step 1: Update JSON contract tests

**Test first** (`internal/render/json/contract_test.go`):
```go
func TestV02LayerKeys(t *testing.T) {
    // Result with all 5 layers
    // Assert JSON output contains dns, reachability, tcp, tls, http in order
}

func TestV02SchemaVersion(t *testing.T) {
    // Assert schema_version = "v0.2"
}

func TestV02TLSObservations(t *testing.T) {
    // Assert cert_verified, cert_subject, cert_san, cert_issuer, etc. present
}

func TestV02DNSHint(t *testing.T) {
    // Assert dns_error_hint present in DNS observations
}

func TestV02ResponseHeaders(t *testing.T) {
    // Assert response_headers in HTTP observations
}

func TestV02ReachabilitySkip(t *testing.T) {
    // Assert reachability with skip_reason="permission_denied"
}
```

### Step 2: Update table renderer for Reachability

**Implementation** (`internal/render/table/renderer.go`):
Add rendering logic for the reachability layer:
- Show probe_method, reachable, rtt_ms
- Handle skip_reason display
- Format new TLS observations (cert_subject, cert_issuer, cert_san)
- Format response_headers

### Step 3: Run tests and commit

```bash
git commit -m "feat(render): update renderers for v0.2 schema

- JSON: 5-layer ordering, new observations
- Table: reachability row, expanded TLS/HTTP display
- Contract tests updated for v0.2"
```

---

## Task 6: main.go Wiring

**Agent:** `core` (or lead)
**Blocked by:** Tasks 1, 5
**Files:**
- Modify: `cmd/stackdiag/main.go`
- Modify: `test/e2e/stackdiag_test.go`

### Step 1: Wire Reachability into buildLayers()

```go
func buildLayers(tgt core.Target, insecure bool) []core.Layer {
    layers := []core.Layer{
        dns.NewDefault(),
        reachability.NewDefault(),  // Always included
        tcp.NewDefault(),
    }
    if tgt.NeedsTLS() {
        layers = append(layers, tls.NewDefault())
    }
    if tgt.NeedsHTTP() {
        layers = append(layers, http.NewDefault(insecure, tgt.Host))
    }
    return layers
}
```

### Step 2: Update E2E tests

Update `test/e2e/stackdiag_test.go` to expect 5 layers in output.

### Step 3: Build and run E2E

```bash
make build && make e2e
```

### Step 4: Commit

```bash
git commit -m "feat: wire reachability layer into CLI

- buildLayers() includes reachability between dns and tcp
- E2E tests updated for 5-layer output"
```

---

## Task 7: Contract Test Updates

**Agent:** `render`
**Blocked by:** Task 5
**Files:**
- Modify: `internal/render/json/contract_test.go`

Ensure contract tests verify:
1. All 5 layers present in JSON output
2. Layer order: dns → reachability → tcp → tls → http
3. New observations fields present
4. Removed error codes NOT in output
5. schema_version = "v0.2"
6. Skip invariants for reachability (permission denied case)

---

## Task 8: Documentation

**Agent:** `docs`
**Blocked by:** None (uses schema-v02.md as source of truth)
**Files:**
- Modify: `docs/schema.md` (v0.1 → v0.2)
- Modify: `docs/design-decisions.md` (add new decisions)
- Modify: `README.md` (update layer count, exit codes)

### Step 1: Update docs/schema.md

Major changes:
- `schema_version` → `"v0.2"`
- `layers` → 5 keys (add reachability)
- DNS observations: add `dns_error_hint`, `ttl`, `resolver_address`
- Reachability section (entirely new)
- TLS observations: add `cert_verified`, `cert_subject`, `cert_san`, `cert_issuer`, `cert_not_after`, `cert_not_before`, `cert_chain`
- HTTP observations: add `response_headers`
- Error codes: remove DNS_SERVFAIL/REFUSED/NO_ANSWER, TCP_RESET; add TLS_NO_CERTIFICATES, REACHABILITY_TIMEOUT/ERROR
- Exit codes: add 15
- Target schemes: update layer execution table
- Examples: update to 5-layer output
- Note TLS_PROTOCOL_ERROR as best-effort classification

### Step 2: Update docs/design-decisions.md

Add decisions:
- Decision 14: Reachability layer addition (ICMP only, no tcp_fallback)
- Decision 15: Error code audit (type-based vs string-based principle)
- Decision 16: dns_error_hint pattern (contract vs best-effort separation)
- Decision 17: TLS observations expansion (cert_verified 2-phase trust model)
- Decision 18: 5-layer architecture rationale (real incident analysis)

### Step 3: Update README.md

- "DNS → TCP → TLS → HTTP" → "DNS → Reachability → TCP → TLS → HTTP"
- Exit code table: add 15
- Update example output

### Step 4: Commit

```bash
git commit -m "docs: update documentation for v0.2 schema

- schema.md: 5-layer architecture, new observations, error code audit
- design-decisions.md: 5 new decisions (14-18)
- README.md: updated layer list and exit codes"
```

---

## Task 9: Benchmark Expansion

**Agent:** `bench`
**Blocked by:** Tasks 6 (needs working binary)
**Files:**
- Modify: `bench/scenarios.json`
- Modify: `bench/docker-compose.yml`
- Modify: `bench/capture.sh`
- Modify: `bench/evaluate.py`
- Create: `bench/docker/nginx/` (new scenario configs)
- Create: `bench/docker/certs/` (new cert types)

### Step 1: Add new benchmark scenarios

New scenarios to add:

| Scenario | Target | Ground Truth | Tests |
|----------|--------|-------------|-------|
| `dns_timeout` | DNS timeout target | `DNS_TIMEOUT` | DNS layer timeout |
| `reachability_timeout` | ICMP-unreachable host | `REACHABILITY_TIMEOUT` | Reachability layer |
| `tcp_timeout` | Blackhole port | `TCP_TIMEOUT` | TCP timeout (vs refused) |
| `tls_no_certs` | Server with no certs | `TLS_NO_CERTIFICATES` | New error code |
| `cert_chain_invalid` | Intermediate missing | `TLS_UNTRUSTED_CHAIN` | Chain validation |
| `cert_expiring_soon` | Cert expiring in 7 days | `TLS_CERT_EXPIRING_SOON` | Warning path |
| `http_429` | Rate limited endpoint | `HTTP_429` | Rate limiting |
| `http_redirect` | 301 redirect | all_ok (status=ok) | Redirect not followed |

### Step 2: Update required_evidence for v0.2 observations

All TLS scenarios should require:
- `cert_verified`, `cert_subject`, `cert_san`, `cert_issuer`, `cert_not_after`

HTTP scenarios should require:
- `response_headers`

### Step 3: Update Docker infrastructure

- Add nginx configs for new scenarios
- Generate certs: expiring-soon (7 days), chain-invalid
- Add ICMP-unreachable target (iptables DROP rule in docker network)

### Step 4: Update evaluate.py

- Add evidence recall checks for new observation fields
- Handle reachability layer in evaluation

### Step 5: Run benchmarks

```bash
make bench
```

Verify:
- accuracy = 100%
- evidence_recall = 100%
- token_efficiency_ratio ≤ 0.25

### Step 6: Commit

```bash
git commit -m "bench: expand to 18 scenarios for v0.2

- Add dns_timeout, reachability_timeout, tcp_timeout, tls_no_certs,
  cert_chain_invalid, cert_expiring_soon, http_429, http_redirect
- Update required_evidence for v0.2 observations
- Update evaluation for reachability layer"
```

---

## Task 10: tcp_refused Token Optimization

**Agent:** `http-enhance`
**Blocked by:** Task 5
**Files:**
- Modify: `internal/render/table/renderer.go` (if table output too verbose)
- Modify: `internal/layers/tcp/tcp.go` (if error message too long)

### Goal: token ratio ≤ 1.0

Current issue: tcp_refused has 1.37 token ratio (stdiag output larger than manual `nc` command).

### Step 1: Analyze current output

Compare `bench/results/*/tcp_refused/stdiag.json` vs `manual.txt` to identify excess tokens.

### Step 2: Minimize output

Likely fixes:
- Shorter error messages
- Remove redundant fields in skip layers
- Ensure skip layers are minimal (`{status: "skip", duration_ms: 0, observations: {}, error: null}`)

### Step 3: Verify with benchmark

Run tcp_refused scenario only. Verify ratio ≤ 1.0.

---

## Task 11: Code Review (Continuous)

**Agent:** `reviewer`
**Runs:** Background, after each task completion

### Process

1. After each implementation task commits, reviewer runs:
   ```bash
   /second-opinion  # Codex review on uncommitted changes or branch diff
   ```

2. Review checklist:
   - [ ] TDD followed (tests written before implementation)
   - [ ] No string-based error classification (type/errno only)
   - [ ] Schema contract maintained (no field removals within v0.2)
   - [ ] observations vs error separation respected
   - [ ] Error messages don't leak sensitive data
   - [ ] Context cancellation properly handled
   - [ ] No external dependencies added

3. Report findings → Lead assigns fixes

---

## Task 12: Security Review

**Agent:** `security`
**Runs:** After Tasks 1-4 complete

### Checklist

1. **cert_verified flag**: Cannot be spoofed. Only true when full verification passes.
2. **Reachability ICMP**: Raw socket usage. Privilege escalation risks.
3. **response_headers**: No sensitive headers leaked (allowlist approach verified)
4. **cert_chain summary**: No PEM data, no private key exposure
5. **dns_error_hint**: Best-effort string, not used for security decisions
6. **TLS reconnection**: Only on x509 errors, 1 attempt, within existing deadline
7. **No credential forwarding**: Reachability layer doesn't send any auth data

Run: semgrep scan on all modified files

---

## Task 13: UI/UX Review

**Agent:** `uiux`
**Runs:** After Task 5 (renderers complete)

### Checklist

1. **Table output**: All 5 layers render correctly
2. **Reachability display**: Clear skip/ok/fail states
3. **TLS expanded info**: cert_subject, cert_issuer displayed readably
4. **cert_san**: Long SAN lists don't break table layout
5. **response_headers**: Formatted as key: value list
6. **Color coding**: Consistent with existing layers
7. **ASCII fallback**: Works without ANSI colors
8. Screenshot test: `stdiag https://example.com` looks screenshot-worthy

---

## Execution Timeline

```
T+0    ─── Task 0 (core) starts
T+10m  ─── Task 0 done. Tasks 1,2,3,4,8 start in parallel (5 agents)
T+30m  ─── Tasks 1-4 done. Tasks 5,6 start. Security review starts.
T+45m  ─── Tasks 5-7 done. Task 9 (bench) starts. UI/UX review starts.
T+60m  ─── Task 9 (bench) done. Task 10 (optimization) starts.
T+75m  ─── All implementation done. Final review round.
T+90m  ─── All reviews complete. Ready for human approval.
```

Code review runs continuously after each commit (background).
Docs (Task 8) runs fully parallel from T+0 (no code dependency).

---

## Merge Strategy

1. Each agent works in isolated worktree on its own branch
2. After review approval, merge to `dev/v0.0.1` in dependency order:
   - Task 0 (core) → Tasks 1-4 → Tasks 5-7 → Task 6 → Tasks 9-10
   - Task 8 (docs) merges independently
3. Final integration test on `dev/v0.0.1`
4. Benchmark run on merged branch
5. Human approval → merge to main → tag v0.2.0

---

## Success Criteria

- [ ] All existing tests pass (`make test`)
- [ ] All new tests pass
- [ ] E2E tests pass (`make e2e`)
- [ ] Benchmark accuracy = 100%
- [ ] Benchmark evidence_recall = 100%
- [ ] Token efficiency ratio ≤ 0.25
- [ ] tcp_refused ratio ≤ 1.0
- [ ] Codex review: no critical findings
- [ ] Security review: no vulnerabilities
- [ ] UI/UX review: screenshot-worthy output
- [ ] docs/schema.md reflects v0.2 completely
- [ ] `make lint` passes
- [ ] Binary size < 7MB
