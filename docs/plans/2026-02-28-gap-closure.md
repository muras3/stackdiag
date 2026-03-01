# Gap Closure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close all remaining gaps in dev/v0.0.1: IPv6 reachability, DNS resolver_address, help text exit code 15, schema doc update.

**Architecture:** IPv6は`icmpPinger`内でv4/v6自動判定。DNS resolver_addressは`net.Resolver.Dial`フックで取得。すべてstdlib完結。

**Tech Stack:** Go stdlib only. CGO_ENABLED=0.

---

## Agent Teams 構成

```
Lead (orchestrator)
├── impl-ipv6         : Task 1-2 (IPv6 reachability) — worktree隔離
├── impl-dns          : Task 3 (DNS resolver_address) — worktree隔離
├── impl-fixes        : Task 4-5 (help text + schema doc) — worktree隔離
├── reviewer-code     : コミット単位でコードレビュー（指摘0まで反復）
└── reviewer-security : セキュリティレビュー（指摘0まで反復）
```

レビューフロー: 実装エージェントがコミット → 即座にreviewerに通知 → レビュー → 指摘あれば実装エージェントが修正&再コミット → 再レビュー → 指摘0で完了

---

## Task 1: IPv6 ICMPv6 Ping実装 (TDD)

**Files:**
- Modify: `internal/layers/reachability/reachability.go:117-215` (icmpPinger + helpers)
- Modify: `internal/layers/reachability/reachability_test.go`
- Modify: `internal/layers/reachability/reachability.go:96-114` (skipReason)

### Step 1: テスト追加 — IPv6アドレス判定ヘルパー

`reachability_test.go` に追加:

```go
func TestIsIPv6(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"203.0.113.10", false},
		{"::1", true},
		{"2001:db8::1", true},
		{"example.com", false}, // hostname, not IPv6
		{"127.0.0.1", false},
	}
	for _, tt := range tests {
		if got := isIPv6(tt.addr); got != tt.want {
			t.Errorf("isIPv6(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}
```

### Step 2: テスト実行 — FAIL確認

```bash
cd /Users/murase/project/probe && go test ./internal/layers/reachability/ -run TestIsIPv6 -v
```
Expected: FAIL — `isIPv6` undefined

### Step 3: isIPv6 実装

`reachability.go` に追加:

```go
// isIPv6 returns true if addr is an IPv6 address literal.
func isIPv6(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}
```

### Step 4: テスト実行 — PASS確認

```bash
cd /Users/murase/project/probe && go test ./internal/layers/reachability/ -run TestIsIPv6 -v
```
Expected: PASS

### Step 5: コミット

```bash
git add internal/layers/reachability/reachability.go internal/layers/reachability/reachability_test.go
git commit -m "feat(reachability): add isIPv6 helper for v4/v6 protocol selection"
```

→ **レビュー依頼**

### Step 6: テスト追加 — ICMPv6 Echo Request構築

```go
func TestBuildICMPv6EchoRequest(t *testing.T) {
	msg := buildICMPv6EchoRequest(0x1234, 1)
	if len(msg) != 8 {
		t.Fatalf("len = %d, want 8", len(msg))
	}
	if msg[0] != 128 { // ICMPv6 Echo Request type
		t.Errorf("type = %d, want 128", msg[0])
	}
	if msg[1] != 0 { // Code
		t.Errorf("code = %d, want 0", msg[1])
	}
	// Checksum should be 0 (kernel computes for ICMPv6)
	csum := binary.BigEndian.Uint16(msg[2:4])
	if csum != 0 {
		t.Errorf("checksum = %d, want 0 (kernel-computed)", csum)
	}
	id := binary.BigEndian.Uint16(msg[4:6])
	if id != 0x1234 {
		t.Errorf("id = 0x%04x, want 0x1234", id)
	}
}
```

### Step 7: テスト実行 — FAIL確認

### Step 8: buildICMPv6EchoRequest 実装

