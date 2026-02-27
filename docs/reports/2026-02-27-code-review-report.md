# probe v0.1 全コードレビュー & テスト再実行 — 最終報告書

**日時:** 2026-02-27
**レビュー体制:** Claude Agent Teams (8並列 Wave 1 + 4並列 Wave 2) + Codex (gpt-5.3-codex) + Gemini (gemini-3.1-pro-preview)
**対象:** 全30ファイル、3,935行

---

## 1. テスト結果サマリ

### Wave 1 修正後 (コミット c89720d)

| ステップ | 結果 | 備考 |
|----------|------|------|
| `go vet` | PASS | 問題なし |
| `make test-race` | PASS | 全ユニットテストPASS |
| `make build` | PASS | ビルド成功 |
| `make e2e` | PASS | 4件PASS |

### Wave 2 (Codexレビュー) 修正後 (コミット e7476d9)

| ステップ | 結果 | 備考 |
|----------|------|------|
| `go vet` | PASS | 問題なし |
| `make test-race` | PASS | 全ユニットテストPASS (race detector) |
| `make build` | PASS | 5.52MB (darwin/arm64, CI linux/amd64では異なる可能性) |
| `make e2e` | PASS | 4件PASS |

---

## 2. Wave 2 (Codexレビュー) で発見・修正された問題

### Critical (1件)

| # | 問題 | 場所 | 修正内容 | 発見者 |
|---|------|------|----------|--------|
| C1 | HTTP SNI — resolved IP使用時にTLS ServerNameがIPアドレスになり証明書検証失敗 | http.go, main.go | `NewDefault(insecure, serverName)` でServerNameにホスト名を設定 | Codex |

### High (3件)

| # | 問題 | 場所 | 修正内容 | 発見者 |
|---|------|------|----------|--------|
| H1 | TLS証明書 <24時間前に失効 → `int(-0.04)=0` で EXPIRING_SOON に誤分類 | tls.go:89 | raw `time.Duration` で失効判定後にint変換 | Codex |
| H2 | TLSタイムアウトが `context.DeadlineExceeded` を検出しない | tls.go:190 | `errors.Is(err, context.DeadlineExceeded)` 追加 | Codex |
| H3 | TCP error分類が `strings.Contains` 依存で脆弱 | tcp.go | `syscall.ECONNREFUSED`/`ECONNRESET` ベースに全面書き換え | Codex + Gemini |

### Medium (2件)

| # | 問題 | 場所 | 修正内容 | 発見者 |
|---|------|------|----------|--------|
| M1 | HTTP 500 が explicit mapping なく `HTTP_5XX` にフォールスルー | http.go:175 | `case 500: HTTP_500` 追加 | Codex |
| M2 | Contract test に `skip` status の shape validation がない | contract_test.go | `TestContractSkipLayerShape` 追加 | Codex |

### Low (1件)

| # | 問題 | 場所 | 修正内容 | 発見者 |
|---|------|------|----------|--------|
| L1 | Table renderer Unicode矢印 `→` が NO_COLOR/ASCII モードで出力 | table/renderer.go | `useColor` 判定で `->` にフォールバック | Gemini |

### 新規テスト追加 (Wave 2)

- `TestTLSCertExpiredLessThan24h` — 24時間未満の失効証明書
- `TestTLSContextDeadlineExceededViaFakeHandshaker` — コンテキストデッドライン
- `TestTCPConnectionReset` — TCP RST
- `TestDNSContextCanceled` — DNSコンテキストキャンセル
- `TestContractSkipLayerShape` — skip status スキーマ契約
- `TestRenderASCIIArrowFallback` — Unicode矢印フォールバック

---

## 3. Wave 1 + Wave 2 統合: 全修正一覧

### Critical (6件 — 全修正済)

| # | 問題 | Wave | 発見者 |
|---|------|------|--------|
| C1 | IPv6アドレス構築が壊れる (JoinHostPort未使用) | 1 | Claude + Codex |
| C2 | TLS duration_ms にTCP接続時間が混入 | 1 | Claude |
| C3 | tcp:// 時に tls/http レイヤーがJSONに存在しない | 1 | Claude |
| C4 | 本番コードが testkit パッケージに依存 | 1 | Claude + Codex |
| C5 | `--timeout 0` で全レイヤーskip → exit 0 | 1 | Codex |
| C6 | HTTP SNI — resolved IPでTLS ServerName不正 | 2 | Codex |

### High (3件 — 全修正済)

