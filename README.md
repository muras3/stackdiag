# stackdiag

[![CI](https://github.com/muras3/stackdiag/actions/workflows/ci.yml/badge.svg)](https://github.com/muras3/stackdiag/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](https://go.dev)

**One command replaces `dig` + `ping` + `openssl` + `curl`.**
Structured evidence for AI agents. Layer-by-layer network diagnostics for humans.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/before-after.svg">
  <img alt="stackdiag compresses 4 commands × 4 agent loops into 1 command × 1 loop" src="docs/before-after.svg" width="820">
</picture>

```bash
$ stdiag --json https://api.example.com
# → DNS ✓ → Reachability ✓ → TCP ✓ → TLS ✗ CERT_EXPIRED (exit 30)
# One structured JSON. One tool call. One reasoning loop.
```

![demo](demo.gif)

## Install

```bash
# macOS / Linux
curl -sL https://github.com/muras3/stackdiag/releases/latest/download/stackdiag_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz | tar xz
sudo mv stdiag /usr/local/bin/

# Go
go install github.com/muras3/stackdiag/cmd/stackdiag@latest
```

## Quick Start

```bash
stdiag https://example.com                       # full stack check
stdiag --json https://example.com                # JSON for agents
stdiag tcp://db.internal:5432                    # TCP connectivity only
stdiag --tls-scan https://example.com            # TLS version audit
stdiag --count 5 https://example.com             # repeated measurement
stdiag --dns-server 8.8.8.8 https://example.com # custom DNS resolver
```

Exit codes tell you which layer broke — no JSON parsing needed:

| Exit | Layer |
|------|-------|
| 0 | All OK |
| 2 | Warning (no failure) |
| 10 / 15 / 20 / 30 / 40 | DNS / Reachability / TCP / TLS / HTTP |
| 1 | Tool error |

## Supported Platforms

- Go: `1.26+`
- Release binaries: `linux/darwin` × `amd64/arm64`
- Windows binaries are not published in current release configuration.

## Known Limitations

- `request_headers` redaction is applied to known sensitive header names (for example `Authorization`, `Cookie`, `X-Access-Token`).
- Reachability may be `skip` on environments without ICMP privileges.
- HTTP redirects are not followed (first response is reported).

## Why stackdiag

- **Compression, not replacement.** stackdiag compresses the common `dig` → `openssl` → `curl` triage path into one deterministic command. In the benchmark harness (`bench/`, 24 scenarios), total token count was `9,994` (`stdiag --json`) vs `52,436` (manual commands), about **80.9% fewer tokens**. Use raw tools when you need deep manual forensics.

- **Deterministic classification.** Primary error codes (for example `TLS_CERT_EXPIRED`, `DNS_NXDOMAIN`) are classified with stable type/errno checks. Some fallback classifications (for example `TLS_PROTOCOL_ERROR`) are best-effort and documented in schema docs. Agents can branch on `error.code` without parsing free-form CLI text.

- **Schema contract.** `--json` output follows a versioned schema — fields are never removed, types never changed. `stdout` = data, `stderr` = logs. Wrap as an MCP tool or LangChain `tool_call` with zero adaptation.

## Docs

- [JSON Schema & Error Codes](docs/schema.md) — output format, typed fields, exit code contract
- [Design Decisions](docs/design-decisions.md) — architecture rationale
- [Benchmarks](bench/) — reproducible token efficiency measurements

## Contribution & Security (Temporary)

- Feature requests and bug reports via GitHub Issues are especially helpful.
- PRs are also welcome, but opening an issue first helps us align on scope and verification.
- For security issues, please do not open a public issue. Contact the maintainer via X (Twitter) DM: [@tomatolinux](https://x.com/tomatolinux)

## License

[MIT](LICENSE)
