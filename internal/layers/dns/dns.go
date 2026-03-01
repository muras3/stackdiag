package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// ResolverWithAddress is an optional interface for resolvers that can report
// the DNS resolver address used.
type ResolverWithAddress interface {
	ResolverAddress() string
}

// Layer performs DNS resolution.
type Layer struct {
	resolver Resolver
}

// New creates a DNS Layer with the given resolver.
func New(resolver Resolver) *Layer {
	return &Layer{resolver: resolver}
}

// NewDefault creates a DNS Layer using the system resolver.
func NewDefault() *Layer {
	return &Layer{resolver: newTrackingResolver()}
}

// NewWithServer creates a DNS Layer using a custom resolver that dials the specified address.
// addr can be "ip:port" or just "ip" (default port 53).
func NewWithServer(addr string) *Layer {
	// Normalize: add default port if not specified.
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}
	return &Layer{resolver: newTrackingResolverWithServer(addr)}
}

// trackingResolver wraps net.Resolver with a Dial hook to capture resolver address.
type trackingResolver struct {
	inner   *net.Resolver
	mu      sync.Mutex
	address string
}

// newTrackingResolver creates a resolver that captures the DNS server address.
// PreferGo:true forces the pure Go DNS resolver over cgo, which:
// - Enables the Dial hook (cgo resolver doesn't call it)
// - May bypass platform-specific resolution (e.g., macOS mDNSResponder)
// This trade-off is acceptable: resolver_address capture requires the hook,
// and the pure Go resolver handles /etc/resolv.conf correctly on all targets.
//
// The Dial hook skips link-local nameservers (IPv6 fe80::/10, IPv4 169.254.0.0/16)
// to work around Go issue #52839: the pure Go resolver hangs on macOS when
// /etc/resolv.conf contains a link-local nameserver with a zone ID (e.g. fe80::1%en0).
// Returning an error causes Go's resolver to immediately try the next nameserver.
func newTrackingResolver() *trackingResolver {
	tr := &trackingResolver{}
	tr.inner = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if isLinkLocalNameserver(address) {
				return nil, fmt.Errorf("skipping link-local nameserver %s", address)
			}
			tr.mu.Lock()
			tr.address = address
			tr.mu.Unlock()
			var d net.Dialer
			return d.DialContext(ctx, network, address)
		},
	}
	return tr
}

// isLinkLocalNameserver reports whether address (host:port) is a link-local
// unicast address. Handles IPv6 zone IDs (e.g. "fe80::1%en0") which
// net.ParseIP cannot parse directly.
// Note: returns false if SplitHostPort fails. Go's net.Resolver always passes
// host:port format to the Dial hook, so this is safe.
func isLinkLocalNameserver(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	// Strip zone ID (e.g. "%en0") before ParseIP; ParseIP rejects zones.
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLinkLocalUnicast()
}

// newTrackingResolverWithServer creates a resolver that always dials the specified server.
// It still captures the resolver address and skips link-local nameservers.
func newTrackingResolverWithServer(server string) *trackingResolver {
	tr := &trackingResolver{}
	tr.inner = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if isLinkLocalNameserver(server) {
				return nil, fmt.Errorf("skipping link-local nameserver %s", server)
			}
			tr.mu.Lock()
			tr.address = server
			tr.mu.Unlock()
			var d net.Dialer
			return d.DialContext(ctx, network, server)
		},
	}
	return tr
}

func (tr *trackingResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return tr.inner.LookupHost(ctx, host)
}

func (tr *trackingResolver) ResolverAddress() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.address
}

func (l *Layer) Name() string { return "dns" }

func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	start := time.Now()
	ips, err := l.resolver.LookupHost(pctx.Context, pctx.Target.Host)
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	// Check if resolver reports its address.
	var resolverAddr any
	if ra, ok := l.resolver.(ResolverWithAddress); ok {
		if addr := ra.ResolverAddress(); addr != "" {
			resolverAddr = addr
		}
	}

	if err != nil {
		probeErr, hint := classifyDNSError(err)
		obs := map[string]any{
			"query_name":       pctx.Target.Host,
			"dns_error_hint":   hint,
			"ttl":              nil,
			"resolver_address": resolverAddr,
		}
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: obs,
			Error:        probeErr,
		}
	}

	// Propagate resolved IPs to context for TCP layer.
	pctx.ResolvedIPs = ips

	return &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: durationMS,
		Observations: map[string]any{
			"query_name":       pctx.Target.Host,
			"answers":          ips,
			"dns_error_hint":   nil,
			"ttl":              nil,
			"resolver_address": resolverAddr,
		},
		Error: nil,
	}
}

func classifyDNSError(err error) (*core.ProbeError, any) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &core.ProbeError{Code: "DNS_TIMEOUT", Message: "DNS resolution timed out"}, nil
	}

	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		return &core.ProbeError{Code: "DNS_ERROR", Message: "DNS resolution failed"}, nil
	}

	if dnsErr.IsNotFound {
		return &core.ProbeError{Code: "DNS_NXDOMAIN", Message: "domain not found"}, nil
	}
	if dnsErr.IsTimeout {
		return &core.ProbeError{Code: "DNS_TIMEOUT", Message: "DNS resolution timed out"}, nil
	}

	// Best-effort hint (not contract code)
	hint := inferDNSHint(dnsErr)
	return &core.ProbeError{Code: "DNS_ERROR", Message: "DNS resolution failed"}, hint
}

func inferDNSHint(dnsErr *net.DNSError) any {
	msg := dnsErr.Err
	switch {
	case strings.Contains(msg, "server misbehaving"):
		return "servfail"
	case strings.Contains(msg, "refused"):
		return "refused"
	case strings.Contains(msg, "no answer"):
		return "no_answer"
	default:
		return nil
	}
}