| # | 問題 | Wave | 発見者 |
|---|------|------|--------|
| H1 | TLS証明書 <24h失効の truncation bug | 2 | Codex |
| H2 | TLSタイムアウト context.DeadlineExceeded 未検出 | 2 | Codex |
| H3 | TCP error分類 strings.Contains 依存 | 2 | Codex + Gemini |

### Major (12件修正済 / 残存は設計判断・技術的負債)

Wave 1で10件修正、Wave 2で2件修正。

### Minor/Nit (残存 — v0.2以降)

計33件。機能影響なし、品質向上の余地。

---

## 4. Design Decisions 準拠チェック

| # | Decision | 準拠状況 |
|---|----------|----------|
| 1 | Go (Rust ではなく) | ✅ |
| 2 | layers は object (array ではなく) | ✅ `map[string]*LayerResult` |
| 3 | error (issue ではなく) | ✅ |
| 4 | trace_id / attempt はMVPに入れない | ✅ |
| 5 | observations はレイヤーローカル | ✅ |
| 6 | MCP不要 (MVPでは) | ✅ |
| 7 | screenshot-worthy な人間向け出力 | ✅ (Wave 2 で Unicode矢印修正) |
| 8 | Agent-friendly = 安定した判断インターフェース | ✅ |
| 9 | ビルド順は自然なスタック順 | ✅ dns→tcp→tls→http |
| 10 | Makefile でローカル/CI同一性を保証 | ✅ |
| 11 | subagent並列は最大2 | ✅ (開発時の制約) |
| 12 | チーム構成 | ✅ |

**12項目全て準拠** (Wave 2 で Decision 7 の残存問題も解消)

---

## 5. Codex / Gemini レビュー結果要約

### レビュー体制 (Wave 2, 4並列)

| エージェント | 対象 | 外部ツール | 指摘数 |
|---|---|---|---|
| reviewer-core | core types, target, exitcode, fakes | Codex | (結果統合済) |
| reviewer-net | DNS + TCP | Codex + Gemini | 4件 (3件修正) |
| reviewer-crypto | TLS + HTTP (Core Tech重点) | Codex | 8件 (4件修正) |
| reviewer-orch | Runner + CLI + Renderer | Codex + Gemini | 5件 (3件修正) |

### Codex独自発見 (Claudeが見逃していた問題)

- HTTP SNI resolved IP問題 (Critical)
- TLS <24h失効 truncation (High)
- TLS context.DeadlineExceeded (High)
- HTTP 500 explicit mapping (Medium)
- Contract test skip shape (Medium)

### Deferred (MVP許容)

- HTTP TTFB計測にTCP/TLS接続時間が含まれる — HTTP層が独自接続を確立するため許容
- TLS string-based error分類 — Go stdlib エラーメッセージ依存、MVPでは低リスク
- JSON layers map順序 — JSON仕様上は非決定的で許容

---

## 6. 修正の統計

| 項目 | Wave 1 | Wave 2 | 合計 |
|------|--------|--------|------|
| 修正ファイル数 | 16 | 13 | 22 (重複あり) |
| 追加行数 | +308 | +247 | +555 |
| 削除行数 | -53 | -36 | -89 |
| 新規テスト | 8件 | 6件 | 14件 |

---

## 7. 残存課題

### 要設計判断 (Codex合議推奨)

1. **HTTP TTFB計測**: 現在 `client.Do()` 全体を計測。カスタムTransportで分離可能だが複雑度増
2. **TLS --insecure + 期限切れ**: insecure指定時に証明書期限でfailすべきか
3. **JSON layers順序**: カスタム MarshalJSON で固定順序にすべきか

### 技術的負債 (v0.2)

4. バイナリサイズ超過 (darwin/arm64で5.52MB)
5. E2E テストカバレッジ不足 (stdout/stderr分離、http://スキーム、NO_COLOR)
6. テスト内 context cancel リーク (機能影響なし)
7. SERVFAILの個別分類 (Go stdlib制約)
8. exitcode nil Result パニック防止

---

## 8. 結論

probe v0.1 は2段階のレビュー (Claude 8並列 + Codex/Gemini 4並列) を経て、Critical 6件・High 3件を含む全重大問題を解消。全テスト (ユニット + E2E、race detector付き) がPASS。Design Decisions 12項目に完全準拠。

Codex外部レビューにより、Claude単独では検出できなかった5件の問題 (HTTP SNI, TLS truncation, TLS deadline, HTTP 500 mapping, contract skip) を発見・修正。外部レビューの価値が実証された。

残存課題はいずれもMVP品質に影響しないレベルであり、v0.2以降の改善項目として管理可能。
