# stackdiag v0.1 実装計画

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Use Agent Teams (TeamCreate → TaskCreate → Task with team_name). Max 2 parallel subagents.

**ゴール:** `stackdiag` — DNS→TCP→TLS→HTTPをレイヤーごとに診断するGo CLI。構造化JSON + 人間向けテーブル出力。

**アーキテクチャ:** Runnerがレイヤーを順次実行し、型付きResult構造体を生成。2つのレンダラー（JSON, table）が同じ構造体を消費。各レイヤーはインターフェース経由でテスタブル。

**技術スタック:** Go stdlib（net, crypto/tls, net/http）、コア部分に外部依存なし。フォーマッタはgofumpt。

**チーム:** Claude = Architect/Test Designer/General Implementer/UI-UX。Codex = Core Tech（TLS/HTTP）+ Code Reviewer。全設計判断はCodex合議必須。

**参照ドキュメント:**
- `docs/product-design.md` — JSONスキーマ、出力仕様、exit code定義
- `docs/development-guide.md` — ディレクトリ構造、CI/CD、ビルドフェーズ
- `docs/design-decisions.md` — Claude-Codex合意ログ（12件）
- `docs/test-strategy.md` — テストピラミッド、障害マトリクス、Acceptance戦略

---

## Phase 0: 基盤（逐次、4タスク）

### Task 1: プロジェクト雛形 + Goモジュール

**ファイル:**
- 作成: `go.mod`, `cmd/stackdiag/main.go`, `Makefile`, `.gitignore`

**手順:**
1. `go mod init github.com/muras3/stackdiag`
2. 最小限のmain.go作成（`--version`フラグのみ対応するスタブ）
3. Makefile作成（`docs/development-guide.md`のCI/CDセクション準拠）
4. .gitignore作成（`bin/`, `*.exe`, `.DS_Store`）
5. `make build && ./bin/stackdiag --version` で動作確認
6. コミット: `feat: project scaffolding with Go module, Makefile, and main stub`

---

### Task 2: コア型 + Layerインターフェース

**ファイル:**
- 作成: `internal/core/types.go`, `internal/core/types_test.go`
- 作成: `internal/core/target.go`, `internal/core/target_test.go`

**手順:**
1. Status型（ok/warn/fail/skip）のテストを書く → 失敗確認
2. `types.go`を実装（Status, ProbeError, LayerResult, Summary, Result, Layer interface, ProbeContext） → テスト通過
3. Targetパーサーのテストを書く（https/http/tcp、デフォルトポート、無効入力） → 失敗確認
4. `target.go`を実装（ParseTarget, NeedsTLS, NeedsHTTP, HostPort） → テスト通過
5. コミット: `feat: core types (Result, LayerResult, Status, ProbeError) and Target parser`

**スキーマ契約（`docs/product-design.md`準拠）:**
- `Result`: schema_version, started_at, target, layers（object）, summary
- `LayerResult`: status, duration_ms, observations, error
- `Summary`: wall_clock_ms, first_non_ok_layer, exit_code

---

### Task 3: JSONレンダラー + 契約テスト

**ファイル:**
- 作成: `internal/render/json/renderer.go`, `internal/render/json/renderer_test.go`
- 作成: `internal/render/json/contract_test.go`

**手順:**
1. Render関数のテストを書く（Result → 有効なJSON出力） → 失敗確認
2. レンダラー実装（`json.Encoder`、インデント付き、HTMLエスケープ無効） → テスト通過
3. 契約テストを書く（`docs/test-strategy.md`のContract Testsセクション準拠）
   - トップレベル必須フィールドの存在確認
   - 各レイヤーの必須フィールド確認
   - statusの取りうる値の検証
4. 全テスト実行 → 通過
5. コミット: `feat: JSON renderer with schema contract tests`

---

### Task 4: CLIスケルトン（フラグパーサー）

**ファイル:**
- 作成: `internal/cli/cli.go`, `internal/cli/cli_test.go`
- 変更: `cmd/stackdiag/main.go`

**手順:**
1. ParseArgs関数のテストを書く（基本、--json、引数なしエラー、--header複数） → 失敗確認
2. cli.go実装（flag.FlagSet使用、Config構造体） → テスト通過
3. main.goをcliパッケージ使用に更新（パース → ターゲット解析 → スタブ出力）
4. `make build && ./bin/stackdiag https://example.com` で動作確認
5. コミット: `feat: CLI skeleton with flag parsing (--json, --method, --header, --timeout, --insecure)`

**対応フラグ:** `--json`, `--method`, `--header`（複数可）, `--timeout`, `--insecure`, `--version`

---

## Phase 1: ネットワークレイヤー（Task 5完了後、最大2並列）

### Task 5: DNSレイヤー（Claude担当）

