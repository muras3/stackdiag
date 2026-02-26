package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/muras3/probe/internal/core"
)

// expiringThresholdDays defines the number of days before expiry at which
// a certificate is considered "expiring soon".
const expiringThresholdDays = 30

// Handshaker abstracts the TLS handshake for testability.
type Handshaker interface {
	Handshake(ctx context.Context, addr string, cfg *tls.Config) (*tls.ConnectionState, error)
}

// DefaultHandshaker performs a real TLS handshake: dial TCP, then TLS.
type DefaultHandshaker struct{}

func (h *DefaultHandshaker) Handshake(ctx context.Context, addr string, cfg *tls.Config) (*tls.ConnectionState, error) {
	dialer := &net.Dialer{}
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}

	tlsConn := tls.Client(tcpConn, cfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, err
	}

	state := tlsConn.ConnectionState()
	tlsConn.Close()
	return &state, nil
}

// Layer performs TLS handshake diagnostics.
type Layer struct {
	handshaker Handshaker
}

// New creates a TLS Layer with the given handshaker.
func New(handshaker Handshaker) *Layer {
	return &Layer{handshaker: handshaker}
}

// NewDefault creates a TLS Layer using the default handshaker.
func NewDefault() *Layer {
	return &Layer{handshaker: &DefaultHandshaker{}}
}

// Name returns the layer name.
func (l *Layer) Name() string { return "tls" }

// Probe performs the TLS handshake and inspects the certificate.
func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	addr := buildAddr(pctx)
	cfg := &tls.Config{
		ServerName:         pctx.Target.Host,
		InsecureSkipVerify: pctx.Insecure,
	}

	start := time.Now()
	state, err := l.handshaker.Handshake(pctx.Context, addr, cfg)
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: map[string]any{},
			Error:        classifyTLSError(err),
		}
	}

	obs := buildObservations(state, pctx.Target.Host)

	// Check certificate expiry and hostname even on successful handshake.
	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		daysUntilExpiry := int(time.Until(leaf.NotAfter).Hours() / 24)

		if daysUntilExpiry < 0 {
			return &core.LayerResult{
				Status:       core.StatusFail,
				DurationMS:   durationMS,
				Observations: obs,
				Error:        &core.ProbeError{Code: "TLS_CERT_EXPIRED", Message: "certificate has expired"},
			}
		}

		if daysUntilExpiry < expiringThresholdDays {
			return &core.LayerResult{
				Status:       core.StatusWarn,
				DurationMS:   durationMS,
				Observations: obs,
				Error: &core.ProbeError{
					Code:    "TLS_CERT_EXPIRING_SOON",
					Message: fmt.Sprintf("certificate expires in %d days", daysUntilExpiry),
				},
			}
		}
	}

	return &core.LayerResult{
		Status:       core.StatusOK,
		DurationMS:   durationMS,
		Observations: obs,
		Error:        nil,
	}
}

// buildAddr constructs the dial address from the probe context.
func buildAddr(pctx *core.ProbeContext) string {
	host := pctx.Target.Host
	if len(pctx.ResolvedIPs) > 0 {
		host = pctx.ResolvedIPs[0]
	}
	return fmt.Sprintf("%s:%d", host, pctx.Target.Port)
}

// buildObservations creates the observations map from the TLS connection state.
func buildObservations(state *tls.ConnectionState, serverName string) map[string]any {
	obs := map[string]any{
		"version":      tlsVersionString(state.Version),
		"cipher_suite": tls.CipherSuiteName(state.CipherSuite),
	}

	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		daysUntilExpiry := int(time.Until(leaf.NotAfter).Hours() / 24)
		obs["cert_days_until_expiry"] = daysUntilExpiry
		obs["cert_hostname_match"] = certMatchesHost(leaf, serverName)
	}

	return obs
}

// certMatchesHost checks if the certificate is valid for the given hostname.
func certMatchesHost(cert *x509.Certificate, host string) bool {
	return cert.VerifyHostname(host) == nil
}

// classifyTLSError converts a TLS error into a structured ProbeError.
func classifyTLSError(err error) *core.ProbeError {
	msg := err.Error()

	switch {
	case isCertExpired(msg):
		return &core.ProbeError{Code: "TLS_CERT_EXPIRED", Message: msg}
	case isHostnameMismatch(msg):
		return &core.ProbeError{Code: "TLS_HOSTNAME_MISMATCH", Message: msg}
	case isUntrustedChain(msg):
		return &core.ProbeError{Code: "TLS_UNTRUSTED_CHAIN", Message: msg}
	case isHandshakeTimeout(err):
		return &core.ProbeError{Code: "TLS_HANDSHAKE_TIMEOUT", Message: msg}
	default:
		return &core.ProbeError{Code: "TLS_ERROR", Message: msg}
	}
}

func isCertExpired(msg string) bool {
	return strings.Contains(msg, "certificate has expired") ||
		strings.Contains(msg, "x509: certificate has expired or is not yet valid")
}

func isHostnameMismatch(msg string) bool {
	return strings.Contains(msg, "x509: certificate is valid for") ||
		strings.Contains(msg, "doesn't match") ||
		strings.Contains(msg, "certificate is not valid for")
}

func isUntrustedChain(msg string) bool {
	return strings.Contains(msg, "x509: certificate signed by unknown authority") ||
		strings.Contains(msg, "unknown authority")
}

func isHandshakeTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	return false
}

// tlsVersionString converts a TLS version constant to a human-readable string.
func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("unknown (0x%04x)", version)
	}
}
