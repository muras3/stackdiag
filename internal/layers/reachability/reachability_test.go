package reachability

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
	"github.com/muras3/stackdiag/internal/testkit"
)

func makeCtx(timeout time.Duration) *core.ProbeContext {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	_ = cancel
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
	pctx := makeCtx(5 * time.Second)
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
	result := layer.Probe(makeCtx(5 * time.Second))

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
	result := layer.Probe(makeCtx(5 * time.Second))

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
	result := layer.Probe(makeCtx(5 * time.Second))

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
	pctx := makeCtx(5 * time.Second)
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
	pctx := makeCtx(5 * time.Second)
	// No ResolvedIPs set
	layer.Probe(pctx)

	if pingedAddr != "example.com" {
		t.Errorf("pinged addr = %q, want example.com", pingedAddr)
	}
}

func TestReachabilityPermissionDeniedEACCES(t *testing.T) {
	layer := New(&testkit.FakePinger{Err: syscall.EACCES})
	result := layer.Probe(makeCtx(5 * time.Second))

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
