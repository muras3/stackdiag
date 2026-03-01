package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

// expiringThresholdDays defines the number of days before expiry at which
// a certificate is considered "expiring soon".
const expiringThresholdDays = 30

// Handshaker abstracts the TLS handshake for testability.
type Handshaker interface {
	Handshake(ctx context.Context, addr string, cfg *tls.Config) (*tls.ConnectionState, float64, error)
}

// DefaultHandshaker performs a real TLS handshake: dial TCP, then TLS.
type DefaultHandshaker struct{}

func (h *DefaultHandshaker) Handshake(ctx context.Context, addr string, cfg *tls.Config) (*tls.ConnectionState, float64, error) {
	dialer := &net.Dialer{}
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, 0, fmt.Errorf("tcp dial: %w", err)
	}

	start := time.Now()
	tlsConn := tls.Client(tcpConn, cfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		durationMS := float64(time.Since(start).Microseconds()) / 1000.0
		tcpConn.Close()
		return nil, durationMS, err
	}
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	state := tlsConn.ConnectionState()
	tlsConn.Close()
	return &state, durationMS, nil
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
// It uses a 2-phase approach: first handshake with InsecureSkipVerify=true
// to always collect connection state, then manual cert validation.
func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	addr := buildAddr(pctx)
	cfg := &tls.Config{
		ServerName:         pctx.Target.Host,
		InsecureSkipVerify: true, // Phase 1: always skip verify; validate manually below
		MinVersion:         tls.VersionTLS12,
	}

	state, durationMS, err := l.handshaker.Handshake(pctx.Context, addr, cfg)
	if err != nil {
		// Handshake itself failed (protocol error, timeout, etc.) — no state available
		result := &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: map[string]any{},
			Error:        classifyTLSError(err),
		}
		if pctx.TLSScan {
			l.performTLSScan(pctx, addr, result)
		}
		return result
	}

	// Start with verified=false; set to true only after successful Phase 2 validation.
	verified := false
	obs := buildObservations(state, pctx.Target.Host, verified)

	// Phase 2: Manual cert validation (unless insecure mode)
	if !pctx.Insecure {
		if len(state.PeerCertificates) == 0 {
			return &core.LayerResult{
				Status:       core.StatusFail,
				DurationMS:   durationMS,
				Observations: obs,
				Error:        &core.ProbeError{Code: "TLS_NO_CERTIFICATES", Message: "server presented no certificates"},
			}
		}

		leaf := state.PeerCertificates[0]

		verifyOpts := x509.VerifyOptions{
			DNSName:       pctx.Target.Host,
			Intermediates: x509.NewCertPool(),
			Roots:         cfg.RootCAs, // nil = system roots; test handshakers inject custom roots
		}
		for _, cert := range state.PeerCertificates[1:] {
			verifyOpts.Intermediates.AddCert(cert)
		}

		if _, verifyErr := leaf.Verify(verifyOpts); verifyErr != nil {
			result := &core.LayerResult{
				Status:       core.StatusFail,
				DurationMS:   durationMS,
				Observations: obs,
				Error:        classifyTLSError(verifyErr),
			}
			if pctx.TLSScan {
				l.performTLSScan(pctx, addr, result)
			}
			return result
		}

		// Verification passed — update cert_verified in observations
		verified = true
		obs["cert_verified"] = true
	}

	// Check certificate expiry, not-yet-valid, and hostname even on successful handshake.
	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		now := time.Now()
		timeUntilExpiry := time.Until(leaf.NotAfter)
		daysUntilExpiry := int(timeUntilExpiry.Hours() / 24)

		// Check not-yet-valid: NotBefore is in the future.
		if now.Before(leaf.NotBefore) {
			if pctx.Insecure {
				return &core.LayerResult{
					Status:       core.StatusWarn,
					DurationMS:   durationMS,
					Observations: obs,
					Error:        &core.ProbeError{Code: "TLS_CERT_NOT_YET_VALID", Message: "certificate is not yet valid"},
				}
			}
			return &core.LayerResult{
				Status:       core.StatusFail,
				DurationMS:   durationMS,
				Observations: obs,
				Error:        &core.ProbeError{Code: "TLS_CERT_NOT_YET_VALID", Message: "certificate is not yet valid"},
			}
		}

		// Use raw duration for expired check to avoid truncation-to-zero
		// when cert expired less than 24h ago (int(-0.5) == 0 in Go).
		if timeUntilExpiry < 0 {
			if pctx.Insecure {
				return &core.LayerResult{
					Status:       core.StatusWarn,
					DurationMS:   durationMS,
					Observations: obs,
					Error:        &core.ProbeError{Code: "TLS_CERT_EXPIRED", Message: "certificate has expired"},
				}
			}
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

	result := &core.LayerResult{
		Status:       core.StatusOK,
		DurationMS:   durationMS,
		Observations: obs,
		Error:        nil,
	}

	if pctx.TLSScan {
		l.performTLSScan(pctx, addr, result)
	}

	return result
}

