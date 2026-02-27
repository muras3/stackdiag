# probe

> Structured network diagnostics for AI agents and humans.

One command to diagnose DNS → TCP → TLS → HTTP — layer by layer.

```
$ probe https://api.example.com/health

  dns   ✓   9ms   api.example.com → 203.0.113.10
  tcp   ✓  16ms   :443
  tls   ⚠  31ms   TLSv1.3, cert expires in 5d
  http  ✗  57ms   503 Service Unavailable

  122ms total | first issue: tls | exit 40
```

## TL;DR

- Diagnose DNS / TCP / TLS / HTTP in **one command**
- Stops at the first failure, tells you **which layer** broke
- `--json` + layer-based exit codes for **automation and AI agents**

## Why probe?

Debugging "HTTPS isn't working" today means running 4 commands: `dig`, `nc`, `openssl s_client`, `curl`. Each has different output formats, exit codes, and failure modes.

**probe** replaces that workflow with a single command that:

- Tests every layer in sequence (DNS → TCP → TLS → HTTP)
- Stops at the first failure and tells you exactly which layer broke
- Outputs structured JSON for automation (`--json`)
- Returns meaningful exit codes for scripting
- Works with zero dependencies — single static binary

## Install

**No runtime dependencies required.** probe is a single static binary.

```bash
# Download binary (Linux amd64)
curl -sL https://github.com/muras3/probe/releases/latest/download/probe_linux_amd64.tar.gz | tar xz
sudo mv probe /usr/local/bin/

# Download binary (macOS Apple Silicon)
curl -sL https://github.com/muras3/probe/releases/latest/download/probe_darwin_arm64.tar.gz | tar xz
sudo mv probe /usr/local/bin/

# Homebrew (macOS / Linux)
brew install muras3/tap/probe
```

<details>
<summary>Other methods</summary>

```bash
# Go (requires Go 1.21+)
go install github.com/muras3/probe/cmd/probe@latest

# From source
git clone https://github.com/muras3/probe.git
cd probe && make build
# Binary at ./bin/probe
```

</details>

<!-- TODO: goreleaser + Homebrew tap setup before v0.1 GA -->

## Quick Start

```bash
# Basic HTTPS check
probe https://example.com

# Check a specific endpoint with JSON output
probe --json https://api.example.com/health

# TCP-only connectivity test
probe tcp://db.internal:5432

# HTTP with custom method and headers
probe --method POST --header "Content-Type: application/json" https://api.example.com/v1/data

# Skip TLS certificate verification
probe --insecure https://self-signed.example.com
```

## Options

```
probe <url> [options]

Targets:
  https://host/path     Full HTTPS check (DNS + TCP + TLS + HTTP)
  http://host/path      HTTP check (DNS + TCP + HTTP, no TLS)
  tcp://host:port       TCP connectivity only (DNS + TCP)
  host                  Bare hostname defaults to https://

Options:
  --json                Output as JSON (default: table)
  --method METHOD       HTTP method (default: GET)
  --header KEY:VALUE    HTTP header (repeatable)
  --timeout N           Timeout in seconds (default: 10)
  --insecure            Skip TLS certificate verification
  --no-redact           Show sensitive header values (default: redacted)
  --version             Show version
```

## JSON Output

With `--json`, probe writes structured JSON to stdout:

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
    "tcp": { "status": "ok", "duration_ms": 16, "..." : "..." },
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
    "http": { "status": "fail", "duration_ms": 57, "..." : "..." }
  },
  "summary": {
    "wall_clock_ms": 122,
    "first_non_ok_layer": "tls",
    "exit_code": 40
  }
}
```

Full schema documentation: [docs/schema.md](docs/schema.md)

## Exit Codes

| Code | Meaning | Example error codes |
|------|---------|-------------------|
| 0 | All layers passed | — |
| 1 | Tool error | `INVALID_TARGET`, `INVALID_ARGS` |
| 2 | Warning (no failures) | `TLS_CERT_EXPIRING_SOON` |
| 10 | DNS failure | `DNS_NXDOMAIN`, `DNS_TIMEOUT`, `DNS_SERVFAIL` |
| 20 | TCP failure | `TCP_TIMEOUT`, `TCP_REFUSED`, `TCP_RESET` |
| 30 | TLS failure | `TLS_CERT_EXPIRED`, `TLS_HOSTNAME_MISMATCH` |
| 40 | HTTP failure | `HTTP_503`, `HTTP_429`, `HTTP_TIMEOUT` |

Exit codes indicate the **first failing layer**, so you can branch on them directly:

```bash
probe https://api.example.com || echo "exit code: $?"
```

## For AI Agents

probe is designed as a **tool interface for AI agents** — not just a CLI with `--json` bolted on.

**What makes it agent-friendly:**

- **Stable schema** — `schema_version` field, backward-compatible evolution, no field removals
- **Machine-readable error codes** — `DNS_NXDOMAIN`, `TLS_CERT_EXPIRED`, `HTTP_503` (not free-form text)
- **Typed fields** — booleans, numbers, enums. Durations in `*_ms` as numbers, not strings like `"120ms"`
- **Deterministic structure** — same JSON shape on success, warning, failure, or skip
- **Clean I/O separation** — `stdout` = data (JSON), `stderr` = logs. Parse stdout, ignore stderr
- **Meaningful exit codes** — 10/20/30/40 map directly to the failing layer

**Agent usage example:**

```bash
# Run probe, parse with jq, decide next action
RESULT=$(probe --json https://api.example.com 2>/dev/null)
STATUS=$(echo "$RESULT" | jq -r '.layers.tls.status')
if [ "$STATUS" = "fail" ]; then
  CODE=$(echo "$RESULT" | jq -r '.layers.tls.error.code')
  echo "TLS issue: $CODE"
fi
```

**MCP integration:** probe is stateless — wrap `probe --json <target>` as a tool in any MCP server. No adapter needed for basic usage.

## When to Use Something Else

- **DNS only?** Use `dig` or `dog` — they support all record types and transports
- **HTTP headers only?** Use `curl -I` — it's already everywhere
- **Load testing?** Use `hey` or `k6` — probe tests one request at a time
- **Full network monitoring?** Use Prometheus + Blackbox Exporter

probe is for **diagnosing connection failures** across the full stack, quickly.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and PR guidelines.

## License

[MIT](LICENSE)
