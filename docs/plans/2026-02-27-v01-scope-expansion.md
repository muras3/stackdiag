# v0.1 Scope Expansion Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** MinVersion TLS1.2明示、認証ヘルパー（--bearer-env, --basic-env）、--tls-scan、--count N + 統計を全てv0.1に実装し、全出力パターンがデザインドキュメント（`docs/product-design.md`）と完全に一致することを検証する。

**Architecture:** 既存のRunner→Layer→Renderer構造を維持。コードは一切事前に書かず、全てCodexレビュー後に確定する。

**Tech Stack:** Go stdlib（crypto/tls, encoding/base64, math, sort）。外部依存なし。

**参照ドキュメント:**
- `docs/product-design.md` — JSONスキーマ、exit code、status values、全仕様
- `docs/design-decisions.md` — Decision 13: v0.1スコープ拡張の合意事項

---

## 終了条件

**以下の全てが満たされるまで完了しない：**

1. `make lint && make test-race && make e2e` が全てPASS
2. Codexが全変更のコードレビューを完了し、指摘事項ゼロ
3. セキュリティレビュー完了（認証情報漏洩なし）
4. 出力検証マトリクスV1-V32の全パターンをバイナリで実行し、`docs/product-design.md` の仕様と一致を確認

### 出力検証マトリクス（V1-V32）

| # | コマンド | 検証項目 |
|---|---------|---------|
| V1 | `stackdiag https://example.com` | table出力、色付き、全レイヤーok |
| V2 | `stackdiag --json https://example.com` | JSON構造がproduct-design.mdのサンプルと一致 |
| V3 | `stackdiag http://example.com` | tls=skip, http層あり |
| V4 | `stackdiag tcp://example.com:443` | dns+tcpのみ、tls/http=skip |
| V5 | `stackdiag example.com` | ベアホスト名（https://推論） |
| V6 | `NO_COLOR=1 stackdiag https://example.com` | ASCIIフォールバック [ok]/[FAIL] |
| V7 | `stackdiag --bearer-env TOK https://httpbin.org/get` | Bearerトークン送信、table出力 |
| V8 | `stackdiag --bearer-env TOK --json https://httpbin.org/get` | JSON: Authorization=[REDACTED] |
| V9 | `stackdiag --basic-env CRED https://httpbin.org/get` | Basic認証送信（Base64） |
| V10 | `stackdiag --basic-env CRED --json https://httpbin.org/get` | JSON: Authorization=[REDACTED] |
| V11 | `stackdiag --bearer-env TOK --no-redact --json https://httpbin.org/get` | 生トークン表示 |
| V12 | `stackdiag --bearer-env NONEXISTENT https://example.com` | stderr: 未設定エラー、exit 1 |
| V13 | `stackdiag --bearer-env EMPTY https://example.com` (EMPTY="") | stderr: 空エラー、exit 1 |
| V14 | `stackdiag --bearer-env TOK --basic-env CRED https://example.com` | stderr: 競合エラー、exit 1 |
| V15 | `stackdiag --bearer-env TOK --header "Authorization: x" https://example.com` | stderr: 競合エラー、exit 1 |
| V16 | `stackdiag --basic-env BAD https://example.com` (BAD="nocolon") | stderr: フォーマットエラー、exit 1 |
| V17 | `stackdiag --tls-scan https://example.com` | table: scan行追加 |
| V18 | `stackdiag --tls-scan --json https://example.com` | observations.tls_scan構造一致 |
| V19 | `stackdiag --json https://example.com` (tls-scan未指定) | tls_scanフィールド非存在 |
| V20 | `stackdiag --tls-scan http://example.com` | tls=skipでscan不実行 |
| V21 | `stackdiag --count 3 https://example.com` | table: attempt + statistics |
| V22 | `stackdiag --count 3 --json https://example.com` | JSON: --count時サンプル一致 |
| V23 | `stackdiag --count 5 tcp://localhost:1` | 全fail、loss_ratio=1.0 |
| V24 | `stackdiag --count 5 --json tcp://localhost:1` | p50/p95=null(全fail時) |
| V25 | `stackdiag --json https://example.com` (count未指定) | count/attempts/statistics非存在 |
| V26 | `stackdiag --count 3 --tls-scan --json https://example.com` | 各attemptにtls_scan |
| V27 | `stackdiag --count 3 --bearer-env TOK --json https://httpbin.org/get` | 認証+反復 |
| V28 | `stackdiag --count 3 --tls-scan --bearer-env TOK --json https://httpbin.org/get` | 全機能組み合わせ |
| V29 | `stackdiag --count 0 https://example.com` | エラー: exit 1 |
| V30 | `stackdiag --count -1 https://example.com` | エラー: exit 1 |
| V31 | `stackdiag --help` | AUTHENTICATION/DIAGNOSTICSセクション |
| V32 | `stackdiag --version` | 既存動作維持 |

