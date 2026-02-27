# stackdiag — Product Design

> **stackdiag** — structured network diagnostics for AI agents and humans.

## Problem

「HTTPSが繋がらない」とき、どこで止まっているか分からない。
今は4コマンド（dig, nc, openssl, curl）を毎回手で打っている。

## Solution

1コマンドでDNS→TCP→TLS→HTTPの全レイヤーを切り分ける。

```
$ stackdiag https://api.example.com/health

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

| 問い | stackdiagの答え方 |
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

### Timing方針

全レイヤー共通: `duration_ms`（float64、レイヤー実行時間）。
HTTPレイヤーのみ追加で `timing.ttfb_ms` / `timing.total_ms` を返すことがある（任意拡張、v0.1では未実装）。

### サンプル出力

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
    "exit_code": 1
  }
}
```

### Observations フィールド一覧

各レイヤーの `observations` に含まれるフィールド（実装準拠）:

| レイヤー | フィールド | 型 | 説明 |
|---------|-----------|-----|------|
| dns | `query_name` | string | 問い合わせホスト名 |
| dns | `answers` | []string | 解決されたIPアドレス群 |
| tcp | `remote_ip` | string | 接続先IPアドレス |
| tcp | `remote_port` | int | 接続先ポート番号 |
| tls | `version` | string | TLSバージョン（例: "TLSv1.3"） |
| tls | `cipher_suite` | string | 使用された暗号スイート |
| tls | `cert_days_until_expiry` | int | 証明書有効期限までの日数 |
| tls | `cert_hostname_match` | bool | 証明書がホスト名と一致するか |
| http | `method` | string | 使用されたHTTPメソッド |
| http | `protocol` | string | HTTPプロトコル（例: "HTTP/2"） |
| http | `status_code` | int | HTTPステータスコード |

### `--count N` 時の構造

`--count` 指定時、出力は反復計測構造に拡張される。

```json
{
  "schema_version": "v0.1",
  "count": 5,
  "exit_code": 20,
  "attempts": [
    {
      "attempt": 1,
      "started_at": "2026-02-27T08:00:00Z",
      "layers": {
        "dns": {"status": "ok", "duration_ms": 9, "observations": {"...": "..."}, "error": null},
        "tcp": {"status": "ok", "duration_ms": 16, "observations": {"...": "..."}, "error": null},
        "tls": {"status": "ok", "duration_ms": 31, "observations": {"...": "..."}, "error": null},
        "http": {"status": "ok", "duration_ms": 57, "observations": {"...": "..."}, "error": null}
      },
      "summary": {"wall_clock_ms": 122, "exit_code": 0}
    }
  ],
  "statistics": {
    "dns": {"p50_ms": 9.5, "p95_ms": 12, "success_count": 5, "fail_count": 0, "skip_count": 0, "sample_count": 5, "loss_ratio": 0.0},
    "tcp": {"p50_ms": 16, "p95_ms": 4500, "success_count": 4, "fail_count": 1, "skip_count": 0, "sample_count": 5, "loss_ratio": 0.2}
  }
}
```

ルール:
- `--count` 未指定時: `attempts`, `statistics`, `count` フィールドなし（既存構造維持）
- 判別子: `count` フィールドの有無
- p50/p95: 成功試行（ok/warn）の `duration_ms` のみ
- `loss_ratio`: `fail_count / sample_count`（skipは分母に入れない）
- `exit_code`: N回中の最悪ケース

### `--tls-scan` 時の observations

`--tls-scan` 指定時、TLSレイヤーの `observations` に `tls_scan` フィールドが追加される。

```json
"tls_scan": {
  "performed": true,
  "attempts": [
    {"version": "TLSv1.0", "supported": true, "duration_ms": 12.0, "error": null},
    {"version": "TLSv1.1", "supported": false, "duration_ms": 5.1, "error": {"code": "TLS_PROTOCOL_ERROR", "message": "..."}},
    {"version": "TLSv1.2", "supported": true, "duration_ms": 8.3, "error": null},
    {"version": "TLSv1.3", "supported": true, "duration_ms": 7.4, "error": null}
  ],
  "supported_versions": ["TLSv1.0", "TLSv1.2", "TLSv1.3"],
  "deprecated_versions_enabled": ["TLSv1.0"]
}
```

