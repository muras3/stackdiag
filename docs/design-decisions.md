# stackdiag — Design Decisions Log

Claude-Codex間の議論で合意した意思決定の記録。

## Decision 1: Go（Rustではなく）

**合意:** Claude・Codex双方

- stdlib `net/tls/http` がprobeの要件にそのまま使える
- クロスコンパイルが容易で信頼できる
- CLI/opsツール領域ではGoのコミュニティが大きい
- Rustの利点（小バイナリ、型安全性）はこのカテゴリでは開発速度に見合わない

## Decision 2: layers はobject（arrayではなく）

**提案:** Codex → array、Claude → object
**結論:** object（Codex同意）

- `result.layers.dns.status` が `result.layers.find(l => l.name == "dns").status` より自然
- jq: `.layers.tls.timing` vs `.layers[] | select(.name=="tls") | .timing`
- レイヤー順はスキーマ契約で固定（データに埋め込む必要なし）
- 将来動的/繰り返しフェーズが必要になったら別フィールドで対応

## Decision 3: error（issueではなく）

**提案:** Codex → issue、Claude → error
**結論:** error（Codex同意）

- API設計の普遍的慣例
- severityは `status` フィールドが担当
- `status: "ok"` + `error: null` = 正常
- `status: "warn"` + `error: {...}` = 警告
- `status: "fail"` + `error: {...}` = 失敗

## Decision 4: trace_id / attempt はMVPに入れない

**提案:** Codex → 入れる、Claude → YAGNI
**結論:** 入れない（Codex同意）

- リトライ、バッチ、分散相関の要件がない
- `--count N` 実装時に追加する

## Decision 5: observations はレイヤーローカル + 最小限のトップレベル

**提案:** Codex → dotted keys廃止、Claude → レイヤー内に移動＋トップレベルは最小限
**結論:** Claudeの提案（Codex同意）

- レイヤー固有の観測はそのレイヤー内に型付きフィールドで格納
- トップレベル `observations` はクロスレイヤーの派生事実のみ（MVPでは空object可）

## Decision 6: MCP不要（MVPでは）

**合意:** Claude・Codex双方

- probeは完全にステートレス
- MCPの常駐プロセスモデルが活きる要件がない
- 内部をrequest→responseの純粋APIとして切り出し、将来MCPアダプタを被せられる設計にする

## Decision 7: screenshot-worthy な人間向け出力

**合意:** Claude・Codex双方

- 色: 緑✓ / 黄⚠ / 赤✗
- モノスペースアラインメント
- `NO_COLOR` / non-TTY でASCIIフォールバック
- タイミングバーはv0.2（MVPでは見送り）

## Decision 8: Agent-friendly = 安定した判断インターフェース

**合意:** Claude・Codex双方

- `--json` は「おまけ機能」ではなく、Agentが使うプロダクト面そのもの
- schema_version で契約を明示
- エラーコードは構造化（free text禁止）
- stdout=データ、stderr=ログの厳格分離

## Decision 9: ビルド順は自然なスタック順

**提案:** Codex → TCP first、Claude → DNS first
**結論:** DNS → TCP → TLS → HTTP（Codex同意）

- 実行順と一致し、メンタルモデルがシンプル
- DNSはfakeが最も簡単で、契約フローの検証が早期にできる

## Decision 10: Makefile でローカル/CI同一性を保証

**合意:** Claude・Codex双方

- CIは `make lint`, `make test-race`, `make build` を直接呼ぶ
- `fmt` と `fmt-check` を分離（CIは strict）
- ローカルで通ればCIでも通る

## Decision 11: subagent並列は最大2

**提案:** Codex → 最初は2で
**結論:** 最大2並列（Claude同意）

- レビュー帯域がボトルネック
- パターン安定前の4並列はマージ摩擦が大きい
- DNS完了後、TCP+TLSを並列にする運用

## Decision 12: チーム構成

**決定:** Human（最終承認者）

- Architect: Claude（設計判断はCodexと合議必須）
- Test Designer: Claude
- General Implementer: Claude
- Core Tech Implementer: Codex（TLS/HTTP計測）
- Code Reviewer: Codex（全コード）
- UI/UX Lead: Claude
