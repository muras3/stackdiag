# Claude Code Agent Teams 実践ガイド

## 概要

Agent Teamsは複数のAIエージェントを並列で動かし、タスクを分担・協調させる機能。調査・実装・レビューを同時進行できる。

## 基本フロー

```
TeamCreate → TaskCreate → Task(team_name付き) → 完了 → SendMessage(shutdown) → TeamDelete
```

## 手順

### 1. チーム作成
```
TeamCreate(team_name="my-team", description="目的")
```

### 2. タスク定義
```
TaskCreate(subject="○○を実装", description="詳細", activeForm="実装中")
```
依存関係があれば`TaskUpdate`で`addBlockedBy`を設定。

### 3. エージェント起動
```
Task(
  prompt="TaskListを確認し、自分のタスクを実行せよ",
  team_name="my-team",
  name="worker-1",
  run_in_background=true
)
```
独立タスクは並列起動。依存タスクは前段完了後に起動。

### 4. 完了・解散
全タスク完了後、`SendMessage(type="shutdown_request")`で各エージェントを停止し、`TeamDelete`で片付け。

## 設計原則

- **ファイル境界の厳守**: エージェント間でファイル競合させない。担当ファイルを明示指定する
- **Wave実行**: 調査→修正→レビューのように段階を分け、各段階内を並列化する
- **subagent上限**: 並列は最大2-4程度が実用的

## tmux並列監視

`run_in_background`で起動したエージェントの出力をtmuxで同時監視する。

### セットアップ
```bash
# 4ペイン構成（エージェント数に応じて調整）
tmux new-session -d -s agents
tmux split-window -h
tmux split-window -v
tmux select-pane -t 0
tmux split-window -v
tmux attach -t agents
```

### 監視開始
Task実行時に返される出力ファイルパスを各ペインで`tail -f`する：
```bash
# ペイン0: リーダー
tail -f /path/to/agent-leader-output.txt
# ペイン1: ワーカー1
tail -f /path/to/worker-1-output.txt
# ペイン2: ワーカー2
tail -f /path/to/worker-2-output.txt
# ペイン3: レビュアー
tail -f /path/to/reviewer-output.txt
```

### 便利設定（~/.tmux.conf）
```bash
# マウス操作有効化（ペイン切替・スクロール）
set -g mouse on
# ペイン境界を見やすく
set -g pane-border-style fg=colour240
set -g pane-active-border-style fg=colour51
# ステータスバーにペイン名表示
set -g status-right '#T'
```

### 終了
```bash
tmux kill-session -t agents
```

## よくある失敗

| 失敗 | 対策 |
|------|------|
| エージェント同士がファイル競合 | 担当ファイルをpromptで明示 |
| 依存タスクを先に実行 | `addBlockedBy`で順序制御 |
| エージェントが暴走 | `TaskStop`で停止 |
| 出力が見えない | tmuxで`tail -f`監視 |
