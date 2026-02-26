# probe — Development Guide

## Team Structure

| ロール | 担当 | 備考 |
|--------|------|------|
| Architect | Claude | **全設計判断はCodexと合議の上で決定** |
| Test Designer | Claude | テスト戦略・構造設計 |
| General Implementer | Claude | CLI, runner, renderer, DNS/TCP |
| Core Tech Implementer | Codex | TLS/HTTP計測、難所の深掘り |
| Code Reviewer | Codex | 全コードレビュー |
| UI/UX Lead | Claude | 体験設計、非機能要件 |
| Final Approver | Human | 全メジャー判断の最終決定 |

### Codex Ground Rules

- Core Tech作業にはインターフェース凍結後に依頼する
- レビュー依頼には diff + テスト結果 + 変更意図（1-3行）を含める
- 計測セマンティクス（TTFBの開始/終了点等）をコード内に文書化する
- Codex自身のコードは利益相反があるため、高リスク箇所は追加レビュー推奨
- リファクタとロジック変更は分離する

## Development Methodology

### TDD (Test-Driven Development)

```
RED → 失敗確認 → GREEN → リファクタ → commit
```

- テストを先に書く。例外なし。
- テストが先に失敗することを確認してから実装。
- 最小限の実装でテストを通す。

### Subagent-Driven Development (Agent Teams)

タスクごとにfreshなsubagentを起動。TeamCreate → TaskCreate → Task(team_name付き)。

各タスクのフロー：
1. Implementer subagent: テスト → 実装 → セルフレビュー → commit
2. Spec reviewer subagent: 仕様準拠チェック
3. Code quality reviewer (Codex): コード品質チェック

並列は最大2 subagent。パターンが安定するまでは逐次。

## Build Phases

```
Phase 0 (逐次・契約固め)
  ① Core types + Layer interface
  ② CLI skeleton + stub runner
  ③ JSON renderer

Phase 1 (最大2並列)
  ④ DNS layer
  ⑤ TCP layer      ← ④完了後、⑤⑥を並列可
  ⑥ TLS layer (Codex)
  ⑦ HTTP layer (Codex)

Phase 2
  ⑧ Runner orchestration
  ⑨ Table renderer
  ⑩ Exit code mapping

Phase 3
  ⑪ E2E tests + CI/CD setup
```

## Test Architecture

### Test Tiers

| 層 | 方式 | 速度 |
|----|------|------|
| Unit | fake Resolver/Dialer/Transport注入 | <1s |
| Integration | localhost httptest, net.Listen | <3s |
| E2E | ビルド済バイナリ exec.Command | <10s |
| External | `RUN_EXTERNAL=1` でopt-in | CI対象外 |

### Faking Strategy

| レイヤー | Fake方式 |
|---------|---------|
| DNS | `Resolver` interface + fake resolver |
| TCP | `Dialer` interface + local `net.Listen` |
| TLS | local TLS server or abstracted handshake |
| HTTP | `http.Client` with custom transport / `httptest.Server` |

### Golden File Tests

- レンダラー出力を `testdata/*.golden` で管理
- シナリオ: success, dns_fail, tls_warn, http_500, timeout
- `-update` フラグで再生成

## Directory Structure

```
cmd/probe/main.go
internal/
  core/           # types, config, result, Layer interface
  runner/         # orchestration
  layers/
    dns/
    tcp/
    tls/
    http/
  render/
    json/
    table/
  exitcode/
  testkit/        # shared fakes
test/e2e/
testdata/         # golden files
Makefile
```

## CI/CD

### Local/CI Parity via Makefile

```makefile
lint:       go vet ./... && gofumpt -l -d . (差分あればfail)
test:       go test ./...
test-race:  go test -race ./...
build:      CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/probe ./cmd/probe
e2e:        build → go test ./test/e2e/...
```

### GitHub Actions Pipeline

| トリガー | 実行 |
|---------|------|
| PR | lint → test-race → build |
| push main | lint → test-race → build → e2e |
| tag `v*` | 上記 + goreleaser |

### Release

- goreleaser で linux/darwin × amd64/arm64 のバイナリ生成
- バージョン注入: `-ldflags "-X main.version=$TAG -X main.commit=$SHA"`
- `CGO_ENABLED=0` 固定
- GitHub Releases にアーカイブ + チェックサム

### Git Hooks

- pre-commit: `gofumpt` のみ（軽量）
- 重いチェック（lint, test）はpre-pushまたはCIで

## Effort Estimate

| パッケージ | Claude Code併用見積 |
|-----------|-------------------|
| Core types | 2-3h |
| CLI skeleton | 2-3h |
| Runner | 3-4h |
| DNS layer | 3-4h |
| TCP layer | 2-3h |
| TLS layer (Codex) | 5-7h |
| HTTP layer (Codex) | 6-8h |
| JSON renderer | 1h |
| Table renderer | 4-6h |
| Exit code | 1-2h |
| E2E + hardening | 4-6h |
| **合計** | **33-47h（計画値: 40h）** |

最大リスク: HTTP/TLS計測の正確性とレイヤー分離。
