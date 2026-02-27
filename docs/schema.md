# probe JSON Schema Reference

> Schema version: **v0.1**

This document is the complete specification for probe's `--json` output.

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
| `started_at` | string | ISO 8601 UTC timestamp of probe start |
| `target` | string | Original target as provided by the user |
| `layers` | object | Per-layer results (see below) |
| `summary` | object | Aggregate probe information |

## Layers

The `layers` object always contains exactly four keys: `dns`, `tcp`, `tls`, `http`. Layers that were not executed have `status: "skip"`.

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

### HTTP

| Field | Type | Description |
|-------|------|-------------|
| `method` | string | HTTP method used |
| `protocol` | string | HTTP protocol (e.g., `"HTTP/2"`) |
| `status_code` | int | HTTP response status code |
| `request_headers` | object | Request headers sent (sensitive values redacted by default) |

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

### DNS Errors

| Code | Description |
|------|-------------|
| `DNS_NXDOMAIN` | Domain does not exist |
| `DNS_TIMEOUT` | DNS resolution timed out |
| `DNS_SERVFAIL` | DNS server returned SERVFAIL |
| `DNS_REFUSED` | DNS server refused the query |
| `DNS_NO_ANSWER` | DNS server returned no answer records |
| `DNS_ERROR` | Other DNS error (fallback) |

### TCP Errors

| Code | Description |
|------|-------------|
| `TCP_TIMEOUT` | TCP connection timed out |
| `TCP_REFUSED` | Connection refused (port closed) |
| `TCP_RESET` | Connection reset by peer |
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
| `TLS_HANDSHAKE_TIMEOUT` | TLS handshake timed out |
| `TLS_PROTOCOL_ERROR` | TLS protocol error |
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
5. **Unknown 4xx** — 4xx codes outside the fixed set become `HTTP_{N}` with the actual status code

## Exit Codes

| Code | Category | Triggered by |
|------|----------|-------------|
| 0 | Success | All layers `ok` or `skip` |
| 1 | Tool error | `INVALID_TARGET`, `INVALID_ARGS` |
| 2 | Warning | Any layer `warn`, none `fail` |
| 10 | DNS failure | First failing layer is DNS |
| 20 | TCP failure | First failing layer is TCP |
| 30 | TLS failure | First failing layer is TLS |
| 40 | HTTP failure | First failing layer is HTTP |

Exit codes reflect the **first failing layer** in execution order (DNS → TCP → TLS → HTTP).

## Target Schemes and Layer Execution

| Target | Layers executed |
|--------|----------------|
| `https://host/path` | DNS → TCP → TLS → HTTP |
| `http://host/path` | DNS → TCP → HTTP |
| `tcp://host:port` | DNS → TCP |
| `host` (bare) | Treated as `https://host` |

Layers not executed for a given scheme have `status: "skip"`.

## Behavioral Rules

### Status Classification

- **DNS**: `ok` if at least one IP resolved; `fail` on NXDOMAIN, timeout, SERVFAIL, etc.
- **TCP**: `ok` if connection established; `fail` on timeout, refused, reset, etc.
- **TLS**: `ok` if handshake succeeded and cert is valid; `warn` if cert expires within 30 days; `fail` on expired cert, hostname mismatch, untrusted chain, etc.
- **HTTP**: `ok` if status code < 400; `fail` if status code >= 400. 3xx responses are treated as `ok` (redirects are **not** followed).

### Skip Invariants

When a layer has `status: "skip"`:
- `duration_ms` is `0`
- `observations` is `{}` (empty object)
- `error` is `null`

### Timeout Behavior

The `--timeout` flag sets a **global** timeout (default: 10s) applied as a Go context deadline. All layers share this deadline — if DNS takes 8s of a 10s timeout, remaining layers have 2s.

### Redirect Policy

HTTP redirects are **not** followed. The response from the first request is reported. `CheckRedirect` returns `http.ErrUseLastResponse`.

### DNS Resolution

Uses the system resolver via Go's `net.LookupHost`. No custom DNS server configuration in v0.1.

### TLS / `--insecure`

`--insecure` sets `InsecureSkipVerify: true` on the TLS config, skipping both certificate chain validation and hostname verification.

### Partial Observations on Failure

Layers may return partial `observations` on failure. For example, an HTTP layer that fails to connect still includes `"method"` in observations.

## Schema Contract

These guarantees hold across all versions:

1. **No field removals** — fields are never removed from the schema
2. **No type changes** — a field's type never changes
3. **Additions only** — new fields may be added (backward-compatible)
4. **Consistent structure on error** — the JSON shape is identical for success, warning, failure, and skip
5. **Skip, not null** — unused layers have `status: "skip"`, not `null`

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
        "answers": ["203.0.113.10"]
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
        "cert_hostname_match": true
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
        "status_code": 503
      },
      "error": {
        "code": "HTTP_503",
        "message": "503 Service Unavailable"
      }
    }
  },
  "summary": {
    "wall_clock_ms": 122,
    "first_non_ok_layer": "tls",
    "exit_code": 40
  }
}
```

## Agent Usage Examples

### Parse with jq

```bash
# Get the status of a specific layer
probe --json https://example.com 2>/dev/null | jq -r '.layers.tls.status'

# Extract the first failing layer
probe --json https://example.com 2>/dev/null | jq -r '.summary.first_non_ok_layer'

# Get all error codes
probe --json https://example.com 2>/dev/null | jq '[.layers[] | select(.error != null) | .error.code]'

# Check if all layers passed
probe --json https://example.com 2>/dev/null | jq '.summary.exit_code == 0'
```

### Use in scripts

```bash
#!/bin/bash
RESULT=$(probe --json "$TARGET" 2>/dev/null)
EXIT=$?

case $EXIT in
  0)  echo "All layers healthy" ;;
  2)  echo "Warning: $(echo "$RESULT" | jq -r '.layers[] | select(.status=="warn") | .error.code')" ;;
  10) echo "DNS failure" ;;
  20) echo "TCP failure" ;;
  30) echo "TLS failure" ;;
  40) echo "HTTP failure" ;;
  *)  echo "Tool error" ;;
esac
```

### Wrap as an MCP tool

probe is stateless and writes structured JSON to stdout — it can be wrapped directly as a tool in any MCP server:

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

Execute: `probe --json <target> 2>/dev/null`