---

## 実行戦略: Scaffold → Worktree並列 → Merge → Review並列 → 検証

### なぜこの構造か

共有ファイルの競合ゾーン分析:
- `cli.go` Config構造体（L16-26）、ParseArgs（L75-175）、needsValue（L177-184）、HelpText（L28-63）→ **4機能全てが触る**
- `main.go` ProbeContext生成（L66-73）、Runner.Run呼び出し（L77）、render分岐（L80-91）→ **3機能が触る**
- `types.go` ProbeContext（L142-151）→ **2機能が触る**

**結論:** 共有ファイルを先にscaffoldすれば、各機能の実装は独立ファイルだけで完結する。worktreeで隔離すれば競合ゼロ。

### Agent Teams構成

```
TeamCreate: "v01-expansion"

リーダー（自分）: 全体統括、T0実行、マージ、最終検証
├── impl-minversion:   T1 MinVersion（worktree隔離）
├── impl-auth:         T2 Auth全スタック（worktree隔離）
├── impl-tls-scan:     T3 TLS scan全スタック（worktree隔離）
├── impl-count:        T4 Count全スタック（worktree隔離）
├── impl-renderers:    T5 全renderer拡張（worktree隔離）
├── security-reviewer: T7 セキュリティレビュー
├── ux-reviewer:       T8 UX/UIレビュー
├── code-reviewer:     T9 Codex最終レビュー
└── verifier:          T10 出力検証V1-V32
```

### タイムライン（依存DAG）

```
T0 Scaffold [リーダー直接実行、10分]
 │
 ├─→ T1 MinVersion [worktree] ──────────────────────────┐
 ├─→ T2 Auth全スタック [worktree] ──────────────────────┤
 ├─→ T3 TLS scan全スタック [worktree, T1完了待ち] ──────┤
 ├─→ T4 Count型+runner+exitcode [worktree] ────────────┤
 │                                                       │
 │   T4完了後即座に:                                      │
 ├─→ T5 Renderer拡張 [worktree, T3+T4完了待ち] ────────┤
 │                                                       │
 T6 Merge [リーダー: 全worktreeをmainにマージ] ←────────┘
 │
 ├─→ T7 Security review ──┐
 ├─→ T8 UX review ────────┼─→ 指摘修正 → T10 出力検証V1-V32
 └─→ T9 Codex review ─────┘

同時稼働ピーク: T1+T2+T4 = 3 → T2+T3+T4 = 3 → T3+T5 = 2 → T7+T8+T9 = 3
```

**T1は最速で終わる（2ファイル変更のみ）ため、T3はすぐ開始できる。**
**T4も独立ファイルのみのため、T1/T2と完全並列。**

---

## T0: Scaffold（リーダー直接実行）

**目的:** 全共有ファイルにフィールド/フラグ/スタブを一括追加し、後続の全実装タスクをunblockする。

