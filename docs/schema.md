# stackdiag JSON Schema Reference

> Schema version: **v0.1**

This document is the complete specification for stackdiag's `--json` output.

## Top-Level Structure

```json
{
  "schema_version": "v0.1",
  "started_at": "2026-02-26T18:42:03Z",
  "target": "https://api.example.com/health",
  "layers": { ... },
  "summary": { ... }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Schema version identifier (currently `"v0.1"`) |
| `started_at` | string | ISO 8601 UTC timestamp of stackdiag start |
| `target` | string | Original target as provided by the user |
| `layers` | object | Per-layer results (see below) |
| `summary` | object | Aggregate stackdiag information |

## Layers

The `layers` object always contains exactly five keys: `dns`, `reachability`, `tcp`, `tls`, `http`. Layers that were not executed have `status: "skip"`.

### Layer Result Structure

Every layer result has the same shape:

```json
{
  "status": "ok" | "warn" | "fail" | "skip",
  "duration_ms": 9.123,
  "observations": { ... },
  "error": null | { "code": "...", "message": "..." }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string enum | `"ok"`, `"warn"`, `"fail"`, or `"skip"` |
| `duration_ms` | float64 | Layer execution time in milliseconds |
| `observations` | object | Layer-specific diagnostic data (see below) |
| `error` | object \| null | Structured error when status is `warn` or `fail` |

### Status Values

| Value | Meaning | Runner behavior |
|-------|---------|-----------------|
| `ok` | Layer passed | Continue to next layer |
| `warn` | Warning (layer functional) | Continue to next layer |
| `fail` | Layer failed | Stop; remaining layers get `skip` |
| `skip` | Not executed | Layer was not applicable or a prior layer failed |

### Error Object

```json
{
  "code": "TLS_CERT_EXPIRING_SOON",
  "message": "Certificate expires in 5 days"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `code` | string | Machine-readable error code (see [Error Codes](#error-codes)) |
| `message` | string | Human-readable description |

The `code` field is the primary interface for programmatic consumers. The `message` field is informational and may change between versions.

## Observations by Layer

### DNS

| Field | Type | Description |
|-------|------|-------------|
| `query_name` | string | Queried hostname |
| `answers` | []string | Resolved IP addresses |
| `dns_error_hint` | string\|null | Best-effort hint: `"servfail"`, `"refused"`, `"no_answer"`, or `null`. Depends on Go internal strings; not a contract value — use for diagnostics only |
| `ttl` | int\|null | Remaining TTL in seconds (not the authoritative TTL). `null` if unavailable |
| `resolver_address` | string\|null | Resolver address used for the query. `null` if system default |

### Reachability

| Field | Type | Description |
|-------|------|-------------|
| `probe_method` | string | `"icmp"` or `"none"` |
| `reachable` | bool\|null | Whether the host is reachable. `null` if determination was not possible (e.g., permission denied) |
| `rtt_ms` | float64\|null | Round-trip time in milliseconds. `null` if unreachable or undetermined |
| `skip_reason` | string\|null | Reason the layer was skipped: `"permission_denied"`, `"unsupported_address"` (reserved for future use), or `null` |

Design notes:
- ICMP only. No TCP fallback (avoids responsibility overlap with the TCP layer).
- When ICMP is not permitted: `probe_method: "none"`, `reachable: null`, `skip_reason: "permission_denied"`, `status: "skip"`.
- IPv4 and IPv6 addresses are both supported via ICMP/ICMPv6.
- `skip_reason: "unsupported_address"` is reserved for future address types that cannot be probed.
- Permission denied results in `skip`, not `fail`, to preserve diagnostic reliability.

### TCP

| Field | Type | Description |
|-------|------|-------------|
| `remote_ip` | string | Connected IP address |
| `remote_port` | int | Connected port number |

### TLS

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | TLS version (e.g., `"TLSv1.3"`) |
| `cipher_suite` | string | Negotiated cipher suite |
| `cert_days_until_expiry` | int | Days until certificate expiration |
| `cert_hostname_match` | bool | Whether the certificate matches the hostname |
| `cert_verified` | bool | `true` if the certificate was fully verified by the system trust store; `false` if it was retrieved via `InsecureSkipVerify` reconnection. Distinguishes "retrieved" from "validated" |
| `cert_subject` | string | Certificate subject Common Name |
| `cert_san` | []string | Subject Alternative Names. Can be an empty array `[]` (for certificates without SANs) |
| `cert_issuer` | string | Certificate issuer Common Name |
| `cert_not_after` | string | Certificate expiration date in ISO 8601 UTC |
| `cert_not_before` | string\|null | Certificate validity start date in ISO 8601 UTC. `null` if unavailable |
| `cert_chain` | []object\|null | Certificate chain in summary format: `[{"subject":"...","issuer":"...","not_after":"..."}]`. Full PEM is never included (token cost and leak risk). `null` if unavailable |

### HTTP

| Field | Type | Description |
|-------|------|-------------|
| `method` | string | HTTP method used |
| `protocol` | string | HTTP protocol (e.g., `"HTTP/2"`) |
| `status_code` | int | HTTP response status code |
| `status_text` | string | HTTP status text (e.g., `"OK"`, `"Service Unavailable"`) |
| `request_headers` | object | Request headers sent (sensitive values redacted by default) |
| `response_headers` | object | Selected response headers. Can be an empty object `{}` (not all headers are returned) |

## Summary

```json
{
  "wall_clock_ms": 122.456,
  "first_non_ok_layer": "tls",
  "exit_code": 40
}
```

| Field | Type | Description |
|-------|------|-------------|
| `wall_clock_ms` | float64 | Total elapsed time in milliseconds |
| `first_non_ok_layer` | string | Name of the first layer that was not `ok`, or `""` if all passed |
| `exit_code` | int | Process exit code (see [Exit Codes](#exit-codes)) |

## Error Codes

All error codes follow the pattern `{LAYER}_{CONDITION}`. The `code` field is the stable interface for programmatic branching.

**Error code principle:** Only conditions that can be reliably distinguished via Go types or errno values are contract codes. Conditions that depend on string matching are best-effort hints in `observations`, not contract codes.

### DNS Errors

| Code | Description |
|------|-------------|
| `DNS_NXDOMAIN` | Domain does not exist |
| `DNS_TIMEOUT` | DNS resolution timed out |
| `DNS_ERROR` | Other DNS error (fallback) |

Note: `DNS_SERVFAIL`, `DNS_REFUSED`, and `DNS_NO_ANSWER` are not contract error codes because they depend on Go internal string matching. These conditions are reported as best-effort hints in the `dns_error_hint` observation field.

### Reachability Errors

| Code | Description |
|------|-------------|
| `REACHABILITY_TIMEOUT` | ICMP probe timed out (no response) |
| `REACHABILITY_ERROR` | Other ICMP error (fallback) |

### TCP Errors

| Code | Description |
|------|-------------|
| `TCP_TIMEOUT` | TCP connection timed out |
| `TCP_REFUSED` | Connection refused (port closed) |
| `TCP_HOST_UNREACHABLE` | Host is unreachable |
| `TCP_NETWORK_UNREACHABLE` | Network is unreachable |
| `TCP_ERROR` | Other TCP error (fallback) |

### TLS Errors

| Code | Description |
|------|-------------|
| `TLS_CERT_EXPIRED` | Certificate has expired |
| `TLS_CERT_NOT_YET_VALID` | Certificate is not yet valid |
| `TLS_CERT_EXPIRING_SOON` | Certificate expires within 30 days (warning) |
| `TLS_HOSTNAME_MISMATCH` | Certificate does not match the hostname |
| `TLS_UNTRUSTED_CHAIN` | Certificate chain is not trusted |
| `TLS_NO_CERTIFICATES` | Server returned no certificates |
| `TLS_HANDSHAKE_TIMEOUT` | TLS handshake timed out |
| `TLS_PROTOCOL_ERROR` | TLS protocol error (best-effort classification) |
| `TLS_DEPRECATED_VERSION_ENABLED` | Server supports deprecated TLS versions (warning, via `--tls-scan`) |
| `TLS_ERROR` | Other TLS error (fallback) |

### HTTP Errors

| Code | Description |
|------|-------------|
| `HTTP_401` | 401 Unauthorized |
| `HTTP_403` | 403 Forbidden |
| `HTTP_404` | 404 Not Found |
| `HTTP_429` | 429 Too Many Requests |
| `HTTP_500` | 500 Internal Server Error |
| `HTTP_502` | 502 Bad Gateway |
| `HTTP_503` | 503 Service Unavailable |
| `HTTP_504` | 504 Gateway Timeout |
| `HTTP_4XX` | Other 4xx status (fallback for unknown 4xx) |
| `HTTP_5XX` | Other 5xx status (fallback for unknown 5xx) |
| `HTTP_TIMEOUT` | HTTP request timed out |
| `HTTP_ERROR` | Other HTTP error (fallback) |

### Tool Errors

| Code | Description |
|------|-------------|
| `INVALID_TARGET` | Target URL could not be parsed |
| `INVALID_ARGS` | Invalid command-line arguments |

### Error Code Normalization Rules

1. **Known specific codes first** — codes from the tables above are used when the condition matches exactly
2. **Fallback to `*_ERROR`** — unclassifiable errors use the layer's generic code (`DNS_ERROR`, `TCP_ERROR`, etc.)
3. **HTTP fixed set** — status codes 401, 403, 404, 429, 500, 502, 503, 504 get individual `HTTP_{N}` codes
4. **Unknown 5xx** — 5xx codes outside the fixed set become `HTTP_5XX`
5. **Unknown 4xx** — 4xx codes outside the fixed set become `HTTP_4XX`

## Exit Codes

| Code | Category | Triggered by |
|------|----------|-------------|
| 0 | Success | All layers `ok` or `skip` |
| 1 | Tool error | `INVALID_TARGET`, `INVALID_ARGS` |
| 2 | Warning | Any layer `warn`, none `fail` |
| 10 | DNS failure | First failing layer is DNS |
| 15 | Reachability failure | First failing layer is Reachability |
| 20 | TCP failure | First failing layer is TCP |
| 30 | TLS failure | First failing layer is TLS |
| 40 | HTTP failure | First failing layer is HTTP |

Exit codes reflect the **first failing layer** in execution order (DNS → Reachability → TCP → TLS → HTTP).

## Target Schemes and Layer Execution

| Target | Layers executed |
|--------|----------------|
| `https://host/path` | DNS → Reachability → TCP → TLS → HTTP |
| `http://host/path` | DNS → Reachability → TCP → HTTP |
| `tcp://host:port` | DNS → Reachability → TCP |
| `host` (bare) | Treated as `https://host` |

Layers not executed for a given scheme have `status: "skip"`.

## Behavioral Rules

### Status Classification

- **DNS**: `ok` if at least one IP resolved; `fail` on NXDOMAIN, timeout, etc.
- **Reachability**: `ok` if ICMP probe received a response; `fail` on timeout or error; `skip` if ICMP is not permitted (permission denied).
- **TCP**: `ok` if connection established; `fail` on timeout, refused, etc.
- **TLS**: `ok` if handshake succeeded and cert is valid; `warn` if cert expires within 30 days; `fail` on expired cert, hostname mismatch, untrusted chain, etc.
- **HTTP**: `ok` if status code < 400; `fail` if status code >= 400. 3xx responses are treated as `ok` (redirects are **not** followed).

### Skip Invariants

When a layer has `status: "skip"`:
- `error` is `null`
- For layers skipped because a prior layer failed or the layer is not applicable:
  - `duration_ms` is `0`
  - `observations` is `{}` (empty object)
- For the **Reachability** layer skipped due to runtime conditions (e.g., ICMP permission denied, unsupported address):
  - `duration_ms` reflects actual probe attempt time
  - `observations` contains diagnostic fields: `probe_method`, `reachable` (null), `rtt_ms` (null), `skip_reason`
  - This exception exists because the skip reason itself is diagnostic data useful to consumers

When ICMP permission is denied or the address type is unsupported, the Reachability layer uses `status: "skip"` (not `"fail"`). This ensures that lack of ICMP privileges does not block downstream layers.

### Timeout Behavior

The `--timeout` flag sets a **global** timeout (default: 10s) applied as a Go context deadline. All layers share this deadline — if DNS takes 8s of a 10s timeout, remaining layers have 2s.

### Redirect Policy

HTTP redirects are **not** followed. The response from the first request is reported. `CheckRedirect` returns `http.ErrUseLastResponse`.

### DNS Resolution

Uses the system resolver via Go's `net.LookupHost` by default. Custom resolver can be specified with `--dns-server <host>[:<port>]` (port defaults to 53).

### TLS / `--insecure`

`--insecure` sets `InsecureSkipVerify: true` on the TLS config, skipping both certificate chain validation and hostname verification.

### Proxy Behavior

- **HTTP layer**: Go's `net/http` respects `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables by default via `http.ProxyFromEnvironment`. When set, HTTP layer diagnostics are routed through the configured proxy.
- **TCP and TLS layers**: Use raw `net.Dialer` for direct connections. Proxy environment variables are **not** respected. This is by design — these layers diagnose the actual network path, not the proxied path.
- **Reachability layer**: Uses raw ICMP sockets. No proxy concept applies.
- **DNS layer**: Uses the system resolver or `--dns-server`. No proxy concept applies.

### Partial Observations on Failure

Layers may return partial `observations` on failure. For example, an HTTP layer that fails to connect still includes `"method"` in observations.

## `--tls-scan` Mode

When `--tls-scan` is used, the TLS layer's `observations` include an additional `tls_scan` object:

```json
{
  "tls_scan": {
    "performed": true,
    "attempts": [
      { "version": "TLSv1.0", "supported": false, "duration_ms": 12.3, "error": { "code": "TLS_PROTOCOL_ERROR", "message": "..." } },
      { "version": "TLSv1.1", "supported": false, "duration_ms": 11.1, "error": { "code": "TLS_PROTOCOL_ERROR", "message": "..." } },
      { "version": "TLSv1.2", "supported": true, "duration_ms": 15.2, "error": null },
      { "version": "TLSv1.3", "supported": true, "duration_ms": 14.8, "error": null }
    ],
    "supported_versions": ["TLSv1.2", "TLSv1.3"],
    "deprecated_versions_enabled": []
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `performed` | bool | Always `true` when `--tls-scan` is used |
| `attempts` | array | One entry per TLS version probed (1.0, 1.1, 1.2, 1.3) |
| `attempts[].version` | string | TLS version string |
| `attempts[].supported` | bool | Whether the server accepted this version |
| `attempts[].duration_ms` | float64 | Handshake duration |
| `attempts[].error` | object\|null | Error details if handshake failed |
| `supported_versions` | []string | List of supported TLS versions |
| `deprecated_versions_enabled` | []string | Deprecated versions (TLSv1.0, TLSv1.1) the server still supports |

If `deprecated_versions_enabled` is non-empty and the normal probe succeeded, the TLS layer status is upgraded to `warn` with error code `TLS_DEPRECATED_VERSION_ENABLED`.

## `--count N` Mode

When `--count N` is used, the output structure changes to a `CountResult`:

```json
{
  "schema_version": "v0.1",
  "target": "https://example.com",
  "count": 5,
  "exit_code": 0,
  "attempts": [
    {
      "attempt": 1,
      "started_at": "2026-02-26T18:42:03Z",
      "layers": { "dns": {...}, "reachability": {...}, "tcp": {...}, "tls": {...}, "http": {...} },
      "summary": { "wall_clock_ms": 120, "first_non_ok_layer": "", "exit_code": 0 }
    }
  ],
  "statistics": {
    "dns": { "p50_ms": 8.5, "p95_ms": 12.1, "success_count": 5, "fail_count": 0, "skip_count": 0, "sample_count": 5, "loss_ratio": 0 },
    "reachability": { "..." : "..." },
    "tcp": { "..." : "..." },
    "tls": { "..." : "..." },
    "http": { "..." : "..." }
  }
}
```

### CountResult Fields

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Schema version identifier |
| `target` | string | Original target URL |
| `count` | int | Number of attempts requested |
| `exit_code` | int | Worst exit code across all attempts |
| `attempts` | array | Individual attempt results (same structure as single-run) |
| `statistics` | object | Per-layer aggregate statistics |

### LayerStatistics Fields

| Field | Type | Description |
|-------|------|-------------|
| `p50_ms` | float64\|null | Median duration (null if no successful samples) |
| `p95_ms` | float64\|null | 95th percentile duration (null if no successful samples) |
| `success_count` | int | Number of attempts where this layer succeeded |
| `fail_count` | int | Number of attempts where this layer failed |
| `skip_count` | int | Number of attempts where this layer was skipped |
| `sample_count` | int | Number of non-skipped attempts (`success_count + fail_count`) |
| `loss_ratio` | float64 | Fraction of failed attempts (0.0 to 1.0) |

## Tool Error JSON

When `--json` is used and an argument or target parsing error occurs, stackdiag outputs a structured error instead of plain text:

```json
{
  "schema_version": "v0.1",
  "error": {
    "code": "INVALID_TARGET",
    "message": "unsupported scheme: \"ftp\""
  },
  "exit_code": 1
}
```

This ensures programmatic consumers always receive parseable JSON, even for tool errors.

## Schema Contract

These guarantees hold **within a given schema version** after publication:

1. **No field removals** — fields are never removed within a schema version
2. **No type changes** — a field's type never changes within a schema version
3. **Additions only** — new fields may be added (backward-compatible)
4. **Consistent structure on error** — the JSON shape is identical for success, warning, failure, and skip
5. **Skip, not null** — unused layers have `status: "skip"`, not `null`

Breaking changes (field removals, type changes) are permitted only across major schema version boundaries (e.g., v0.1 to v0.1). Consumers should check `schema_version` to select the appropriate parser.

## Full Example

```json
{
  "schema_version": "v0.1",
  "started_at": "2026-02-26T18:42:03Z",
  "target": "https://api.example.com/health",
  "layers": {
    "dns": {
      "status": "ok",
      "duration_ms": 9,
      "observations": {
        "query_name": "api.example.com",
        "answers": ["203.0.113.10"],
        "dns_error_hint": null,
        "ttl": 300,
        "resolver_address": null
      },
      "error": null
    },
    "reachability": {
      "status": "ok",
      "duration_ms": 12,
      "observations": {
        "probe_method": "icmp",
        "reachable": true,
        "rtt_ms": 11.5,
        "skip_reason": null
      },
      "error": null
    },
    "tcp": {
      "status": "ok",
      "duration_ms": 16,
      "observations": {
        "remote_ip": "203.0.113.10",
        "remote_port": 443
      },
      "error": null
    },
    "tls": {
      "status": "warn",
      "duration_ms": 31,
      "observations": {
        "version": "TLSv1.3",
        "cipher_suite": "TLS_AES_256_GCM_SHA384",
        "cert_days_until_expiry": 5,
        "cert_hostname_match": true,
        "cert_verified": true,
        "cert_subject": "api.example.com",
        "cert_san": ["api.example.com", "*.example.com"],
        "cert_issuer": "R3",
        "cert_not_after": "2026-03-05T12:00:00Z",
        "cert_not_before": "2025-12-05T12:00:00Z",
        "cert_chain": [
          {"subject": "api.example.com", "issuer": "R3", "not_after": "2026-03-05T12:00:00Z"},
          {"subject": "R3", "issuer": "ISRG Root X1", "not_after": "2035-09-15T16:00:00Z"}
        ]
      },
      "error": {
        "code": "TLS_CERT_EXPIRING_SOON",
        "message": "Certificate expires in 5 days"
      }
    },
    "http": {
      "status": "fail",
      "duration_ms": 57,
      "observations": {
        "method": "GET",
        "protocol": "HTTP/2",
        "status_code": 503,
        "status_text": "Service Unavailable",
        "response_headers": {
          "content-type": "application/json",
          "retry-after": "30"
        }
      },
      "error": {
        "code": "HTTP_503",
        "message": "503 Service Unavailable"
      }
    }
  },
  "summary": {
    "wall_clock_ms": 134,
    "first_non_ok_layer": "tls",
    "exit_code": 40
  }
}
```

## Agent Usage Examples

### Parse with jq

```bash
# Get the status of a specific layer
stdiag --json https://example.com 2>/dev/null | jq -r '.layers.tls.status'

# Check reachability
stdiag --json https://example.com 2>/dev/null | jq -r '.layers.reachability.observations.reachable'

# Extract the first failing layer
stdiag --json https://example.com 2>/dev/null | jq -r '.summary.first_non_ok_layer'

# Get all error codes
stdiag --json https://example.com 2>/dev/null | jq '[.layers[] | select(.error != null) | .error.code]'

# Check if all layers passed
stdiag --json https://example.com 2>/dev/null | jq '.summary.exit_code == 0'
```

### Use in scripts

```bash
#!/bin/bash
RESULT=$(stdiag --json "$TARGET" 2>/dev/null)
EXIT=$?

case $EXIT in
  0)  echo "All layers healthy" ;;
  2)  echo "Warning: $(echo "$RESULT" | jq -r '.layers[] | select(.status=="warn") | .error.code')" ;;
  10) echo "DNS failure" ;;
  15) echo "Reachability failure" ;;
  20) echo "TCP failure" ;;
  30) echo "TLS failure" ;;
  40) echo "HTTP failure" ;;
  *)  echo "Tool error" ;;
esac
```

### Wrap as an MCP tool

stackdiag is stateless and writes structured JSON to stdout — it can be wrapped directly as a tool in any MCP server:

```json
{
  "name": "network_diagnose",
  "description": "Diagnose network connectivity layer by layer",
  "inputSchema": {
    "type": "object",
    "properties": {
      "target": { "type": "string", "description": "URL to diagnose (e.g., https://api.example.com)" }
    },
    "required": ["target"]
  }
}
```

Execute: `stdiag --json <target> 2>/dev/null`