// buildAddr constructs the dial address from the stackdiag context.
func buildAddr(pctx *core.ProbeContext) string {
	host := pctx.Target.Host
	if len(pctx.ResolvedIPs) > 0 {
		host = pctx.ResolvedIPs[0]
	}
	return net.JoinHostPort(host, strconv.Itoa(pctx.Target.Port))
}

// buildObservations creates the observations map from the TLS connection state.
// The verified parameter indicates whether the certificate was validated through
// normal verification (true) or retrieved via InsecureSkipVerify (false).
func buildObservations(state *tls.ConnectionState, serverName string, verified bool) map[string]any {
	obs := map[string]any{
		"version":      tlsVersionString(state.Version),
		"cipher_suite": tls.CipherSuiteName(state.CipherSuite),
	}

	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		daysUntilExpiry := int(time.Until(leaf.NotAfter).Hours() / 24)
		obs["cert_days_until_expiry"] = daysUntilExpiry
		obs["cert_hostname_match"] = certMatchesHost(leaf, serverName)

		// v0.2 expanded observations
		obs["cert_verified"] = verified
		obs["cert_subject"] = leaf.Subject.CommonName
		san := leaf.DNSNames
		if san == nil {
			san = []string{}
		}
		obs["cert_san"] = san
		obs["cert_issuer"] = leaf.Issuer.CommonName
		obs["cert_not_after"] = leaf.NotAfter.UTC().Format(time.RFC3339)
		obs["cert_not_before"] = leaf.NotBefore.UTC().Format(time.RFC3339)

		// cert_chain: summary of each certificate in the chain
		chain := make([]map[string]string, len(state.PeerCertificates))
		for i, cert := range state.PeerCertificates {
			chain[i] = map[string]string{
				"subject":   cert.Subject.CommonName,
				"issuer":    cert.Issuer.CommonName,
				"not_after": cert.NotAfter.UTC().Format(time.RFC3339),
			}
		}
		obs["cert_chain"] = chain
	}

	return obs
}

// certMatchesHost checks if the certificate is valid for the given hostname.
func certMatchesHost(cert *x509.Certificate, host string) bool {
	return cert.VerifyHostname(host) == nil
}

// classifyTLSError converts a TLS error into a structured ProbeError.
// Uses Go type assertions for x509 errors where possible for reliability.
func classifyTLSError(err error) *core.ProbeError {
	msg := err.Error()

	// Type-based classification (preferred — reliable across Go versions).
	// Note: x509.HostnameError and x509.UnknownAuthorityError implement error
	// on the value receiver, so errors.As targets must be value types.
	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return &core.ProbeError{Code: "TLS_HOSTNAME_MISMATCH", Message: msg}
	}

	var unknownAuth x509.UnknownAuthorityError
	if errors.As(err, &unknownAuth) {
		return &core.ProbeError{Code: "TLS_UNTRUSTED_CHAIN", Message: msg}
	}

	// String-based classification (for errors without exported types)
	switch {
	case isCertNotYetValid(msg):
		return &core.ProbeError{Code: "TLS_CERT_NOT_YET_VALID", Message: msg}
	case isCertExpired(msg):
		return &core.ProbeError{Code: "TLS_CERT_EXPIRED", Message: msg}
	case isHandshakeTimeout(err):
		return &core.ProbeError{Code: "TLS_HANDSHAKE_TIMEOUT", Message: msg}
	case isProtocolError(msg):
		// TLS_PROTOCOL_ERROR is a best-effort classification (not type-based)
		return &core.ProbeError{Code: "TLS_PROTOCOL_ERROR", Message: msg}
	default:
		return &core.ProbeError{Code: "TLS_ERROR", Message: msg}
	}
}

