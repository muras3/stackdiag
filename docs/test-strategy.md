# probe — Test Strategy

## 設計思想

probeは障害を診断するツール。**障害テストが成功テストより重要。**

品質担保の3本柱：
1. **テストピラミッド** — Smallが大多数、Largeは少数
2. **障害マトリクス** — レイヤー×障害パターンの網羅的カバレッジ
3. **リファレンスツールとのクロス検証** — dig/openssl/curlとの突き合わせ

## Test Pyramid（Google方式）

```
        ╱╲
       ╱ L ╲      Large: ビルド済バイナリ + Docker canary
      ╱──────╲     5-10件
     ╱   M    ╲   Medium: httptest, localhost listener, TLS fixture
    ╱──────────╲   レイヤーあたり5-10件
   ╱     S      ╲ Small: パーサー、分類、レンダラー、exit code
  ╱──────────────╲ テストの大多数
```

## テスト種別一覧

| 種別 | 目的 | 導入時期 |
|------|------|---------|
| Unit (Small) | 純粋ロジック検証。fake注入 | Phase 0〜 |
| Integration (Medium) | localhost I/O。httptest, net.Listen | Phase 1〜 |
| E2E (Large) | ビルド済バイナリのstdout/stderr/exit code | Phase 3 |
| Contract | JSONスキーマの後方互換性検証 | Phase 0〜 |
| Failure-matrix | レイヤー×障害パターン網羅 | Phase 1〜 |
| Fuzz | URL/引数パーサーのpanic/hang検出 | Phase 0〜 |
| Golden file | レンダラー出力のスナップショット比較 | Phase 0〜 |
| Race | `go test -race` で並行性バグ検出 | Phase 2〜 |
| Benchmark | ホットパスのalloc/latency追跡 | Phase 2〜 |
| Regression | バグ修正ごとのreproducerテスト | 継続 |
| Smoke | リリースバイナリの最低限動作確認 | Phase 3 |
| Acceptance | 実環境クロス検証（dig/openssl/curl） | Phase 3 |

## Faking Strategy

各レイヤーはインターフェース経由で依存注入し、テスト時はfakeに差し替える。

| レイヤー | Interface | Fake方式 |
|---------|-----------|---------|
| DNS | `Resolver` | fake: 固定IP返却 / エラー返却 / context deadline待ち |
| TCP | `Dialer` | fake: 即成功 / localhost `net.Listen` / 即エラー |
| TLS | `TLSHandshaker` | fake: 固定結果 / local TLS server with fixture certs |
| HTTP | `HTTPDoer` | fake: 固定レスポンス / `httptest.Server` |
| Time | `Clock` | fake: 固定時刻（タイミングテストのflake防止） |

## 障害マトリクス（Failure-Matrix Testing）

Netflix chaos engineeringの応用。probeでは「決定的障害注入」として実装。

### レイヤー別障害パターン

#### DNS

| パターン | テスト方法 | 期待結果 |
|---------|-----------|---------|
| NXDOMAIN | fake resolver: エラーコード返却 | `dns.status=fail`, `error.code=DNS_NXDOMAIN` |
| Timeout | fake resolver: context deadline待ち | `dns.status=fail`, `error.code=DNS_TIMEOUT` |
| SERVFAIL | fake resolver: SERVFAILエラー | `dns.status=fail`, `error.code=DNS_SERVFAIL` |

#### TCP

| パターン | テスト方法 | 期待結果 |
|---------|-----------|---------|
| Connection refused | localhost閉じポートへconnect | `tcp.status=fail`, `error.code=TCP_REFUSED` |
| Timeout | fake dialer: deadline待ち | `tcp.status=fail`, `error.code=TCP_TIMEOUT` |
| Reset | listener accept→即RST送信 | `tcp.status=fail`, `error.code=TCP_RESET` |

#### TLS

