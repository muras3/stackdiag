# probe v0.1 全コードレビュー & テスト再実行計画

## Context

v0.1実装の全16タスクが完了済みだが、Task 15「Codexフルコードレビュー」が未実施。プロジェクト規定では **Code Reviewer: Codex（全コード）** が必須。本計画では Agent Teams を用いて最大並列でコードレビューを実施し、テストを再実行する。

## チーム構成: `probe-review`（8並列）

| エージェント名 | 役割 | 担当範囲 |
|---|---|---|
| **review-lead** (自分) | 統括・Config/CI レビュー・Codex連携・最終報告 | 全体調整 + Task 9,10,11 |
| **test-runner** | テスト実行 | make lint/test-race/build/e2e |
| **reviewer-core** | コア型レビュー | types, target, exitcode, fakes |
| **reviewer-dns** | DNSレイヤーレビュー | dns.go + dns_test.go |
| **reviewer-tcp** | TCPレイヤーレビュー | tcp.go + tcp_test.go |
| **reviewer-tls** | TLSレイヤーレビュー | tls.go + tls_test.go |
| **reviewer-http** | HTTPレイヤーレビュー | http.go + http_test.go |
| **reviewer-orch** | オーケストレーションレビュー | runner, cli, main.go, E2E |
| **reviewer-render** | レンダリングレビュー | JSON/table renderer, contract tests |

## tmux 可視化

Wave 1の8エージェントを全て `run_in_background: true` で起動し、各 `output_file` を tmux ペインで `tail -f` 表示。

**レイアウト（9ペイン）:**
```
┌─────────────────────────┬──────────────┬──────────────┐
│                         │ test-runner  │ reviewer-dns │
│                         ├──────────────┤──────────────┤
│                         │ reviewer-core│ reviewer-tcp │
│   Claude Code (Lead)    ├──────────────┤──────────────┤
│                         │ reviewer-orch│ reviewer-tls │
│                         ├──────────────┤──────────────┤
│                         │reviewer-render│reviewer-http│
└─────────────────────────┴──────────────┴──────────────┘
```

**手順:**
1. 全エージェントを `run_in_background: true` で起動
2. 返される `output_file` パスを収集
3. tmuxで8ペインを作成し、各ペインで `tail -f <output_file>` を実行

## 実行フロー

```
Wave 1（8並列・同時起動）:
  [test-runner]      make lint → test-race → build → e2e
  [reviewer-core]    types.go, target.go, exitcode.go, fakes.go + テスト
  [reviewer-dns]     dns.go + dns_test.go
  [reviewer-tcp]     tcp.go + tcp_test.go
  [reviewer-tls]     tls.go + tls_test.go
  [reviewer-http]    http.go + http_test.go
  [reviewer-orch]    runner.go, cli.go, main.go, e2e_test.go
  [reviewer-render]  json/renderer.go, table/renderer.go, contract_test.go + テスト

Wave 2（Wave 1完了後）:
  [review-lead]      Config/CIレビュー（Makefile, ci.yml, goreleaser, go.mod）
  [review-lead]      second-opinion スキルでCodexフルレビュー実行

Wave 3（Codex修正）:
  [review-lead]      Wave 1-2の指摘事項をもとにコード修正案を作成
  [review-lead]      second-opinion スキルでCodexに修正案のレビュー＆承認を依頼
  [review-lead]      Codex承認後にコード修正を適用

Wave 4:
  [review-lead]      テスト再実行（make lint → test-race → build → e2e）
  [review-lead]      全結果を統合 → 最終レビュー報告書
```

## 各タスク詳細

### Task 1: テスト実行 (test-runner)
- `make lint` — go vet + gofumpt
- `make test-race` — 全テスト（レースディテクタ付き）
- `make build` — バイナリビルド + サイズ確認（<5MB）
- `make e2e` — E2Eテスト
- 結果: pass/fail数、失敗詳細、実行時間

