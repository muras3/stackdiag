package dns

import (
	"context"
	"errors"
	"net"
	"strings"
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

func dnsObs(t *testing.T, result *core.LayerResult) *core.DNSObservations {
	t.Helper()
	obs, ok := result.Observations.(*core.DNSObservations)
	if !ok {
		t.Fatalf("observations type = %T, want *core.DNSObservations", result.Observations)
	}
	return obs
}

func TestDNSSuccess(t *testing.T) {
	layer := New(&testkit.FakeResolver{IPs: []string{"203.0.113.10"}})
	pctx := makeCtx(t, 5*time.Second)
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
	obs := dnsObs(t, result)
	if len(obs.Answers) != 1 || obs.Answers[0] != "203.0.113.10" {
		t.Errorf("answers = %v", obs.Answers)
	}
	// ResolvedIPs should be propagated to context.
	if len(pctx.ResolvedIPs) != 1 || pctx.ResolvedIPs[0] != "203.0.113.10" {
		t.Errorf("ResolvedIPs = %v", pctx.ResolvedIPs)
	}
}

func TestDNSNXDOMAIN(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "nx.example.com", IsNotFound: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

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
	result := layer.Probe(makeCtx(t, 5*time.Second))

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
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
}

func TestDNSGenericError(t *testing.T) {
	layer := New(&testkit.FakeResolver{Err: errors.New("something went wrong")})
	result := layer.Probe(makeCtx(t, 5*time.Second))

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
	result := layer.Probe(makeCtx(t, 5*time.Second))

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
	pctx := makeCtx(t, 5*time.Second)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	obs := dnsObs(t, result)
	if len(obs.Answers) != 3 {
		t.Errorf("answers len = %d, want 3", len(obs.Answers))
	}
	if len(pctx.ResolvedIPs) != 3 {
		t.Errorf("ResolvedIPs len = %d, want 3", len(pctx.ResolvedIPs))
	}
}

func TestDNSServfailBecomesErrorWithHint(t *testing.T) {
	dnsErr := &net.DNSError{Err: "server misbehaving", Name: "fail.example.com"}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
	obs := dnsObs(t, result)
	if obs.DNSErrorHint == nil || *obs.DNSErrorHint != "servfail" {
		t.Errorf("dns_error_hint = %v, want servfail", obs.DNSErrorHint)
	}
}

func TestDNSRefusedBecomesErrorWithHint(t *testing.T) {
	dnsErr := &net.DNSError{Err: "connection refused", Name: "refused.example.com"}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
	obs := dnsObs(t, result)
	if obs.DNSErrorHint == nil || *obs.DNSErrorHint != "refused" {
		t.Errorf("dns_error_hint = %v, want refused", obs.DNSErrorHint)
	}
}

func TestDNSNoAnswerBecomesErrorWithHint(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no answer from DNS server", Name: "empty.example.com"}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error code = %v, want DNS_ERROR", result.Error)
	}
	obs := dnsObs(t, result)
	if obs.DNSErrorHint == nil || *obs.DNSErrorHint != "no_answer" {
		t.Errorf("dns_error_hint = %v, want no_answer", obs.DNSErrorHint)
	}
}

func TestDNSNxdomainUnchanged(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "nx.example.com", IsNotFound: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_NXDOMAIN" {
		t.Errorf("error code = %v, want DNS_NXDOMAIN", result.Error)
	}
	obs := dnsObs(t, result)
	if obs.DNSErrorHint != nil {
		t.Errorf("dns_error_hint = %v, want nil", obs.DNSErrorHint)
	}
}

func TestDNSTimeoutUnchanged(t *testing.T) {
	dnsErr := &net.DNSError{Err: "timeout", Name: "slow.example.com", IsTimeout: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_TIMEOUT" {
		t.Errorf("error code = %v, want DNS_TIMEOUT", result.Error)
	}
	obs := dnsObs(t, result)
	if obs.DNSErrorHint != nil {
		t.Errorf("dns_error_hint = %v, want nil", obs.DNSErrorHint)
	}
}

func TestDNSObservationsHaveHintField(t *testing.T) {
	// On success, dns_error_hint should be nil.
	layer := New(&testkit.FakeResolver{IPs: []string{"203.0.113.10"}})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	obs := dnsObs(t, result)
	if obs.DNSErrorHint != nil {
		t.Errorf("dns_error_hint = %v, want nil on success", obs.DNSErrorHint)
	}
}

func TestDNSObservationsHaveTTLAndResolver(t *testing.T) {
	// On success, ttl and resolver_address should be nil placeholders.
	layer := New(&testkit.FakeResolver{IPs: []string{"203.0.113.10"}})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok", result.Status)
	}
	obs := dnsObs(t, result)
	if obs.TTL != nil {
		t.Errorf("ttl = %v, want nil", obs.TTL)
	}
	if obs.ResolverAddress != nil {
		t.Errorf("resolver_address = %v, want nil", obs.ResolverAddress)
	}
}

func TestDNSResolverAddressReturned(t *testing.T) {
	resolver := &testkit.FakeResolver{
		IPs:          []string{"203.0.113.10"},
		ResolverAddr: "192.168.1.1:53",
	}
	layer := New(resolver)
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok", result.Status)
	}
	obs := dnsObs(t, result)
	if obs.ResolverAddress == nil || *obs.ResolverAddress != "192.168.1.1:53" {
		t.Errorf("resolver_address = %v, want 192.168.1.1:53", obs.ResolverAddress)
	}
}

