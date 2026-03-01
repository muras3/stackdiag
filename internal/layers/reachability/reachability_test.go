package reachability

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
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

func reachObs(t *testing.T, result *core.LayerResult) *core.ReachabilityObservations {
	t.Helper()
	obs, ok := result.Observations.(*core.ReachabilityObservations)
	if !ok {
		t.Fatalf("observations type = %T, want *core.ReachabilityObservations", result.Observations)
	}
	return obs
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

	obs := reachObs(t, result)
	if obs.Reachable == nil || *obs.Reachable != true {
		t.Errorf("reachable = %v, want true", obs.Reachable)
	}
	if obs.ProbeMethod != "icmp" {
		t.Errorf("probe_method = %v, want icmp", obs.ProbeMethod)
	}
	if obs.RTTMS == nil || *obs.RTTMS <= 0 {
		t.Errorf("rtt_ms = %v, want > 0", obs.RTTMS)
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

	obs := reachObs(t, result)
	if obs.Reachable == nil || *obs.Reachable != false {
		t.Errorf("reachable = %v, want false", obs.Reachable)
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

	obs := reachObs(t, result)
	if obs.Reachable != nil {
		t.Errorf("reachable = %v, want nil", obs.Reachable)
	}
	if obs.ProbeMethod != "none" {
		t.Errorf("probe_method = %v, want none", obs.ProbeMethod)
	}
	if obs.SkipReason == nil || *obs.SkipReason != "permission_denied" {
		t.Errorf("skip_reason = %v, want permission_denied", obs.SkipReason)
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

	obs := reachObs(t, result)
	if obs.Reachable == nil || *obs.Reachable != false {
		t.Errorf("reachable = %v, want false", obs.Reachable)
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
	obs := reachObs(t, result)
	if obs.SkipReason == nil || *obs.SkipReason != "permission_denied" {
		t.Errorf("skip_reason = %v, want permission_denied", obs.SkipReason)
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
	tests := []struct {
		name string
		err  error
	}{
		{"dns_error", &net.DNSError{Err: "no suitable address found", Name: "test"}},
		{"string_match", errors.New("no suitable address found")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := skipReason(tt.err)
			if reason != "" {
				t.Errorf("skipReason(%v) = %q, want empty (no skip)", tt.err, reason)
			}
		})
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
