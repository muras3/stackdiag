# stackdiag

[![CI](https://github.com/muras3/stackdiag/actions/workflows/ci.yml/badge.svg)](https://github.com/muras3/stackdiag/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev)

Structured network diagnostics for AI agents and humans.

![demo](demo.gif)

One command to diagnose DNS → Reachability → TCP → TLS → HTTP — layer by layer. Stops at the first failure. Tells you which layer broke.

## Install

```bash
# Binary (Linux amd64)
curl -sL https://github.com/muras3/stackdiag/releases/latest/download/stdiag_linux_amd64.tar.gz | tar xz
sudo mv stdiag /usr/local/bin/

# Binary (macOS Apple Silicon)
curl -sL https://github.com/muras3/stackdiag/releases/latest/download/stdiag_darwin_arm64.tar.gz | tar xz
sudo mv stdiag /usr/local/bin/

# Go
go install github.com/muras3/stackdiag/cmd/stackdiag@latest
```

## Usage

```bash
stdiag https://example.com                  # HTTPS check
stdiag --json https://example.com           # JSON output
stdiag tcp://db.internal:5432               # TCP only
stdiag --tls-scan https://example.com       # TLS version scan
stdiag --count 5 https://example.com        # Repeated measurement
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All layers passed |
| 1 | Tool error |
| 10 / 15 / 20 / 30 / 40 | DNS / Reachability / TCP / TLS / HTTP failure |

```bash
stdiag https://api.example.com || echo "exit: $?"
```

## For AI Agents

`--json` outputs stable, structured JSON with machine-readable error codes (`DNS_NXDOMAIN`, `TLS_CERT_EXPIRED`, `HTTP_503`), typed fields, and deterministic structure. `stdout` = data, `stderr` = logs.

Wrap `stdiag --json <target>` as a tool in any MCP server — no adapter needed.

Full schema: [docs/schema.md](docs/schema.md)

Note: HTTP layer respects `HTTP_PROXY`/`HTTPS_PROXY` env vars. TCP/TLS/ICMP layers connect directly.

## Docs

- [`stdiag --help`](docs/schema.md) — All CLI flags and options
- [docs/schema.md](docs/schema.md) — JSON schema, error codes, exit codes
- [docs/design-decisions.md](docs/design-decisions.md) — Architecture decisions

## License

[MIT](LICENSE)
