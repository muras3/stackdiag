# probe — Product Design

> **probe** — structured network diagnostics for AI agents and humans.

## Problem

「HTTPSが繋がらない」とき、どこで止まっているか分からない。
今は4コマンド（dig, nc, openssl, curl）を毎回手で打っている。

## Solution

1コマンドでDNS→TCP→TLS→HTTPの全レイヤーを切り分ける。

```
$ probe https://api.example.com/health

  dns   ✓   9ms   api.example.com → 203.0.113.10
  tcp   ✓  16ms   :443
  tls   ⚠  31ms   TLSv1.3, cert expires in 5d
  http  ✗  57ms   503 Service Unavailable

  122ms total | first issue: tls | exit 1
```

## Core Value

- **人間向け:** 障害初動を数十秒短縮する
- **Agent向け:** 安定した判断インターフェース（構造化JSON）を公開する

「for AI agents」が先。「and humans」が後。この順序が設計思想。

## Technical Decisions

| 項目 | 決定 | 理由 |
|------|------|------|
| 言語 | Go | stdlib net/tls/httpが最適。開発速度と保守性優先 |
| 配布 | シングルバイナリ | ゼロ依存、5MB以下目標 |
| 出力 | table（default）+ JSON（--json） | 内部データモデル=JSON。tableはそのレンダリング |
| MCP | MVP不要 | ステートレスなのでCLI+--jsonで十分。将来アダプタを被せられる設計 |

## Architecture

```
Runner (core) → typed Result struct
                  ├→ JSON renderer  (--json)
                  └→ Table renderer (default, screenshot-worthy)
```

内部データモデル＝JSON出力。人間向け出力はその上のレンダリング層。

## Agent-Friendly Design

Agent-friendlyとは「`--json`がある」ことではない。「安定した判断インターフェースを公開している」こと。

Agentがツールに求める3つ：

| 問い | probeの答え方 |
|------|-------------|
| 何が起きた？ | `layers.tls.status: "warn"` |
| なぜ？ | `error.code: "TLS_CERT_EXPIRING_SOON"` |
| 次どうする？ | exit code + error codeで分岐可能 |

具体的なAgent-friendly特性：
- 安定したコマンド文法（曖昧な位置引数なし）
- 決定的な出力構造（schema_version付き）
- 型付きフィールド（boolean/number/string/enum、散文禁止）
- 明示的な単位（`*_ms`、`"120ms"`ではなく数値）
- stdout=データ、stderr=ログの厳格分離
- 構造化エラーコード（`DNS_TIMEOUT`、`TLS_HANDSHAKE_FAILED`等）
- 信頼できるexit code

## JSON Schema (v0.1)

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
        "cert_days_until_expiry": 5
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
    "exit_code": 1
  }
}
```

### Schema Contract

1. フィールドを削除しない
2. nullではなく `status: "skip"`
3. 型変更禁止
4. エラーでも同じ構造
5. 追加は許可（後方互換）

### Status Values

- `ok` — 正常
- `warn` — 警告（動作はする）
- `fail` — 失敗
- `skip` — 対象外（tcp://ならtls/httpはskip）

### Exit Codes

| code | 大分類 | 細分類例 |
|------|--------|---------|
| 0 | 正常 | — |
| 1 | ツールエラー | `INVALID_TARGET`, `INVALID_ARGS` |
| 2 | warn | `TLS_CERT_EXPIRING_SOON` |
| 10 | DNS障害 | `DNS_NXDOMAIN`, `DNS_TIMEOUT` |
| 20 | TCP障害 | `TCP_TIMEOUT`, `TCP_REFUSED` |
| 30 | TLS障害 | `TLS_CERT_EXPIRED`, `TLS_HOSTNAME_MISMATCH` |
| 40 | HTTP障害 | `HTTP_401`, `HTTP_503`, `HTTP_TIMEOUT` |

### Success Conditions

| ターゲット | 成功条件 |
|-----------|---------|
| `https://host/path` | dns + tcp + tls + http 全てok |
| `http://host/path` | dns + tcp + http 全てok |
| `tcp://host:port` | dns + tcp がok |

## MVP Scope (v0.1)

### IN

- `probe <url>`
- DNS / TCP / TLS / HTTP レイヤー
- `--json` / デフォルトtable
- `--method`, `--header`, `--timeout`, `--insecure`
- exit code（大分類）
- 色付き出力 + `NO_COLOR` 対応

### OUT

- root_cause / failure_chain / next_actions（Phase 2）
- 認証ヘルパー（Phase 1後半）
- UDP / QUIC / gRPC（Phase 3）
- MCP adapter（Phase 2）
- 複数ターゲット並列
- TUI / timing bar

## Phases

| Phase | 内容 |
|-------|------|
| MVP (v0.1) | HTTPS中心レイヤー切り分け + --json + table |
| 1後半 | 認証ヒント（--bearer-env等）、schema.md公開 |
| 2 | Rule Engine（evaluations/root_cause/failure_chain）、MCP adapter |
| 3 | UDP / Proxy / HTTP3 / gRPC |
