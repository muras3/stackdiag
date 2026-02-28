# stackdiag — Project Instructions

## What is this?

`stackdiag` — structured network diagnostics for AI agents and humans.
1コマンドでDNS→TCP→TLS→HTTPを切り分ける軽量Go CLI。

## Key Documents

- `docs/schema.md` — JSONスキーマ仕様、レイヤー定義、exit codes
- `docs/design-decisions.md` — Claude-Codex合意の意思決定ログ
- `docs/plans/` — 実装計画

## Rules

### Architecture
- 設計判断は必ずCodexと合議する（Claude単独で決めない）
- 内部データモデル = JSON出力。人間向け出力はそのレンダリング層
- スキーマ契約: フィールド削除禁止、型変更禁止、追加のみ許可
- stdout = データ、stderr = ログ（厳格分離）

### Development
- TDD必須。テストを先に書く。例外なし。
- コミットは小刻みに
- Makefileでローカル/CI同一性を保証

### Agent Teams
- Agent Teamsを最大限活用する（TeamCreate → TaskCreate → Task with team_name）
- 並列数に上限なし。独立性の高いタスクは可能な限り同時起動
- 各エージェントは `isolation: worktree` で隔離し、マージ競合を排除
- 各エージェントは自己検証してから完了報告すること
- 実装エージェントはレビューを待たずに次の作業に進む（待ち時間ゼロ設計）
- インターフェースを事前合意し、エージェント間の依存をデータ形式に限定

### Code Review
- 全コードをCodexにレビュー依頼する。例外なし
- コミットごとにCodexレビューを実行（`codex exec`）
- コードレビューとセキュリティレビューは別エージェントで並列実行
- レビュー担当エージェントはバックグラウンドで常駐し、コミット通知を受け次第レビュー開始

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