| パターン | テスト方法 | 期待結果 |
|---------|-----------|---------|
| Expired cert | fixture証明書（期限切れ） | `tls.status=fail`, `error.code=TLS_CERT_EXPIRED` |
| Hostname mismatch | SAN不一致のfixture | `tls.status=fail`, `error.code=TLS_HOSTNAME_MISMATCH` |
| Untrusted CA | unknown CAの自己署名 | `tls.status=fail`, `error.code=TLS_UNTRUSTED_CHAIN` |
| Cert expiring soon | 残り30日未満のfixture | `tls.status=warn`, `error.code=TLS_CERT_EXPIRING_SOON` |

#### HTTP

| パターン | テスト方法 | 期待結果 |
|---------|-----------|---------|
| 4xx (401, 403, 404) | httptest handler | `http.status=fail`, `error.code=HTTP_4XX` |
| 5xx (500, 502, 503) | httptest handler | `http.status=fail`, `error.code=HTTP_5XX` |
| Timeout | handler内sleep超過 | `http.status=fail`, `error.code=HTTP_TIMEOUT` |
| Reset mid-response | hijack→partial write→close | `http.status=fail` |

### 部分障害シナリオ（probeのコアテスト）

| シナリオ | dns | tcp | tls | http | exit |
|---------|-----|-----|-----|------|------|
| DNS失敗 | fail | skip | skip | skip | 10 |
| DNS ok → TCP失敗 | ok | fail | skip | skip | 20 |
| TCP ok → TLS失敗 | ok | ok | fail | skip | 30 |
| TLS ok → HTTP失敗 | ok | ok | ok | fail | 40 |
| TLS warn + HTTP失敗 | ok | ok | warn | fail | 40 |
| 全成功 | ok | ok | ok | ok | 0 |

### タイムアウト伝播テスト

- 全体バジェット2秒 → DNSで1.5秒消費 → TCP以降は残り0.5秒
- バジェット切れ → 後続レイヤーは `skip`
- 報告されるタイムアウト段階 = 根本原因（最後に観測した症状ではなく）

## Contract Tests（Stripe方式）

JSONスキーマを公開APIとして扱い、後方互換性を自動検証する。

検証項目：
- 全フィールドの存在と型
- `status` の取りうる値: `ok`, `warn`, `fail`, `skip`
- `error` が null or `{code, message}`
- `schema_version` の存在
- フィールド削除がないこと（前バージョンのgoldenと比較）

Golden fileだけでは不十分。**セマンティックなcontract assertion**を別途書く。

## Fuzz Tests（Cloudflare方式）

Go native fuzzingを使用。

対象：
- URL/ターゲットパーサー（スキーム、ホスト、ポート、パス、IPv6、IDNA）
- CLI引数パーサー
- エラーコード分類ロジック

アサーション：
- panic しない
- hang しない（タイムアウト付き）
- メモリ使用量が有界
- エラーが返る場合は正しいエラーコード分類

Seed corpusは `testdata/fuzz/` にチェックイン。

## 実環境テスト（Acceptance Testing）

### 2層構造

#### 層1: Docker Compose canary（決定的・再現可能）

```
docker-compose.yml
├── dns-server (CoreDNS)        # 制御可能なDNS
├── ok-server                    # 200 OK、高速レスポンス
├── slow-server                  # ヘッダー遅延（TTFB検証用）
├── selfsigned-server            # 自己署名証明書
├── expired-server               # 期限切れ証明書
├── wrong-san-server             # SAN不一致
└── error-server                 # 503等
```

ローカルでもCIでも `make acceptance` で同じものが動く。

#### 層2: リファレンスツールとのクロス検証

| 項目 | 比較対象 | 比較方法 |
|------|---------|---------|
| DNS解決結果 | `dig` | 完全一致（IPアドレス） |
| TLS証明書 | `openssl s_client` | 完全一致（fingerprint, SAN, issuer, expiry） |
| HTTPステータス | `curl -w` | 完全一致 |
| DNSタイミング | `dig` | 許容範囲（±200ms or 比率2.5x以内） |
| TCPタイミング | `curl` connect time | 許容範囲 |
| TTFB | `curl` time_starttransfer | 許容範囲（計測定義を先に合わせる） |

