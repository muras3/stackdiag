package dns

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/muras3/probe/internal/core"
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
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: map[string]any{"query_name": pctx.Target.Host},
			Error:        classifyDNSError(err),
		}
	}

	// Propagate resolved IPs to context for TCP layer.
	pctx.ResolvedIPs = ips

	return &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: durationMS,
		Observations: map[string]any{
			"query_name": pctx.Target.Host,
			"answers":    ips,
		},
		Error: nil,
	}
}

func classifyDNSError(err error) *core.ProbeError {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &core.ProbeError{Code: "DNS_TIMEOUT", Message: err.Error()}
	}

	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		return &core.ProbeError{Code: "DNS_ERROR", Message: err.Error()}
	}

	switch {
	case dnsErr.IsNotFound:
		return &core.ProbeError{Code: "DNS_NXDOMAIN", Message: dnsErr.Error()}
	case dnsErr.IsTimeout:
		return &core.ProbeError{Code: "DNS_TIMEOUT", Message: dnsErr.Error()}
	default:
		return &core.ProbeError{Code: "DNS_ERROR", Message: dnsErr.Error()}
	}
}