- deprecated検出時: `status=warn`, `error.code=TLS_DEPRECATED_VERSION_ENABLED`
- `--tls-scan` 未指定時: `tls_scan` フィールドなし

### 認証競合ルール

- `--header Authorization` と `--bearer-env` / `--basic-env` の同時指定はエラー
- `--bearer-env` と `--basic-env` の同時指定もエラー
- 環境変数が未設定/空/空白のみの場合もエラー

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
| 10 | DNS障害 | `DNS_NXDOMAIN`, `DNS_TIMEOUT`, `DNS_ERROR` |
| 20 | TCP障害 | `TCP_TIMEOUT`, `TCP_REFUSED`, `TCP_RESET`, `TCP_ERROR` |
| 30 | TLS障害 | `TLS_CERT_EXPIRED`, `TLS_CERT_NOT_YET_VALID`, `TLS_HOSTNAME_MISMATCH`, `TLS_UNTRUSTED_CHAIN`, `TLS_HANDSHAKE_TIMEOUT`, `TLS_ERROR` |
| 40 | HTTP障害 | `HTTP_401`, `HTTP_403`, `HTTP_404`, `HTTP_429`, `HTTP_500`, `HTTP_502`, `HTTP_503`, `HTTP_504`, `HTTP_TIMEOUT`, `HTTP_5XX`, `HTTP_ERROR` |

### Error Code 正規化規則

1. **既知の具体コード優先**: 上記テーブルの具体コード（例: `DNS_NXDOMAIN`, `HTTP_503`）が最優先
2. **フォールバック**: 分類不能な場合は `*_ERROR`（`DNS_ERROR`, `TCP_ERROR`, `TLS_ERROR`, `HTTP_ERROR`）にフォールバック
3. **HTTP固定集合**: 401, 403, 404, 429, 500, 502, 503, 504 は `HTTP_{N}` で個別コード化
4. **未知の5xx**: 固定集合外の5xxは `HTTP_5XX` にフォールバック
5. **未知の4xx**: 固定集合外の4xxは `HTTP_{N}`（実コード付き）を返す

### Success Conditions

| ターゲット | 成功条件 |
|-----------|---------|
| `https://host/path` | dns + tcp + tls + http 全てok |
| `http://host/path` | dns + tcp + http 全てok |
| `tcp://host:port` | dns + tcp がok |

## CLI機能スコープ

### MVP (v0.1)

- `stackdiag <url>`
- DNS / TCP / TLS / HTTP レイヤー
- `--json` / デフォルトtable
- `--method`, `--header`, `--timeout`, `--insecure`
- `--version`
- `--redact`（デフォルトON — Authorization等の機密ヘッダをマスク）
- exit code（大分類）
- 色付き出力 + `NO_COLOR` 対応
- MinVersion: `tls.VersionTLS12` 明示（TLS/HTTP両経路）
- `--bearer-env ENV_VAR`（環境変数からBearerトークン取得）
- `--basic-env ENV_VAR`（環境変数からBasic認証取得）
- `--tls-scan`（TLSバージョンスキャン: 1.0/1.1/1.2/1.3個別試行）
- `--count N`（反復計測 + p50/p95/loss統計）

### v0.2

（予約 — v0.1完了後に再検討）

### Phase 1

- `--verbose`（詳細出力）
- `dns://` スキーム（DNS単独プローブ）

### Phase 2

- `--header-file`（ファイルからヘッダ読み込み）
- `--netrc`（.netrcからの認証情報）

### OUT (将来検討)

- root_cause / failure_chain / next_actions（Phase 2）
- UDP / QUIC / gRPC（Phase 3）
- MCP adapter（Phase 2）
- 複数ターゲット並列
- TUI / timing bar

## Phases

| Phase | 内容 |
|-------|------|
| MVP (v0.1) | HTTPS中心レイヤー切り分け + --json + table + --redact + MinVersion TLS1.2 + --bearer-env + --basic-env + --tls-scan + --count N |
| v0.2 | （予約 — v0.1完了後に再検討） |
| 1 | --verbose, dns:// スキーム |
| 2 | 認証ヘルパー（--header-file, --netrc）、Rule Engine、MCP adapter |
| 3 | UDP / Proxy / HTTP3 / gRPC |