### Task 2: コア型レビュー (reviewer-core)
**対象ファイル（7ファイル, 688行）:**
- `internal/core/types.go` + `types_test.go`
- `internal/core/target.go` + `target_test.go`
- `internal/exitcode/exitcode.go` + `exitcode_test.go`
- `internal/testkit/fakes.go`

**チェックリスト:**
- JSONスキーマ準拠（Result/LayerResult/Summary が product-design.md と一致）
- Status enum完全性（ok, warn, fail, skip）
- `ShouldContinue()` ロジック正確性
- Exit code マッピング一致（0/1/2/10/20/30/40）
- `Layers` が `map[string]*LayerResult`（配列でなくオブジェクト、Decision 2）
- `error` フィールド命名（`issue`でない、Decision 3）
- Target parser: https/http/tcp/ベアホスト/IPv6/不正入力
- Design Decision 2,3,4,5 準拠

### Task 3: DNSレイヤーレビュー (reviewer-dns)
**対象ファイル:** `internal/layers/dns/dns.go` (72行) + `dns_test.go` (108行)
**チェックリスト:**
- `core.Layer` インターフェース実装
- NXDOMAIN/TIMEOUT/SERVFAIL エラー分類の正確性
- observations: query_name, answers 存在
- `Resolver` インターフェースによるDI（testkit.Resolver使用）
- ResolvedIPs を ProbeContext に伝播
- コンテキストデッドライン伝播
- テスト: 全failure patternカバー（NXDOMAIN, TIMEOUT, SERVFAIL, generic error）
- Decision 5（observations layer-local）準拠

### Task 4: TCPレイヤーレビュー (reviewer-tcp)
**対象ファイル:** `internal/layers/tcp/tcp.go` (93行) + `tcp_test.go` (228行)
**チェックリスト:**
- `core.Layer` インターフェース実装
- REFUSED/TIMEOUT/RESET エラー分類の正確性
- observations: remote_ip, remote_port 存在
- `Dialer` インターフェースによるDI（testkit.Dialer使用）
- ResolvedIPs からの接続先IP選択ロジック
- フォールバック（ResolvedIPs空時のHostPort使用）
- コンテキストデッドライン伝播
- テスト: 実TCPリスナー使用、全failure patternカバー

### Task 5: TLSレイヤーレビュー (reviewer-tls)
**対象ファイル:** `internal/layers/tls/tls.go` (210行) + `tls_test.go` (360行)
**チェックリスト:**
- `core.Layer` インターフェース実装
- `Handshaker` インターフェースによるDI
- CERT_EXPIRED/EXPIRING_SOON/HOSTNAME_MISMATCH/UNTRUSTED_CHAIN/HANDSHAKE_TIMEOUT エラー分類
- observations: version, cipher_suite, cert_days_until_expiry, cert_hostname_match
- 証明書期限閾値: 30日未満でwarn、0日未満でfail
- `--insecure` モード動作（ServerName検証スキップ）
- TLSバージョン文字列変換（TLSv1.0〜1.3）
- Duration: ハンドシェイクのみ計測（TCP接続時間除外）
- テスト: 自己署名証明書生成、全failure patternカバー
- **Core Tech（Codex担当領域）: 計測セマンティクス重点確認**

### Task 6: HTTPレイヤーレビュー (reviewer-http)
**対象ファイル:** `internal/layers/http/http.go` (179行) + `http_test.go` (311行)
**チェックリスト:**
- `core.Layer` インターフェース実装
- 401/403/404/429/500/502/503/504 エラー分類
- observations: method, protocol, status_code
- `CheckRedirect` で自動リダイレクト無効化
- `DisableKeepAlives: true` 設定
- カスタムMethod/Headers対応
- Resolved IP使用時のHost headerセット
- Duration: TTFB計測（リクエスト送信→最初のレスポンスバイト）
- `--insecure` モード: TLS検証スキップ
- テスト: httptest.Server使用、全status code + timeout + reset
- **Core Tech（Codex担当領域）: 計測セマンティクス重点確認**

### Task 7: オーケストレーションレビュー (reviewer-orch)
**対象ファイル（6ファイル, 716行）:**
- `internal/runner/runner.go` + `runner_test.go`
- `internal/cli/cli.go` + `cli_test.go`
- `cmd/probe/main.go`
- `test/e2e/probe_test.go`