func TestDNSResolverAddressNilWhenEmpty(t *testing.T) {
	resolver := &testkit.FakeResolver{
		IPs: []string{"203.0.113.10"},
	}
	layer := New(resolver)
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok", result.Status)
	}
	obs := dnsObs(t, result)
	if obs.ResolverAddress != nil {
		t.Errorf("resolver_address = %v, want nil", obs.ResolverAddress)
	}
}

// simpleResolver implements Resolver but NOT ResolverWithAddress.
type simpleResolver struct {
	ips []string
	err error
}

func (r *simpleResolver) LookupHost(_ context.Context, _ string) ([]string, error) {
	return r.ips, r.err
}

func TestDNSResolverAddressNilWhenInterfaceNotImplemented(t *testing.T) {
	layer := New(&simpleResolver{ips: []string{"203.0.113.10"}})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok", result.Status)
	}
	obs := dnsObs(t, result)
	if obs.ResolverAddress != nil {
		t.Errorf("resolver_address = %v, want nil (interface not implemented)", obs.ResolverAddress)
	}
}

func TestIsLinkLocalNameserver(t *testing.T) {
	// Pure function test for Go issue #52839 workaround.
	// No network calls, no implementation detail access.
	tests := []struct {
		address string
		want    bool
	}{
		// IPv6 link-local with zone ID (the actual Go #52839 trigger)
		{"[fe80::1%en0]:53", true},
		{"[fe80::34d2:25ff:feef:c79e%en0]:53", true},
		// IPv6 link-local without zone ID
		{"[fe80::1]:53", true},
		// IPv4 link-local (169.254.0.0/16, common on WSL2/Docker)
		{"169.254.0.1:53", true},
		{"169.254.169.254:53", true},
		// Non-link-local addresses must NOT be skipped
		{"192.168.1.1:53", false},
		{"8.8.8.8:53", false},
		{"[2001:4860:4860::8888]:53", false},
		{"[::1]:53", false},
		// Malformed input
		{"not-an-address", false},
	}
	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := isLinkLocalNameserver(tt.address)
			if got != tt.want {
				t.Errorf("isLinkLocalNameserver(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestTrackingResolverSkipsLinkLocalAddress(t *testing.T) {
	// Integration test: verify the Dial hook in trackingResolver correctly
	// skips link-local and does not store it as resolver_address.
	// Accesses tr.inner.Dial directly because the Dial hook is the unit under test;
	// the pure detection logic is tested separately in TestIsLinkLocalNameserver.
	tr := newTrackingResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	// Dial with link-local address should return an error.
	_, err := tr.inner.Dial(ctx, "udp", "[fe80::1%en0]:53")
	if err == nil {
		t.Fatal("expected error for link-local nameserver, got nil")
	}
	if !strings.Contains(err.Error(), "link-local") {
		t.Errorf("error = %q, want substring 'link-local'", err.Error())
	}

	// resolver_address must remain empty (link-local was never stored).
	addr := tr.ResolverAddress()
	if addr != "" {
		t.Errorf("ResolverAddress() = %q, want empty for skipped link-local", addr)
	}
}

func TestDNSAllLinkLocalServersProduceDNSError(t *testing.T) {
	// When all nameservers are link-local, the resolver fails with a generic
	// DNS error. The Probe layer should classify this as DNS_ERROR with
	// resolver_address nil (no server was actually used).
	dnsErr := &net.DNSError{
		Err:  "dial udp [fe80::1%en0]:53: skipping link-local nameserver [fe80::1%en0]:53",
		Name: "example.com",
	}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "DNS_ERROR" {
		t.Errorf("error = %v, want DNS_ERROR", result.Error)
	}
	obs := dnsObs(t, result)
	if obs.ResolverAddress != nil {
		t.Errorf("resolver_address = %v, want nil when all servers are link-local", obs.ResolverAddress)
	}
}

func TestNewWithServerCreatesLayerWithCustomResolver(t *testing.T) {
	// NewWithServer should create a Layer using a trackingResolver
	// that dials the specified address.
	layer := NewWithServer("8.8.8.8:53")
	if layer == nil {
		t.Fatal("NewWithServer returned nil")
	}
	if layer.Name() != "dns" {
		t.Errorf("Name() = %q, want dns", layer.Name())
	}
	// The resolver should implement ResolverWithAddress.
	if _, ok := layer.resolver.(ResolverWithAddress); !ok {
		t.Error("NewWithServer resolver should implement ResolverWithAddress")
	}
}

func TestNewWithServerDefaultPort(t *testing.T) {
	// When only an IP is given without port, default port 53 should be used.
	layer := NewWithServer("8.8.8.8")
	if layer == nil {
		t.Fatal("NewWithServer returned nil")
	}
	if _, ok := layer.resolver.(ResolverWithAddress); !ok {
		t.Error("NewWithServer resolver should implement ResolverWithAddress")
	}
}

func TestDNSQueryNameOnFailure(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "bad.example.com", IsNotFound: true}
	layer := New(&testkit.FakeResolver{Err: dnsErr})
	result := layer.Probe(makeCtx(t, 5*time.Second))

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	obs := dnsObs(t, result)
	if obs.QueryName != "example.com" {
		t.Errorf("query_name = %q, want example.com", obs.QueryName)
	}
}
