package reachability

import (
	"context"
	"encoding/binary"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
	"github.com/muras3/stackdiag/internal/testkit"
)

func makeCtx(t *testing.T, timeout time.Duration) *core.ProbeContext {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return &core.ProbeContext{
		Context: ctx,
		Target:  core.Target{Host: "example.com", Port: 443, Scheme: "https"},
	}
}

func TestReachabilityName(t *testing.T) {
	layer := New(&testkit.FakePinger{})
	if layer.Name() != "reachability" {
		t.Errorf("Name() = %q, want reachability", layer.Name())
	}
}

func TestReachabilityOK(t *testing.T) {
	layer := New(&testkit.FakePinger{RTT: 12 * time.Millisecond})
	pctx := makeCtx(t, 5*time.Second)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	reachable, ok := result.Observations["reachable"]
	if !ok {
		t.Fatal("missing reachable observation")
	}
	if reachable != true {
		t.Errorf("reachable = %v, want true", reachable)
	}

	method, ok := result.Observations["probe_method"]
	if !ok {
		t.Fatal("missing probe_method observation")
	}
	if method != "icmp" {
		t.Errorf("probe_method = %v, want icmp", method)
	}

	rtt, ok := result.Observations["rtt_ms"]
	if !ok {
		t.Fatal("missing rtt_ms observation")
	}
	rttVal, ok := rtt.(float64)
	if !ok || rttVal <= 0 {
		t.Errorf("rtt_ms = %v, want > 0", rtt)
	}
}

func TestReachabilityTimeout(t *testing.T) {
	layer := New(&testkit.FakePinger{Err: context.DeadlineExceeded})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "REACHABILITY_TIMEOUT" {
		t.Errorf("error code = %v, want REACHABILITY_TIMEOUT", result.Error)
	}

	reachable, ok := result.Observations["reachable"]
	if !ok {
		t.Fatal("missing reachable observation")
	}
	if reachable != false {
		t.Errorf("reachable = %v, want false", reachable)
	}
}

func TestReachabilityPermissionDenied(t *testing.T) {
	layer := New(&testkit.FakePinger{Err: syscall.EPERM})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusSkip {
		t.Errorf("status = %q, want skip", result.Status)
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	reachable := result.Observations["reachable"]
	if reachable != nil {
		t.Errorf("reachable = %v, want nil", reachable)
	}

	method, ok := result.Observations["probe_method"]
	if !ok {
		t.Fatal("missing probe_method observation")
	}
	if method != "none" {
		t.Errorf("probe_method = %v, want none", method)
	}

	skipReason, ok := result.Observations["skip_reason"]
	if !ok {
		t.Fatal("missing skip_reason observation")
	}
	if skipReason != "permission_denied" {
		t.Errorf("skip_reason = %v, want permission_denied", skipReason)
	}
}

func TestReachabilityError(t *testing.T) {
	layer := New(&testkit.FakePinger{Err: errors.New("network unreachable")})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "REACHABILITY_ERROR" {
		t.Errorf("error code = %v, want REACHABILITY_ERROR", result.Error)
	}

	reachable, ok := result.Observations["reachable"]
	if !ok {
		t.Fatal("missing reachable observation")
	}
	if reachable != false {
		t.Errorf("reachable = %v, want false", reachable)
	}
}

func TestReachabilityUsesResolvedIPs(t *testing.T) {
	var pingedAddr string
	pinger := &recordingPinger{
		FakePinger: testkit.FakePinger{RTT: 5 * time.Millisecond},
		addrRef:    &pingedAddr,
	}
	layer := New(pinger)
	pctx := makeCtx(t, 5*time.Second)
	pctx.ResolvedIPs = []string{"203.0.113.10", "203.0.113.11"}
	layer.Probe(pctx)

	if pingedAddr != "203.0.113.10" {
		t.Errorf("pinged addr = %q, want 203.0.113.10", pingedAddr)
	}
}

func TestReachabilityUsesHostWhenNoResolvedIPs(t *testing.T) {
	var pingedAddr string
	pinger := &recordingPinger{
		FakePinger: testkit.FakePinger{RTT: 5 * time.Millisecond},
		addrRef:    &pingedAddr,
	}
	layer := New(pinger)
	pctx := makeCtx(t, 5*time.Second)
	// No ResolvedIPs set
	layer.Probe(pctx)

	if pingedAddr != "example.com" {
		t.Errorf("pinged addr = %q, want example.com", pingedAddr)
	}
}

func TestReachabilityPermissionDeniedEACCES(t *testing.T) {
	layer := New(&testkit.FakePinger{Err: syscall.EACCES})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusSkip {
		t.Errorf("status = %q, want skip", result.Status)
	}
	skipReason := result.Observations["skip_reason"]
	if skipReason != "permission_denied" {
		t.Errorf("skip_reason = %v, want permission_denied", skipReason)
	}
}

// recordingPinger wraps FakePinger and records the address passed to Ping.
type recordingPinger struct {
	testkit.FakePinger
	addrRef *string
}

func (r *recordingPinger) Ping(ctx context.Context, addr string) (time.Duration, error) {
	*r.addrRef = addr
	return r.FakePinger.Ping(ctx, addr)
}

func TestSkipReasonNoLongerSkipsIPv6(t *testing.T) {
	// After IPv6 support, "no suitable address found" should no longer
	// produce unsupported_address skip. Permission errors should still skip.
	layer := New(&testkit.FakePinger{Err: syscall.EPERM})
	result := layer.Probe(makeCtx(t, 5*time.Second))
	if result.Observations["skip_reason"] != "permission_denied" {
		t.Errorf("skip_reason = %v, want permission_denied", result.Observations["skip_reason"])
	}
}

func TestBuildICMPv6EchoRequest(t *testing.T) {
	msg := buildICMPv6EchoRequest(0x1234, 1)
	if len(msg) != 8 {
		t.Fatalf("len = %d, want 8", len(msg))
	}
	if msg[0] != 128 {
		t.Errorf("type = %d, want 128", msg[0])
	}
	if msg[1] != 0 {
		t.Errorf("code = %d, want 0", msg[1])
	}
	csum := binary.BigEndian.Uint16(msg[2:4])
	if csum != 0 {
		t.Errorf("checksum = %d, want 0 (kernel-computed)", csum)
	}
	id := binary.BigEndian.Uint16(msg[4:6])
	if id != 0x1234 {
		t.Errorf("id = 0x%04x, want 0x1234", id)
	}
	seq := binary.BigEndian.Uint16(msg[6:8])
	if seq != 1 {
		t.Errorf("seq = %d, want 1", seq)
	}
}

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"203.0.113.10", false},
		{"::1", true},
		{"2001:db8::1", true},
		{"example.com", false},
		{"127.0.0.1", false},
	}
	for _, tt := range tests {
		if got := isIPv6(tt.addr); got != tt.want {
			t.Errorf("isIPv6(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}