**ファイル:**
- 作成: `internal/layers/dns/dns.go`, `internal/layers/dns/dns_test.go`
- 作成: `internal/testkit/fakes.go`

**インターフェース:** `Resolver`（`LookupHost(ctx, host) ([]string, error)`）

**テストケース（`docs/test-strategy.md`障害マトリクス準拠）:**
- 成功: resolve → IP返却、status=ok、duration計測
- NXDOMAIN: status=fail、code=DNS_NXDOMAIN
- タイムアウト: status=fail、code=DNS_TIMEOUT
- SERVFAIL: status=fail、code=DNS_SERVFAIL

コミット: `feat: DNS layer with Resolver interface and failure tests`

---

### Task 6: TCPレイヤー（Claude担当）

**ファイル:**
- 作成: `internal/layers/tcp/tcp.go`, `internal/layers/tcp/tcp_test.go`

**インターフェース:** `Dialer`（`DialContext(ctx, network, address) (net.Conn, error)`）

**テストケース:**
- 成功: localhostリスナーに接続、status=ok
- 接続拒否: status=fail、code=TCP_REFUSED
- タイムアウト: status=fail、code=TCP_TIMEOUT

コミット: `feat: TCP layer with Dialer interface and failure tests`

---

### Task 7: TLSレイヤー（Codex担当）

**ファイル:**
- 作成: `internal/layers/tls/tls.go`, `internal/layers/tls/tls_test.go`
- 作成: `testdata/certs/`（fixture証明書）

**担当:** Codex（Core Tech）
**前提条件:** Task 2（コア型）凍結済み

**Codexへの提供情報:**
- 凍結済みインターフェース（core.LayerResult, ProbeError, ProbeContext）
- 期待observations: version, cipher_suite, cert_days_until_expiry, cert_hostname_match
- 期待エラーコード: TLS_CERT_EXPIRED, TLS_CERT_EXPIRING_SOON, TLS_HOSTNAME_MISMATCH, TLS_UNTRUSTED_CHAIN, TLS_HANDSHAKE_TIMEOUT
- 計測セマンティクス: duration = ハンドシェイク開始〜完了（TCP接続除く）

**テストケース:**
- 成功: ローカルTLSサーバー、有効証明書、status=ok
- 期限切れ: status=fail、code=TLS_CERT_EXPIRED
- ホスト名不一致: status=fail、code=TLS_HOSTNAME_MISMATCH
- 期限間近（<30日）: status=warn、code=TLS_CERT_EXPIRING_SOON
- insecureモード: 不正証明書でもstatus=ok

コミット: `feat: TLS layer with cert inspection and failure classification`

---

### Task 8: HTTPレイヤー（Codex担当）

**ファイル:**
- 作成: `internal/layers/http/http.go`, `internal/layers/http/http_test.go`

**担当:** Codex（Core Tech）
**前提条件:** Task 7完了

**Codexへの提供情報:**
- 凍結済みインターフェース
- 期待observations: method, protocol, status_code
- 期待エラーコード: HTTP_401, HTTP_403, HTTP_404, HTTP_429, HTTP_5XX, HTTP_502, HTTP_503, HTTP_504, HTTP_TIMEOUT
- 計測セマンティクス: duration = TTFB（リクエスト送信〜レスポンスヘッダー初バイト受信）
- DisableKeepAlives: true、自動リダイレクト無効
- --insecure、カスタムヘッダー/メソッド対応

**テストケース:**
- 成功: httptest 200、status=ok
- 4xx/5xx: 正しいエラーコード分類
- タイムアウト: status=fail、code=HTTP_TIMEOUT
- 接続リセット: hijack + close、status=fail

コミット: `feat: HTTP layer with TTFB measurement and status classification`

---

## Phase 2: 統合（2レイヤー以上完成後）

### Task 9: Runnerオーケストレーション

**ファイル:**
- 作成: `internal/runner/runner.go`, `internal/runner/runner_test.go`

**主要動作:**
- レイヤー順次実行（dns → tcp → tls → http）
- スキームに基づくスキップ（tcp:// → tls/httpスキップ）
- fail時停止（後続=skip）、warn時続行
- DNS解決IP → TCP渡し、タイムアウトバジェット伝播
- wall_clock_ms収集、first_non_ok_layer判定

**テストケース（全てfake使用、`docs/test-strategy.md`部分障害シナリオ準拠）:**
- 全成功: exit 0
- DNS失敗: dns=fail、残り=skip
- TCP失敗: dns=ok、tcp=fail、残り=skip
- TLS warn + HTTP fail: first_non_ok_layer=tls
- TCPのみターゲット: dns+tcpのみ
- タイムアウトバジェット枯渇: 後続skip

