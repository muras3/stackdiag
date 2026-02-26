# probe — Project Instructions

## What is this?

`probe` — structured network diagnostics for AI agents and humans.
1コマンドでDNS→TCP→TLS→HTTPを切り分ける軽量Go CLI。

## Key Documents

- `docs/product-design.md` — プロダクト設計、JSONスキーマ、MVP scope
- `docs/development-guide.md` — チーム構成、開発手法、CI/CD、ディレクトリ構造
- `docs/design-decisions.md` — Claude-Codex合意の意思決定ログ
- `docs/test-strategy.md` — テスト戦略、障害マトリクス、実環境検証

## Rules

### Architecture
- 設計判断は必ずCodexと合議する（Claude単独で決めない）
- 内部データモデル = JSON出力。人間向け出力はそのレンダリング層
- スキーマ契約: フィールド削除禁止、型変更禁止、追加のみ許可
- stdout = データ、stderr = ログ（厳格分離）

### Development
- TDD必須。テストを先に書く。例外なし。
- Agent Teams使用（TeamCreate → TaskCreate → Task with team_name）
- subagent並列は最大2
- コミットは小刻みに
- Makefileでローカル/CI同一性を保証

### Team Roles
- Architect: Claude（Codexと合議）
- Test Designer: Claude
- General Implementer: Claude（CLI, runner, renderer, DNS/TCP）
- Core Tech Implementer: Codex（TLS/HTTP計測）
- Code Reviewer: Codex（全コード）
- UI/UX Lead: Claude
- Final Approver: Human

### Codex Integration
- Core Tech依頼時はインターフェース凍結済であること
- レビュー依頼: diff + テスト結果 + 変更意図（1-3行）
- 計測セマンティクスはコード内に文書化
- `codex exec` で実装タスク、`codex review` でレビュー

### Tech Stack
- Go (stdlib中心)
- CGO_ENABLED=0
- goreleaser でリリース
- GitHub Actions でCI/CD
