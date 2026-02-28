package dns

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
	"github.com/muras3/stackdiag/internal/testkit"
)

func makeCtx(timeout time.Duration) *core.ProbeContext {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	_ = cancel // caller should defer cancel in real code
	return &core.ProbeContext{
		Context: ctx,
		Target:  core.Target{Host: "example.com", Port: 443, Scheme: "https"},
	}
}

func TestDNSSuccess(t *testing.T) {
	layer := New(&testkit.FakeResolver{IPs: []string{"203.0.113.10"}})
	pctx := makeCtx(5 * time.Second)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	if result.DurationMS < 0 {
		t.Error("expected non-negative duration")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
	answers, ok := result.Observations["answers"]
	if !ok {
		t.Fatal("missing answers observation")
	}
	ips := answers.([]string)
	if len(ips) != 1 || ips[0] != "203.0.113.10" {
		t.Errorf("answers = %v", ips)
	}
	// ResolvedIPs should be propagated to context.
	if len(pctx.ResolvedIPs) != 1 || pctx.ResolvedIPs[0] != "203.0.113.10" {
		t.Errorf("ResolvedIPs = %v", pctx.ResolvedIPs)
	}
}

func TestDNSNXDOMAIN(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "nx.example.com", IsNotFound: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_NXDOMAIN" {
		t.Errorf("error code = %v, want DNS_NXDOMAIN", result.Error)
	}
}

func TestDNSTimeout(t *testing.T) {
	dnsErr := &net.DNSError{Err: "timeout", Name: "slow.example.com", IsTimeout: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_TIMEOUT" {
		t.Errorf("error code = %v, want DNS_TIMEOUT", result.Error)
	}
}

func TestDNSErrorDefault(t *testing.T) {
	dnsErr := &net.DNSError{Err: "server failure", Name: "fail.example.com"}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
}

func TestDNSGenericError(t *testing.T) {
	layer := New(&testkit.FakeResolver{Err: errors.New("something went wrong")})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
}

func TestDNSContextCanceled(t *testing.T) {
	// Verify that context.DeadlineExceeded is classified as DNS_TIMEOUT.
	layer := New(&testkit.FakeResolver{Err: context.DeadlineExceeded})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_TIMEOUT" {
		t.Errorf("error code = %v, want DNS_TIMEOUT", result.Error)
	}
}

func TestDNSName(t *testing.T) {
	layer := New(&testkit.FakeResolver{IPs: []string{"1.2.3.4"}})
	if layer.Name() != "dns" {
		t.Errorf("Name() = %q, want dns", layer.Name())
	}
}

func TestDNSMultipleIPs(t *testing.T) {
	layer := New(&testkit.FakeResolver{IPs: []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"}})
	pctx := makeCtx(5 * time.Second)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	answers, ok := result.Observations["answers"]
	if !ok {
		t.Fatal("missing answers observation")
	}
	ips := answers.([]string)
	if len(ips) != 3 {
		t.Errorf("answers len = %d, want 3", len(ips))
	}
	if len(pctx.ResolvedIPs) != 3 {
		t.Errorf("ResolvedIPs len = %d, want 3", len(pctx.ResolvedIPs))
	}
}

func TestDNSServfailBecomesErrorWithHint(t *testing.T) {
	dnsErr := &net.DNSError{Err: "server misbehaving", Name: "fail.example.com"}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
	hint, ok := result.Observations["dns_error_hint"]
	if !ok {
		t.Fatal("missing dns_error_hint observation")
	}
	if hint != "servfail" {
		t.Errorf("dns_error_hint = %v, want servfail", hint)
	}
}

func TestDNSRefusedBecomesErrorWithHint(t *testing.T) {
	dnsErr := &net.DNSError{Err: "connection refused", Name: "refused.example.com"}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
	hint, ok := result.Observations["dns_error_hint"]
	if !ok {
		t.Fatal("missing dns_error_hint observation")
	}
	if hint != "refused" {
		t.Errorf("dns_error_hint = %v, want refused", hint)
	}
}

func TestDNSNoAnswerBecomesErrorWithHint(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no answer from DNS server", Name: "empty.example.com"}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
	hint, ok := result.Observations["dns_error_hint"]
	if !ok {
		t.Fatal("missing dns_error_hint observation")
	}
	if hint != "no_answer" {
		t.Errorf("dns_error_hint = %v, want no_answer", hint)
	}
}

func TestDNSNxdomainUnchanged(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "nx.example.com", IsNotFound: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_NXDOMAIN" {
		t.Errorf("error code = %v, want DNS_NXDOMAIN", result.Error)
	}
	hint, ok := result.Observations["dns_error_hint"]
	if !ok {
		t.Fatal("missing dns_error_hint observation")
	}
	if hint != nil {
		t.Errorf("dns_error_hint = %v, want nil", hint)
	}
}

func TestDNSTimeoutUnchanged(t *testing.T) {
	dnsErr := &net.DNSError{Err: "timeout", Name: "slow.example.com", IsTimeout: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_TIMEOUT" {
		t.Errorf("error code = %v, want DNS_TIMEOUT", result.Error)
	}
	hint, ok := result.Observations["dns_error_hint"]
	if !ok {
		t.Fatal("missing dns_error_hint observation")
	}
	if hint != nil {
		t.Errorf("dns_error_hint = %v, want nil", hint)
	}
}

func TestDNSObservationsHaveHintField(t *testing.T) {
	// On success, dns_error_hint should still be present as nil.
	layer := New(&testkit.FakeResolver{IPs: []string{"203.0.113.10"}})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	hint, ok := result.Observations["dns_error_hint"]
	if !ok {
		t.Fatal("missing dns_error_hint observation on success")
	}
	if hint != nil {
		t.Errorf("dns_error_hint = %v, want nil on success", hint)
	}
}

func TestDNSObservationsHaveTTLAndResolver(t *testing.T) {
	// On success, ttl and resolver_address should be present as nil placeholders.
	layer := New(&testkit.FakeResolver{IPs: []string{"203.0.113.10"}})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	ttl, ok := result.Observations["ttl"]
	if !ok {
		t.Fatal("missing ttl observation")
	}
	if ttl != nil {
		t.Errorf("ttl = %v, want nil", ttl)
	}
	resolver, ok := result.Observations["resolver_address"]
	if !ok {
		t.Fatal("missing resolver_address observation")
	}
	if resolver != nil {
		t.Errorf("resolver_address = %v, want nil", resolver)
	}
}

func TestDNSQueryNameOnFailure(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "bad.example.com", IsNotFound: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(5 * time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	qn, ok := result.Observations["query_name"]
	if !ok {
		t.Fatal("missing query_name observation on failure")
	}
	if qn != "example.com" {
		t.Errorf("query_name = %q, want example.com", qn)
	}
}