```go
// buildICMPv6EchoRequest builds an ICMPv6 Echo Request (type=128, code=0).
// Checksum is set to 0 — the kernel computes it for ICMPv6 raw sockets.
func buildICMPv6EchoRequest(id, seq uint16) []byte {
	msg := make([]byte, 8)
	msg[0] = 128 // Type: ICMPv6 Echo Request
	msg[1] = 0   // Code
	// Checksum at [2:4] = 0 (kernel-computed for ICMPv6)
	binary.BigEndian.PutUint16(msg[4:6], id)
	binary.BigEndian.PutUint16(msg[6:8], seq)
	return msg
}
```

### Step 9: テスト実行 — PASS確認

### Step 10: コミット

```bash
git add internal/layers/reachability/reachability.go internal/layers/reachability/reachability_test.go
git commit -m "feat(reachability): add ICMPv6 Echo Request builder"
```

→ **レビュー依頼**

---

## Task 2: icmpPinger IPv6対応 + skipReason更新

**Files:**
- Modify: `internal/layers/reachability/reachability.go:120-181` (Ping method)
- Modify: `internal/layers/reachability/reachability.go:96-114` (skipReason)
- Modify: `internal/layers/reachability/reachability_test.go`
- Modify: `docs/schema.md:98-101` (design notes更新)

### Step 1: テスト追加 — Pingメソッドのプロトコル選択

FakePingerはアドレスを記録しないため、integration-levelの確認はicmpPinger自体で行う。
ここではskipReason更新のユニットテストを追加:

```go
func TestSkipReasonNoLongerSkipsIPv6(t *testing.T) {
	// After IPv6 support, "no suitable address found" for IPv6 should not
	// produce unsupported_address skip. However, we still handle the case
	// where the error is genuinely unsupported (e.g., non-IP hostname resolution fail).
	// This test verifies the skip reason logic remains correct for permission errors.
	layer := New(&testkit.FakePinger{Err: syscall.EPERM})
	result := layer.Probe(makeCtx(t, 5*time.Second))
	if result.Observations["skip_reason"] != "permission_denied" {
		t.Errorf("skip_reason = %v, want permission_denied", result.Observations["skip_reason"])
	}
}
```

### Step 2: icmpPinger.Ping を IPv6 対応にリファクタ

```go
func (p *icmpPinger) Ping(ctx context.Context, addr string) (time.Duration, error) {
	// Select protocol based on address type.
	network := "ip4:icmp"
	v6 := isIPv6(addr)
	if v6 {
		network = "ip6:ipv6-icmp"
	}

	conn, err := net.Dial(network, addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	id := uint16(os.Getpid() & 0xffff)
	var msg []byte
	if v6 {
		msg = buildICMPv6EchoRequest(id, 1)
	} else {
		msg = buildICMPEchoRequest(id, 1)
	}

	start := time.Now()
	if _, err := conn.Write(msg); err != nil {
		return 0, err
	}

	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			return 0, err
		}

		if v6 {
			// IPv6 raw sockets do NOT include the IP header.
			if n < 8 {
				continue
			}
			// ICMPv6 Echo Reply: type=129, code=0
			if buf[0] == 129 && buf[1] == 0 && n >= 6 {
				replyID := binary.BigEndian.Uint16(buf[4:6])
				if replyID == id {
					return time.Since(start), nil
				}
			}
		} else {
			// IPv4: skip IP header
			if n < 20 {
				continue
			}
			ipHeaderLen := int(buf[0]&0x0f) << 2
			if n < ipHeaderLen+8 {
				continue
			}
			icmpData := buf[ipHeaderLen:n]
			if icmpData[0] == 0 && icmpData[1] == 0 && len(icmpData) >= 6 {
				replyID := binary.BigEndian.Uint16(icmpData[4:6])
				if replyID == id {
					return time.Since(start), nil
				}
			}
		}
	}
}
```

### Step 3: skipReason更新 — IPv6 "unsupported_address" を削除

```go
func skipReason(err error) string {
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return "permission_denied"
	}
	var opErr *os.PathError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.EPERM) || errors.Is(opErr.Err, syscall.EACCES) {
			return "permission_denied"
		}
	}
	return ""
}
```