**Files:**
- Modify: `internal/cli/cli.go`
  - Config構造体に4フィールド追加: BearerEnv, BasicEnv, TLSScan, Count
  - ParseArgsに4フラグ登録 + needsValueに追加
  - 競合検出ロジック（--bearer-env vs --basic-env vs --header Authorization）
  - HelpTextにAUTHENTICATION/DIAGNOSTICSセクション追加
  - --count バリデーション（>= 1）
- Modify: `internal/cli/cli_test.go`
  - 全フラグのパーステスト
  - 競合検出テスト（3パターン: bearer+basic, bearer+header, basic+header）
  - --count バリデーションテスト（0, -1, abc）
  - HelpTextセクション存在テスト
- Modify: `internal/core/types.go`
  - ProbeContextにTLSScanフィールド追加
- Modify: `cmd/stackdiag/main.go`
  - pctx.TLSScan = cfg.TLSScan 設定
  - count分岐のスタブ（TODO: T4/T5で実装）
  - auth注入のスタブ（TODO: T2で実装）

**テスト仕様:**
- --bearer-env VAR → Config.BearerEnv パース正常
- --basic-env VAR → Config.BasicEnv パース正常
- --tls-scan → Config.TLSScan = true
- --count N → Config.Count = N
- --count 0, -1, abc → エラー
- --bearer-env + --header Authorization（case-insensitive）→ 競合エラー
- --basic-env + --header authorization → 競合エラー
- --bearer-env + --basic-env → 競合エラー
- HelpTextにAUTHENTICATION/DIAGNOSTICSセクション存在

**手順:** TDD → `make test-race` → Codex review → commit
**コミット:** `feat: scaffold CLI flags, types, and main.go stubs for v0.1 features`

---

## T1: MinVersion TLS1.2（worktree隔離）

**依存:** T0
**リスク:** 低
**担当:** impl-minversion

**Files（独立、競合なし）:**
- Modify: `internal/layers/tls/tls.go` — tls.ConfigにMinVersion: tls.VersionTLS12
- Modify: `internal/layers/tls/tls_test.go` — configCaptorパターンでMinVersion検証
- Modify: `internal/layers/http/http.go` — NewDefaultのtls.ConfigにMinVersion追加
- Modify: `internal/layers/http/http_test.go` — Transport.TLSClientConfig.MinVersion検証

**テスト仕様:**
- TLS層: captorでMinVersion == tls.VersionTLS12
- TLS層: --insecure時もMinVersion維持
- HTTP層: NewDefaultのTransport.TLSClientConfig.MinVersion == tls.VersionTLS12
- HTTP層: insecure=true時もMinVersion維持

**手順:** TDD → `make test-race` → Codex review → commit
**コミット:** `security: enforce MinVersion TLS1.2 in TLS and HTTP layers`

---

## T2: Auth全スタック（worktree隔離）

**依存:** T0
**リスク:** 中（セキュリティ重点）
**担当:** impl-auth

**Files（T0でスタブ済みの箇所を実装）:**
- Modify: `cmd/stackdiag/main.go` — auth注入スタブを実装に置換
- Modify: `test/e2e/stackdiag_test.go` — E2Eテスト追加

**テスト仕様:**
- --bearer-env正常: ローカルHTTPサーバでAuthorization = "Bearer <value>" 検証
- --basic-env正常: Authorization = "Basic <base64(user:pass)>" 検証
- 環境変数未設定 → stderr エラー + exit 1、stderrに値なし
- 環境変数空 → stderr エラー + exit 1
- 環境変数空白のみ → stderr エラー + exit 1
- --basic-env コロンなし → stderr エラー + exit 1、stderrに値なし
- --redact時JSON: Authorization=[REDACTED]
- --no-redact時JSON: 生値表示
- セキュリティ: エラーメッセージに認証情報の値が含まれない

**手順:** TDD → `make test-race && make e2e` → Codex review（セキュリティ重点）→ commit
**コミット:** `feat: implement auth header injection from environment variables`

---

## T3: TLS scan全スタック（worktree隔離）

**依存:** T0, T1（MinVersion完了後）
**リスク:** 中〜高
**担当:** impl-tls-scan