**重要:** タイミングの定義はツール間で異なる。比較前に定義を合わせること。

### Acceptance Test マニフェスト

```yaml
- name: canary-ok
  url: https://ok.probe-test.local
  dns:
    compare_with: dig
    exact_answer: true
  tls:
    compare_with: openssl
    compare_fields: [leaf_sha256, subject, not_after]
  http:
    compare_with: curl
    status_exact: true
    ttfb_tolerance_ms: 200
  retries: 3
  require_consensus: 2

- name: canary-expired-cert
  url: https://expired.probe-test.local
  tls:
    expected_status: fail
    expected_error_code: TLS_CERT_EXPIRED
  retries: 1
```

### Acceptance Test 実行フロー

1. `probe --json <url>` → JSON取得
2. `dig` / `openssl` / `curl` を同じターゲットに実行
3. 正規化して比較
   - 事実（IP, cert, status） → 厳密一致
   - タイミング → 許容範囲（`abs(delta) <= N ms` OR `ratio <= 2.5`）
4. 3回リトライ、2/3で合格
5. 失敗時は分類: `tool_bug` / `timing_drift` / `infra_flake`

### Flakiness対策

- 正確性（IP, cert, status）とパフォーマンス（timing）を分離
- リトライ + コンセンサス（2/3 pass）
- 自前canaryインフラをブロッキング判定に使用
- 公開エンドポイントはsmoke用（CI合否には使わない）
- 環境コンテキスト記録（解決IP, プロトコル, ALPN, タイムスタンプ）

## Golden File Tests

- レンダラー出力を `testdata/*.golden` で管理
- シナリオ: `success`, `dns_fail`, `tls_warn`, `http_500`, `timeout`, `partial_failure`
- `-update` フラグで再生成
- タイミング値はテスト時に正規化（`XXms` 等に置換）して決定的に比較

## フェーズごとのテスト計画

### Phase 0: Core types + CLI skeleton + JSON renderer

- Small tests: パーサー、バリデーション、デフォルト値、エラー型、JSON shape
- Contract tests: JSONスキーマのフィールド存在・型検証（ここから開始）
- Golden tests: JSON出力
- Fuzz tests: URL/CLI引数パーサー（ここから開始）

### Phase 1: DNS → TCP → TLS → HTTP layers

- Medium tests: localhost fixture使用（httptest, net.Listen, local TLS）
- **障害マトリクス**: 成功テストより障害テストを先に・多く書く
- タイムアウト/deadline伝播テスト
- 部分障害シナリオ（DNS ok → TCP fail 等）
- Benchmark: レイヤーごとのオーバーヘッド・alloc計測

### Phase 2: Runner + Table renderer + Exit codes

- Race tests: `go test -race` でrunner/timeout周りの並行性
- Exit code contract tests
- Golden tests: table出力（タイミング値正規化）
- ローカルE2E: ビルド済バイナリ + fixture

### Phase 3: E2E + CI/CD + Acceptance

- CI full pyramid: PR=Small+Medium, Nightly=全層+fuzz+acceptance
- Docker Compose canary環境構築
- Acceptance harness: YAML駆動 + クロス検証
- Smoke tests: リリースバイナリの最低限動作確認
- クロスプラットフォーム: linux/darwin × amd64/arm64

## CI統合

| タイミング | テスト内容 |
|-----------|----------|
| PR | `make test-race` (Small + Medium) + `make build` |
| push main | 上記 + `make e2e` + Docker canary acceptance |
| Nightly | 全層 + fuzz（長時間） + 公開エンドポイントsmoke + benchmark trend |
| tag `v*` | 全層 + acceptance + smoke + goreleaser |

## 品質の証拠として公開するもの

1. テストピラミッドの比率（Small:Medium:Large）
2. 障害マトリクスのカバレッジ表
3. スキーマ契約テストの存在
4. Fuzz seed corpusとCI実行ログ
5. `go test -race` がCIで常にpass
6. 全バグ修正にregressionテスト付き
7. リファレンスツールとのクロス検証結果