注意: `unsupported_address` は完全削除ではなく、IPv6固有の自動スキップを削除。
ただしスキーマ上 `unsupported_address` は残す（将来の拡張用）。

### Step 4: テスト実行 — 全テスト PASS確認

```bash
cd /Users/murase/project/probe && go test ./internal/layers/reachability/ -v
```

### Step 5: docs/schema.md のdesign notes更新

100行目を変更:
```
- When the target address is not supported (e.g., IPv6 with IPv4-only ICMP): ...
```
を:
```
- IPv4 and IPv6 addresses are both supported via ICMP/ICMPv6.
- `skip_reason: "unsupported_address"` is reserved for future address types that cannot be probed.
```

### Step 6: コミット

```bash
git add internal/layers/reachability/reachability.go internal/layers/reachability/reachability_test.go docs/schema.md
git commit -m "feat(reachability): add IPv6 ICMPv6 support

- Auto-detect v4/v6 via net.ParseIP
- ICMPv6 Echo Request (type=128) / Reply (type=129)
- Kernel-computed checksum for ICMPv6
- IPv6 raw socket has no IP header (direct ICMP payload)
- Remove automatic unsupported_address skip for IPv6"
```

→ **レビュー依頼**

---

## Task 3: DNS resolver_address 実装 (TDD)

**Files:**
- Modify: `internal/layers/dns/dns.go` (Resolver interface拡張 + Dialフック)
- Modify: `internal/layers/dns/dns_test.go`
- Modify: `internal/testkit/fakes.go` (FakeResolver更新)

### Step 1: テスト追加 — resolver_addressが返されること

```go
func TestDNSResolverAddressReturned(t *testing.T) {
	resolver := &testkit.FakeResolver{
		IPs:             []string{"203.0.113.10"},
		ResolverAddress: "192.168.1.1:53",
	}
	layer := New(resolver)
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok", result.Status)
	}
	addr := result.Observations["resolver_address"]
	if addr != "192.168.1.1:53" {
		t.Errorf("resolver_address = %v, want 192.168.1.1:53", addr)
	}
}
```

### Step 2: テスト実行 — FAIL確認

### Step 3: Resolver interface拡張

DNS layerのResolverインターフェースを拡張する方法:

Option A: インターフェース自体を拡張（後方互換なし）
Option B: オプショナルインターフェースで型アサーション

→ **Option B** を採用（既存FakeResolverとの互換性維持）

```go
// ResolverWithAddress is an optional interface that Resolver implementations
// can satisfy to report the DNS resolver address used.
type ResolverWithAddress interface {
	ResolverAddress() string
}
```

DNS Layer の `NewDefault()` で `net.Resolver` をカスタム `Dial` 付きでラップ:

```go
// trackingResolver wraps net.Resolver with a Dial hook to capture resolver address.
type trackingResolver struct {
	inner   *net.Resolver
	address string // last resolver address seen
}

func newTrackingResolver() *trackingResolver {
	tr := &trackingResolver{}
	tr.inner = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			tr.address = address
			var d net.Dialer
			return d.DialContext(ctx, network, address)
		},
	}
	return tr
}

func (tr *trackingResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return tr.inner.LookupHost(ctx, host)
}

func (tr *trackingResolver) ResolverAddress() string {
	return tr.address
}
```

Probe内で型アサーション:

```go
// After resolution, check if resolver reports its address.
var resolverAddr any
if ra, ok := l.resolver.(ResolverWithAddress); ok {
	if addr := ra.ResolverAddress(); addr != "" {
		resolverAddr = addr
	}
}
```

### Step 4: testkit/fakes.go の FakeResolver に ResolverAddress追加

```go
type FakeResolver struct {
	IPs             []string
	Err             error
	ResolverAddress string // optional: simulates resolver address
}

func (f *FakeResolver) LookupHost(_ context.Context, _ string) ([]string, error) {
	return f.IPs, f.Err
}

// ResolverAddress implements ResolverWithAddress.
func (f *FakeResolver) ResolverAddress() string {
	return f.ResolverAddress
}
```