**チェックリスト:**
- Runner: 順次実行順序（dns→tcp→tls→http）
- Runner: fail停止、warn継続
- Runner: skip時は `status: "skip"`（nullでない、スキーマ契約ルール2）
- Runner: `first_non_ok_layer` 追跡
- Runner: `wall_clock_ms` 計算
- CLI: 全フラグ正常パース（--json, --method, --header, --timeout, --insecure, --version）
- main.go: stdout/stderr分離（Decision 8）
- main.go: NO_COLOR/非TTY検出（Decision 7）
- main.go: `buildLayers()` の NeedsTLS/NeedsHTTP 使用
- E2E: バイナリ実行テスト、JSON出力検証、終了コード検証
- Decision 7,8,9 準拠

### Task 8: レンダリングレビュー (reviewer-render)
**対象ファイル（5ファイル, 889行）:**
- `internal/render/json/renderer.go` + `renderer_test.go` + `contract_test.go`
- `internal/render/table/renderer.go` + `renderer_test.go`

**チェックリスト:**
- JSON: HTMLエスケープ無効、インデント付き
- JSON: 内部データモデル = JSON出力（レンダリング層のみ）
- Contract tests: product-design.md のスキーマフィールド全検証
- Table: カラー記号（✓⚠✗）、NO_COLOR/ASCIIフォールバック
- Table: タイミング右揃え、サマリ行フォーマット
- Table: レイヤー表示順序（map順序非保証問題への対処）
- Decision 7 準拠

### Task 9: Config/CIレビュー (review-lead)
**対象ファイル:**
- `Makefile` — 全ターゲット存在確認、CGO_ENABLED=0、ldflags
- `go.mod` — 外部依存なし（stdlib only）
- `.github/workflows/ci.yml` — PR/push mainトリガー、Go version、バイナリサイズチェック
- `.goreleaser.yml` — linux/darwin × amd64/arm64
- Decision 1,10 準拠

### Task 10: Codexフルレビュー (second-opinion スキル)
- Wave 1の全結果 + テスト結果をインプット
- 全.goファイルを対象
- 重点: TLS/HTTP計測セマンティクス、レース条件、エラー分類、Go慣用表現

### Task 11: コード修正（Codex必須）
- Wave 1-2で発見された問題を重大度順にリスト化
- 修正案を作成し、**second-opinion スキルでCodexにレビュー＆承認を依頼**
- Codexが承認した修正のみ適用（Claude単独での修正は禁止）
- 修正フロー: 問題特定 → 修正案作成 → Codexレビュー → 承認 → 適用

### Task 12: テスト再実行
- 修正適用後に `make lint && make test-race && make build && make e2e` 再実行
- 全パス確認

### Task 13: 最終報告書
- テスト結果サマリ（修正前 vs 修正後）
- 発見された問題と修正内容の一覧
- 問題点を重大度別分類（critical / major / minor / nit）
- Design Decisions 12項目の準拠チェック
- Codexレビュー結果の要約
- 残存課題（あれば）

## 特に注意すべき懸念事項

1. **map順序問題**: `map[string]*LayerResult` はJSON出力順序を保証しない → レンダラーの対処確認
2. **Duration精度**: 各レイヤーのミリ秒変換の一貫性
3. **エラー分類の網羅性**: failure matrix の全コードが実装されているか
4. **コンテキストキャンセル**: レイヤー実行中のキャンセル処理
5. **HTTP リダイレクト**: `CheckRedirect` 設定で自動リダイレクト無効化確認
6. **TLS証明書閾値**: 30日未満でwarn の実装確認

## 検証方法

1. `make lint` — フォーマット・静的解析パス
2. `make test-race` — 全テストパス（レースディテクタ）
3. `make build` — ビルド成功 + バイナリ<5MB
4. `make e2e` — E2Eテストパス
5. Codexレビュー（second-opinion）— 重大な問題なし
6. 12 Design Decisions 全準拠確認