**Files（独立、競合なし）:**
- Modify: `internal/layers/tls/tls.go` — Probe()にscanロジック追加
- Modify: `internal/layers/tls/tls_test.go` — fakeHandshakerで各version応答制御

**テスト仕様:**
- --tls-scan時: TLS 1.0/1.1/1.2/1.3で個別試行（MinVersion=MaxVersion=対象version固定）
- fakeHandshaker制御: 1.0=supported, 1.1=error, 1.2=supported, 1.3=supported
- observations["tls_scan"]構造がproduct-design.mdと完全一致
  - attempts配列（version, supported, duration_ms, error）
  - supported_versions配列
  - deprecated_versions_enabled配列
- deprecated検出（1.0 or 1.1 supported）&& 通常ok → status=warn, error.code=TLS_DEPRECATED_VERSION_ENABLED
- 通常probe fail → failが優先、scanはobservationsに入るがstatusはfailのまま
- --tls-scan未指定 → observations["tls_scan"]非存在
- scan中の接続失敗がProbe全体をfailにしない
- scan用tls.Configが通常接続のMinVersion設定を弱めないこと

**手順:** TDD → `make test-race` → Codex review → commit
**コミット:** `feat: implement --tls-scan TLS version probing`

---

## T4: Count型 + Runner.RunCount + exitcode（worktree隔離）

**依存:** T0
**リスク:** 高
**担当:** impl-count

**Files（独立、競合なし）:**
- Modify: `internal/core/types.go` — AttemptResult, LayerStatistics, CountResult型追加、カスタムMarshalJSON
- Modify: `internal/core/types_test.go` — JSON構造検証
- Modify: `internal/runner/runner.go` — RunOnce()リファクタ + RunCount()追加
- Modify: `internal/runner/runner_test.go` — fakeLayerでRunCountテスト
- Modify: `internal/exitcode/exitcode.go` — WorstExitCode()追加
- Modify: `internal/exitcode/exitcode_test.go` — WorstExitCodeテスト

**テスト仕様:**

型定義:
- CountResult JSON出力がproduct-design.mdの`--count N`時サンプルと完全一致
- AttemptResult.LayersのJSON順序: dns→tcp→tls→http固定
- LayerStatistics: p50_ms/p95_ms成功試行のみ、全fail→null（omitempty）
- loss_ratio = fail_count / sample_count、skip層はsample_count除外

statistics計算:
- percentile: 境界値テスト（1要素、2要素、100要素）
- loss_ratio: 0/0=0.0, 4/5=0.2, 5/5=1.0

Runner.RunCount:
- RunCount(3, factory): 3回実行、attempts長=3
- 各attemptで独立ProbeContext（factory経由）
- 1回目fail → 2回目も実行（中断しない）
- attempt番号1始まり

WorstExitCode:
- [0, 20, 10] → 20
- [0, 0, 0] → 0
- [2, 10] → 10

リファクタ:
- 既存Run()をRunOnce()にリネーム、Run()はRunOnce()ラッパー → 既存テスト回帰なし

**手順:**
1. Run→RunOnceリファクタ → 既存テスト回帰なし確認
2. CountResult型 TDD → commit
3. statistics計算 TDD → commit
4. RunCount TDD → commit
5. WorstExitCode TDD → commit
6. `make test-race` → Codex review

**コミット群:**
- `refactor: rename Run to RunOnce, add Run wrapper`
- `feat: add CountResult, LayerStatistics types`
- `feat: add percentile and statistics calculation`
- `feat: add Runner.RunCount for repeated measurements`
- `feat: add WorstExitCode for --count aggregation`

---

## T5: Renderer拡張（worktree隔離）

**依存:** T3（tls_scan observations構造）、T4（CountResult型）
**リスク:** 中
**担当:** impl-renderers