コミット: `feat: runner with sequential layer execution and skip/fail logic`

---

### Task 10: 終了コードマッピング

**ファイル:**
- 作成: `internal/exitcode/exitcode.go`, `internal/exitcode/exitcode_test.go`

**マッピング（`docs/product-design.md`準拠）:**
- 0=ok, 1=ツールエラー, 2=warn, 10=dns, 20=tcp, 30=tls, 40=http

**テスト:** テーブル駆動、各カテゴリに1件。

コミット: `feat: exit code mapping from Result to process exit status`

---

### Task 11: テーブルレンダラー

**ファイル:**
- 作成: `internal/render/table/renderer.go`, `internal/render/table/renderer_test.go`
- 作成: `testdata/golden/`（success, tls_warn_http_fail, dns_fail）

**出力仕様（`docs/design-decisions.md` Decision 7準拠）:**
- 色: 緑✓ / 黄⚠ / 赤✗（ANSIエスケープ）
- NO_COLOR / 非TTY → ASCIIフォールバック
- タイミング列右揃え
- ゴールデンファイルテスト（タイミング値正規化）

コミット: `feat: table renderer with color, alignment, and NO_COLOR fallback`

---

### Task 12: main.goの結合

**ファイル:**
- 変更: `cmd/stackdiag/main.go`

**結合:** CLI → Targetパーサー → Runner（実レイヤー） → レンダラー → 終了コード
**分離:** stdout = データ、stderr = エラー/ログ

コミット: `feat: wire CLI, runner, layers, and renderers in main`

---

## Phase 3: E2E + CI

### Task 13: E2Eテスト

**ファイル:**
- 作成: `test/e2e/probe_test.go`

**テスト（ビルド済みバイナリをexec.Commandで実行）:**
- `stackdiag https://example.com --json` → 有効JSON、exit 0
- `stackdiag tcp://localhost:閉じポート` → exit 20
- `stackdiag`（引数なし） → exit 1、stderrにusage
- `stackdiag --version` → バージョン文字列

コミット: `feat: E2E tests against built binary`

---

### Task 14: GitHub Actions CI

**ファイル:**
- 作成: `.github/workflows/ci.yml`

**構成:** lint → test-race → build → e2e（Go 1.24.x, ubuntu-latest）
**トリガー:** push main, pull_request

コミット: `ci: GitHub Actions pipeline with lint, test, build, e2e`

---

### Task 15: Codex全体コードレビュー

**担当:** Codex（Code Reviewer）
コードベース全体に`codex review`実行。発見された問題を全て修正。

コミット: `fix: address code review findings`

---

### Task 16: goreleaserセットアップ

**ファイル:**
- 作成: `.goreleaser.yml`

クロスプラットフォーム: linux/darwin × amd64/arm64。タグトリガーリリース。

コミット: `ci: goreleaser config for cross-platform releases`

---

## Agent Teams設定

```
チーム: stackdiag-dev

ロール → エージェント割り当て:
- architect:     Claude（メインコンテキスト）
- implementer-1: Claudeサブエージェント
- implementer-2: Claudeサブエージェント
- implementer-3: Claudeサブエージェント（Phase 2以降、パターン安定後に投入）
- codex-impl:    Codex via codex exec（Task 7-8、Core Tech）
- reviewer:      Codex via codex review（Task 15 + タスクごとのレビュー）
```

### 並列化戦略:

Phase 0はTask間の依存が強く逐次。Phase 1以降で段階的に並列数を増やす。

- **Phase 0:** 逐次（Task 1→2→3→4）— 各タスクが前タスクの成果物に依存
- **Phase 1:** Task 5を逐次 → Task 6 + Task 7 を並列（2並列）→ Task 8
- **Phase 2:** **Task 9 + Task 10 + Task 11 を並列（3並列）** → Task 12
  - Runner/ExitCode/Tableは互いに独立。パターン安定済みのため3並列可。
- **Phase 3:** **Task 13 + Task 14 + Task 16 を並列（3並列）** → Task 15
  - E2E/CI/goreleaserは互いに独立。全体レビューは最後。

### タスクごとのレビュー手順:
各タスク完了後、`codex review`を以下と共に実行:
- git diff
- テスト出力
- 変更意図の要約（1-3行）

---

## 検証

全タスク完了後:
1. `make lint` → クリーン
2. `make test-race` → 全パス
3. `make build` → バイナリ5MB以下
4. `./bin/stackdiag https://example.com` → ✓/⚠/✗付きテーブル出力
5. `./bin/stackdiag https://example.com --json` → スキーマ準拠の有効なJSON
6. `./bin/stackdiag tcp://localhost:1` → exit code 20（TCP拒否）
7. `./bin/stackdiag --version` → バージョン文字列
8. `make e2e` → 全パス
