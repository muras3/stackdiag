package dns

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
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
	return &Layer{resolver: net.DefaultResolver}
}

func (l *Layer) Name() string { return "dns" }

func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	start := time.Now()
	ips, err := l.resolver.LookupHost(pctx.Context, pctx.Target.Host)
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		probeErr, hint := classifyDNSError(err)
		obs := map[string]any{
			"query_name":       pctx.Target.Host,
			"dns_error_hint":   hint,
			"ttl":              nil,
			"resolver_address": nil,
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
			"resolver_address": nil,
		},
		Error: nil,
	}
}

func classifyDNSError(err error) (*core.ProbeError, any) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &core.ProbeError{Code: "DNS_TIMEOUT", Message: err.Error()}, nil
	}

	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		return &core.ProbeError{Code: "DNS_ERROR", Message: err.Error()}, nil
	}

	if dnsErr.IsNotFound {
		return &core.ProbeError{Code: "DNS_NXDOMAIN", Message: err.Error()}, nil
	}
	if dnsErr.IsTimeout {
		return &core.ProbeError{Code: "DNS_TIMEOUT", Message: err.Error()}, nil
	}

	// Best-effort hint (not contract code)
	hint := inferDNSHint(dnsErr)
	return &core.ProbeError{Code: "DNS_ERROR", Message: err.Error()}, hint
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