func isCertNotYetValid(msg string) bool {
	// Go's x509 uses "certificate has expired or is not yet valid" for both cases.
	// The disambiguator is "is before" (not-yet-valid) vs "is after" (expired).
	return strings.Contains(msg, "is not yet valid") && strings.Contains(msg, "is before")
}

func isCertExpired(msg string) bool {
	return strings.Contains(msg, "certificate has expired") ||
		strings.Contains(msg, "x509: certificate has expired or is not yet valid")
}

func isProtocolError(msg string) bool {
	return strings.Contains(msg, "protocol version") ||
		strings.Contains(msg, "oversized record") ||
		strings.Contains(msg, "tls: alert") ||
		strings.Contains(msg, "first record does not look like a TLS handshake")
}

func isHandshakeTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

// tlsScanVersions defines the TLS versions to probe during a scan, in order.
var tlsScanVersions = []uint16{
	tls.VersionTLS10,
	tls.VersionTLS11,
	tls.VersionTLS12,
	tls.VersionTLS13,
}

// deprecatedVersions are TLS versions considered deprecated.
var deprecatedVersions = map[uint16]bool{
	tls.VersionTLS10: true,
	tls.VersionTLS11: true,
}

// performTLSScan probes each TLS version individually and adds tls_scan to observations.
// If deprecated versions are found and the current status is ok, it upgrades to warn.
func (l *Layer) performTLSScan(pctx *core.ProbeContext, addr string, result *core.LayerResult) {
	attempts := make([]map[string]any, 0, len(tlsScanVersions))
	var supportedVersions []string
	var deprecatedEnabled []string

	for _, ver := range tlsScanVersions {
		scanCfg := &tls.Config{
			ServerName:         pctx.Target.Host,
			InsecureSkipVerify: true, // scan only tests protocol support
			MinVersion:         ver,
			MaxVersion:         ver,
		}

		_, dur, err := l.handshaker.Handshake(pctx.Context, addr, scanCfg)

		attempt := map[string]any{
			"version":     tlsVersionString(ver),
			"duration_ms": dur,
		}

		if err != nil {
			attempt["supported"] = false
			attempt["error"] = map[string]any{
				"code":    classifyTLSError(err).Code,
				"message": err.Error(),
			}
		} else {
			attempt["supported"] = true
			attempt["error"] = nil
			supportedVersions = append(supportedVersions, tlsVersionString(ver))
			if deprecatedVersions[ver] {
				deprecatedEnabled = append(deprecatedEnabled, tlsVersionString(ver))
			}
		}

		attempts = append(attempts, attempt)
	}

	if supportedVersions == nil {
		supportedVersions = []string{}
	}
	if deprecatedEnabled == nil {
		deprecatedEnabled = []string{}
	}

	tlsScanData := map[string]any{
		"performed":                   true,
		"attempts":                    attempts,
		"supported_versions":          supportedVersions,
		"deprecated_versions_enabled": deprecatedEnabled,
	}
	if m, ok := result.Observations.(map[string]any); ok {
		m["tls_scan"] = tlsScanData
	}

	// If deprecated versions found and normal probe was ok → warn
	if len(deprecatedEnabled) > 0 && result.Status == core.StatusOK {
		result.Status = core.StatusWarn
		result.Error = &core.ProbeError{
			Code:    "TLS_DEPRECATED_VERSION_ENABLED",
			Message: fmt.Sprintf("deprecated TLS versions enabled: %s", strings.Join(deprecatedEnabled, ", ")),
		}
	}
}

// tlsVersionString converts a TLS version constant to a human-readable string.
func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLSv1.0"
	case tls.VersionTLS11:
		return "TLSv1.1"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS13:
		return "TLSv1.3"
	default:
		return fmt.Sprintf("unknown (0x%04x)", version)
	}
}