**Files:**
- Modify: `internal/render/json/renderer.go` — CountResult出力関数追加
- Modify: `internal/render/json/contract_test.go` — CountResult JSON contract test
- Modify: `internal/render/table/renderer.go` — tls-scan行 + count表示
- Modify: `internal/render/table/renderer_test.go` — 表示テスト

**テスト仕様:**

JSON renderer:
- CountResult出力がインデント付き、HTMLエスケープなし
- layers順序保証、statistics順序保証

Table tls-scan:
- tls_scanあり: `scan: TLSv1.0 ⚠ | TLSv1.1 ✗ | TLSv1.2 ✓ | TLSv1.3 ✓` 形式
- tls_scanなし: scan行非表示（既存維持）
- NO_COLOR時ASCIIフォールバック

Table count:
- 各attempt概要表示
- Statistics表（p50/p95/loss per layer）
- NO_COLOR時ASCIIフォールバック
- アラインメント崩れなし

**手順:** TDD → `make test-race` → Codex review → commit
**コミット:** `feat: add JSON and table rendering for --tls-scan and --count`

---

## T6: Merge + main.go統合（リーダー直接実行）

**依存:** T1-T5全完了

**作業:**
1. 全worktreeブランチをmainにマージ（コンフリクト解決）
2. main.goのcount分岐スタブを実装に置換:
   - cfg.Count > 0 → Runner.RunCount使用 + CountResult render
   - else → Runner.Run使用（既存）
3. E2Eテスト追加: 全機能組み合わせ（--count + --tls-scan + --bearer-env）
4. `make lint && make test-race && make e2e` 全PASS確認
5. Codex review → commit

**コミット:** `feat: integrate all v0.1 features in main.go`

---

## T7: セキュリティレビュー

**依存:** T6（マージ完了後）
**担当:** security-reviewer

**チェック項目:**
- [ ] 認証情報がstdoutに出力されない（--redactデフォルト時）
- [ ] 認証情報がstderrエラーメッセージに含まれない
- [ ] 認証情報がJSON error.messageに含まれない
- [ ] MinVersion TLS1.2が全経路（TLS Probe, HTTP NewDefault）で有効
- [ ] --tls-scanが通常接続のMinVersionを弱めない
- [ ] 環境変数の未設定/空/空白のみが全拒否
- [ ] --basic-envのBase64エンコード正確性
- [ ] --count NでProbeContextが各attemptで独立
- [ ] 大きなNでのメモリ使用量妥当性

---

## T8: UX/UIレビュー

**依存:** T6（マージ完了後）
**担当:** ux-reviewer

**チェック項目:**
- [ ] --helpセクション構成（USAGE/TARGETS/OPTIONS/AUTHENTICATION/DIAGNOSTICS/EXIT CODES/EXAMPLES）
- [ ] table出力アラインメント（全パターン目視）
- [ ] --tls-scan table表示の直感性
- [ ] --count table表示の情報量バランス
- [ ] NO_COLOR / non-TTY ASCIIフォールバック全動作
- [ ] エラーメッセージの一貫性

---

## T9: Codex最終統合レビュー

**依存:** T6（マージ完了後）
**担当:** code-reviewer

**レビュー範囲:** `git diff main...HEAD` 全変更

---

## T10: 出力検証マトリクス全実行

**依存:** T7+T8+T9完了 + 指摘修正完了
**担当:** verifier

**手順:**
1. `make build`
2. 環境変数: `export TOK="test-token" CRED="user:pass" EMPTY=""`
3. V1-V32を順次実行、product-design.mdと照合
4. 不一致 → 修正 → 再検証
5. 全PASS → 完了

---

## 各コミット共通ルール

1. テストを先に書く（TDD）
2. `make test-race` PASS
3. `make lint` PASS
4. Codex code review完了、指摘事項ゼロ
5. コミットメッセージは変更意図を示す

## 最終完了条件

1. `make lint && make test-race && make e2e` 全PASS
2. T7セキュリティレビュー完了
3. T8 UXレビュー完了
4. T9 Codex最終レビュー完了
5. **T10 出力検証マトリクスV1-V32全PASS**