注意: フィールド名とメソッド名の衝突回避が必要。フィールドを `ResolverAddr` にリネーム:

```go
type FakeResolver struct {
	IPs          []string
	Err          error
	ResolverAddr string
}

func (f *FakeResolver) ResolverAddress() string {
	return f.ResolverAddr
}
```

### Step 5: NewDefault更新

```go
func NewDefault() *Layer {
	return &Layer{resolver: newTrackingResolver()}
}
```

### Step 6: テスト実行 — PASS確認

```bash
cd /Users/murase/project/probe && go test ./internal/layers/dns/ -v
```

### Step 7: 既存テスト確認 — TestDNSObservationsHaveTTLAndResolver が nil→実値に変わるか確認

既存テストの `resolver_address` が `nil` を期待しているので、FakeResolverの`ResolverAddr`が空文字列の場合は`nil`を返すようにする。

### Step 8: コミット

```bash
git add internal/layers/dns/dns.go internal/layers/dns/dns_test.go internal/testkit/fakes.go
git commit -m "feat(dns): implement resolver_address via net.Resolver.Dial hook

- Add trackingResolver with PreferGo:true and custom Dial
- Capture resolver IP:port from Dial callback
- Optional ResolverWithAddress interface for backward compat
- Update FakeResolver with ResolverAddr field"
```

→ **レビュー依頼**

---

## Task 4: ヘルプテキスト exit code 15 追加

**Files:**
- Modify: `internal/cli/cli.go:66-73`
- Modify: E2Eテスト（もしヘルプ出力テストがあれば）

### Step 1: テスト追加

```go
func TestHelpTextContainsExitCode15(t *testing.T) {
	help := HelpText()
	if !strings.Contains(help, "15") {
		t.Error("HelpText() does not contain exit code 15")
	}
	if !strings.Contains(help, "Reachability") || !strings.Contains(help, "reachability") {
		t.Error("HelpText() does not mention reachability for exit code 15")
	}
}
```

### Step 2: テスト実行 — FAIL確認

### Step 3: `cli.go` 70行目の後に追加

```
  10  DNS failure
  15  Reachability failure
  20  TCP failure
```

### Step 4: テスト実行 — PASS確認

### Step 5: コミット

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "fix(cli): add exit code 15 (reachability) to help text"
```

→ **レビュー依頼**

---

## Task 5: スキーマドキュメント整合

**Files:**
- Modify: `docs/schema.md` (reachability design notes — Task 2 で対応済みなら確認のみ)
- Modify: `CLAUDE.md` (参照ドキュメントパス修正)

### Step 1: CLAUDE.md の Key Documents を実在ファイルに修正

```markdown
## Key Documents

- `docs/schema.md` — JSONスキーマ仕様、レイヤー定義、exit codes
- `docs/design-decisions.md` — Claude-Codex合意の意思決定ログ
- `docs/plans/` — 実装計画
```

### Step 2: コミット

```bash
git add CLAUDE.md
git commit -m "docs: fix Key Documents references in CLAUDE.md to match actual files"
```

→ **レビュー依頼**

---

## Review Protocol

### コードレビュー (reviewer-code)

各コミットに対して:
1. diff を読む
2. 以下の観点でチェック:
   - TDD順守（テスト先行か）
   - スキーマ契約違反なし（フィールド削除禁止、型変更禁止）
   - エラーハンドリング漏れ
   - 既存テスト破壊なし
   - CGO_ENABLED=0 互換性
3. 指摘事項をリスト化
4. 指摘0になるまで反復

### セキュリティレビュー (reviewer-security)

全変更に対して:
1. Raw socket操作の安全性（バッファオーバーフロー、境界チェック）
2. ICMPv6チェックサム処理の正当性
3. `net.Resolver.Dial` フックでの情報漏洩リスク
4. 入力バリデーション（IPv6アドレスパース）
5. 指摘0になるまで反復
