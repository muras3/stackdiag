# stackdiag — Development Guide

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

詳細は `docs/test-strategy.md` を参照。

要約：
- **テストピラミッド**: Small多数 / Medium中程度 / Large少数
- **障害マトリクス**: レイヤー×障害パターンの網羅（成功より障害テストが重要）
- **実環境検証**: Docker Compose canary + dig/openssl/curlクロス検証
- **Contract tests**: JSONスキーマの後方互換性自動検証
- **Fuzz tests**: Go native fuzzing でパーサー堅牢性保証

## Directory Structure

```
cmd/stackdiag/main.go
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
test/
  e2e/            # binary-level tests
  acceptance/     # real-network cross-validation harness
testdata/
  golden/         # renderer output snapshots
  fuzz/           # fuzz seed corpus
  certs/          # TLS test fixture certificates
docker-compose.test.yml  # canary test infrastructure
Makefile
```

## CI/CD

### Local/CI Parity via Makefile

```makefile
lint:        go vet ./... && gofumpt -l -d . (差分あればfail)
test:        go test ./...
test-race:   go test -race ./...
test-fuzz:   go test -fuzz=. -fuzztime=30s ./internal/core/...
build:       CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/stdiag ./cmd/stackdiag
e2e:         build → go test ./test/e2e/...
acceptance:  docker compose -f docker-compose.test.yml up -d → go test ./test/acceptance/... → down
```

### GitHub Actions Pipeline

| トリガー | 実行 |
|---------|------|
| PR | lint → test-race → build |
| push main | lint → test-race → build → e2e → acceptance (Docker canary) |
| Nightly | 全層 + fuzz(長時間) + 公開エンドポイントsmoke + benchmark trend |
| tag `v*` | 全層 + acceptance + smoke → goreleaser |

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
