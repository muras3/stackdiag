package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

// --- Helper: generate a self-signed certificate with configurable properties ---

type certOpts struct {
	hosts     []string // SANs (DNS names)
	notBefore time.Time
	notAfter  time.Time
}

func generateCert(t *testing.T, opts certOpts) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"stackdiag-test"}},
		NotBefore:             opts.notBefore,
		NotAfter:              opts.notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              opts.hosts,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	parsedCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(parsedCert)

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return tlsCert, pool
}

// startTLSServer starts a local TLS listener and returns the address and a cleanup func.
// The server completes the TLS handshake before closing each connection.
func startTLSServer(t *testing.T, cert tls.Certificate) (string, func()) {
	t.Helper()

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			// Complete the TLS handshake on the server side before closing.
			// tls.Listen wraps accepted connections as *tls.Conn; the handshake
			// is lazy and must be triggered explicitly or via Read/Write.
			if tlsConn, ok := conn.(*tls.Conn); ok {
				_ = tlsConn.Handshake()
			}
			conn.Close()
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func makePctx(host string, port int, resolvedIP string, insecure bool) *core.ProbeContext {
	return &core.ProbeContext{
		Context:     context.Background(),
		Target:      core.Target{Host: host, Port: port, Scheme: "https"},
		ResolvedIPs: []string{resolvedIP},
		Insecure:    insecure,
	}
}

// --- Fake Handshaker for unit tests ---

type fakeHandshaker struct {
	state      *tls.ConnectionState
	durationMS float64
	err        error
}

func (f *fakeHandshaker) Handshake(_ context.Context, _ string, _ *tls.Config) (*tls.ConnectionState, float64, error) {
	return f.state, f.durationMS, f.err
}

// =============================================================================
// Test: Name()
// =============================================================================

func TestTLSName(t *testing.T) {
	layer := NewDefault()
	if layer.Name() != "tls" {
		t.Errorf("Name() = %q, want tls", layer.Name())
	}
}

// =============================================================================
// Test 1: Success - local TLS server with valid cert, status=ok
// =============================================================================

func TestTLSSuccess(t *testing.T) {
	now := time.Now()
	cert, pool := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(365 * 24 * time.Hour),
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	handshaker := &realHandshakerWithRoots{roots: pool}
	layer := New(handshaker)

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok; error = %v", result.Status, result.Error)
	}
	if result.DurationMS < 0 {
		t.Error("expected non-negative duration")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	// Check observations.
	obs, ok := result.Observations.(*core.TLSObservations)
	if !ok {
		t.Fatalf("observations type = %T, want *core.TLSObservations", result.Observations)
	}
	if obs.Version == "" {
		t.Error("missing 'version' observation")
	}
	if obs.CipherSuite == "" {
		t.Error("missing 'cipher_suite' observation")
	}
	if obs.CertDaysUntilExpiry == nil {
		t.Error("missing 'cert_days_until_expiry' observation")
	} else if *obs.CertDaysUntilExpiry < 300 {
		t.Errorf("cert_days_until_expiry = %d, expected > 300", *obs.CertDaysUntilExpiry)
	}
	if obs.CertHostnameMatch == nil {
		t.Error("missing 'cert_hostname_match' observation")
	} else if !*obs.CertHostnameMatch {
		t.Errorf("cert_hostname_match = %v, want true", *obs.CertHostnameMatch)
	}
}

// =============================================================================
// Test 2: Expired cert — status=fail, code=TLS_CERT_EXPIRED
// =============================================================================

func TestTLSExpiredCert(t *testing.T) {
	now := time.Now()
	cert, pool := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(-48 * time.Hour),
		notAfter:  now.Add(-1 * time.Hour), // expired
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	handshaker := &realHandshakerWithRoots{roots: pool}
	layer := New(handshaker)

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_EXPIRED" {
		t.Errorf("error = %v, want TLS_CERT_EXPIRED", result.Error)
	}
}

// =============================================================================
// Test 3: Hostname mismatch — status=fail, code=TLS_HOSTNAME_MISMATCH
// =============================================================================

func TestTLSHostnameMismatch(t *testing.T) {
	now := time.Now()
	cert, pool := generateCert(t, certOpts{
		hosts:     []string{"other.example.com"}, // SAN does not match "localhost"
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(365 * 24 * time.Hour),
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	handshaker := &realHandshakerWithRoots{roots: pool}
	layer := New(handshaker)

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_HOSTNAME_MISMATCH" {
		t.Errorf("error = %v, want TLS_HOSTNAME_MISMATCH", result.Error)
	}
}

// =============================================================================
// Test 4: Cert expiring soon (<30 days) — status=warn, code=TLS_CERT_EXPIRING_SOON
// =============================================================================

func TestTLSCertExpiringSoon(t *testing.T) {
	now := time.Now()
	cert, pool := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(15 * 24 * time.Hour), // expires in 15 days
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	handshaker := &realHandshakerWithRoots{roots: pool}
	layer := New(handshaker)

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusWarn {
		t.Errorf("status = %q, want warn; error = %v", result.Status, result.Error)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_EXPIRING_SOON" {
		t.Errorf("error = %v, want TLS_CERT_EXPIRING_SOON", result.Error)
	}

	tlsObs, ok := result.Observations.(*core.TLSObservations)
	if !ok {
		t.Fatalf("observations type = %T, want *core.TLSObservations", result.Observations)
	}
	if tlsObs.CertDaysUntilExpiry == nil {
		t.Fatal("missing cert_days_until_expiry observation")
	}
	if d := *tlsObs.CertDaysUntilExpiry; d < 10 || d > 20 {
		t.Errorf("cert_days_until_expiry = %d, want 14-16", d)
	}
}

// =============================================================================
// Test 5: Insecure mode — invalid cert but status=ok
// =============================================================================

func TestTLSInsecureMode(t *testing.T) {
	now := time.Now()
	cert, _ := generateCert(t, certOpts{
		hosts:     []string{"other.example.com"}, // hostname mismatch
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(365 * 24 * time.Hour),
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	// Use default handshaker (no custom root pool — will not trust self-signed cert),
	// but insecure=true should skip verification.
	layer := NewDefault()

	pctx := makePctx("localhost", port, host, true)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok (insecure mode); error = %v", result.Status, result.Error)
	}
	if result.Error != nil {
		t.Errorf("unexpected error in insecure mode: %v", result.Error)
	}
}

// =============================================================================
// realHandshakerWithRoots is a test helper that injects a custom root CA pool
// into the TLS config before performing the handshake, so self-signed certs
// are trusted by the handshake.
// =============================================================================

type realHandshakerWithRoots struct {
	roots *x509.CertPool
}

func (h *realHandshakerWithRoots) Handshake(ctx context.Context, addr string, cfg *tls.Config) (*tls.ConnectionState, float64, error) {
	cfg.RootCAs = h.roots
	return (&DefaultHandshaker{}).Handshake(ctx, addr, cfg)
}

type fakeTimeoutError struct {
	wrapped error
}

func (e *fakeTimeoutError) Error() string { return "wrapped timeout" }
func (e *fakeTimeoutError) Unwrap() error { return e.wrapped }

func TestTLSUntrustedChainViaFakeHandshaker(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 4.5,
		err:        x509.UnknownAuthorityError{Cert: &x509.Certificate{}},
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_UNTRUSTED_CHAIN" {
		t.Fatalf("error = %v, want TLS_UNTRUSTED_CHAIN", result.Error)
	}
	if result.DurationMS != 4.5 {
		t.Fatalf("duration_ms = %v, want 4.5", result.DurationMS)
	}
}

func TestTLSContextDeadlineExceededViaFakeHandshaker(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 5.0,
		err:        context.DeadlineExceeded,
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_HANDSHAKE_TIMEOUT" {
		t.Fatalf("error = %v, want TLS_HANDSHAKE_TIMEOUT", result.Error)
	}
}

func TestTLSHandshakeTimeoutViaFakeHandshaker(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 12.3,
		err: &fakeTimeoutError{
			wrapped: &net.DNSError{Err: "i/o timeout", IsTimeout: true},
		},
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_HANDSHAKE_TIMEOUT" {
		t.Fatalf("error = %v, want TLS_HANDSHAKE_TIMEOUT", result.Error)
	}
	if result.DurationMS != 12.3 {
		t.Fatalf("duration_ms = %v, want 12.3", result.DurationMS)
	}
}

// =============================================================================
// Test: Insecure mode with expired cert — should bypass expiry (status=warn)
// =============================================================================

func TestTLSInsecureModeExpiredCert(t *testing.T) {
	now := time.Now()
	cert, _ := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(-48 * time.Hour),
		notAfter:  now.Add(-1 * time.Hour), // expired
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	// insecure=true should bypass expired cert
	layer := NewDefault()
	pctx := makePctx("localhost", port, host, true)
	result := layer.Probe(pctx)

	if result.Status == core.StatusFail {
		t.Errorf("status = %q, want ok or warn (insecure mode should bypass expired cert); error = %v", result.Status, result.Error)
	}
}

// =============================================================================
// Test: Insecure mode with expired cert via fake handshaker — status=warn
// =============================================================================

func TestTLSInsecureModeExpiredCertViaFake(t *testing.T) {
	now := time.Now()
	expiredAt := now.Add(-1 * time.Hour)

	leaf := &x509.Certificate{
		NotAfter: expiredAt,
		DNSNames: []string{"localhost"},
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	layer := New(&fakeHandshaker{
		state:      state,
		durationMS: 3.0,
	})

	pctx := makePctx("localhost", 443, "127.0.0.1", true) // insecure=true
	result := layer.Probe(pctx)

	if result.Status != core.StatusWarn {
		t.Errorf("status = %q, want warn (insecure mode with expired cert)", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_EXPIRED" {
		t.Errorf("error = %v, want TLS_CERT_EXPIRED warning", result.Error)
	}
}

func TestTLSProtocolVersionError(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 3.0,
		err:        errors.New("tls: protocol version not supported"),
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_PROTOCOL_ERROR" {
		t.Fatalf("error = %v, want TLS_PROTOCOL_ERROR", result.Error)
	}
}

func TestTLSRecordError(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 3.0,
		err:        errors.New("tls: oversized record received with length 20527"),
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_PROTOCOL_ERROR" {
		t.Fatalf("error = %v, want TLS_PROTOCOL_ERROR", result.Error)
	}
}

func TestTLSAlertError(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 3.0,
		err:        errors.New("tls: alert(40): handshake failure"),
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_PROTOCOL_ERROR" {
		t.Fatalf("error = %v, want TLS_PROTOCOL_ERROR", result.Error)
	}
}

func TestTLSGenericError(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 2.0,
		err:        errors.New("some unknown TLS error"),
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_ERROR" {
		t.Fatalf("error = %v, want TLS_ERROR", result.Error)
	}
}

// Test: Cert expired less than 24h ago — must be EXPIRED, not EXPIRING_SOON.
// Regression test for int truncation: int(-0.04) == 0 in Go.
// Uses insecure=true to exercise the post-validation expiry check path
// (with 2-phase handshake, insecure=false catches expired via x509.Verify).
func TestTLSCertExpiredLessThan24h(t *testing.T) {
	now := time.Now()
	expiredAt := now.Add(-1 * time.Hour) // expired 1 hour ago

	leaf := &x509.Certificate{
		NotAfter: expiredAt,
		DNSNames: []string{"localhost"},
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	layer := New(&fakeHandshaker{
		state:      state,
		durationMS: 3.0,
	})

	// insecure=true to skip manual cert verification (fake cert can't pass x509.Verify)
	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", true))
	if result.Status != core.StatusWarn {
		t.Fatalf("status = %q, want warn (insecure mode with expired cert)", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_EXPIRED" {
		t.Fatalf("error = %v, want TLS_CERT_EXPIRED", result.Error)
	}
}

// =============================================================================
// Test: Not-yet-valid cert via classifyTLSError — status=fail, code=TLS_CERT_NOT_YET_VALID
// =============================================================================

func TestTLSNotYetValidViaFake(t *testing.T) {
	// Simulate the real Go x509 error message for not-yet-valid
	layer := New(&fakeHandshaker{
		durationMS: 4.0,
		err:        errors.New("x509: certificate has expired or is not yet valid: current time 2026-02-27T00:00:00Z is before 2026-03-01T00:00:00Z"),
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_NOT_YET_VALID" {
		t.Fatalf("error code = %q, want TLS_CERT_NOT_YET_VALID", result.Error.Code)
	}
}

// Test: classifyTLSError with expired message (contains "is after") → TLS_CERT_EXPIRED
func TestTLSExpiredMessageViaFake(t *testing.T) {
	layer := New(&fakeHandshaker{
		durationMS: 4.0,
		err:        errors.New("x509: certificate has expired or is not yet valid: current time 2026-02-27T00:00:00Z is after 2026-02-01T00:00:00Z"),
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_EXPIRED" {
		t.Fatalf("error code = %q, want TLS_CERT_EXPIRED", result.Error.Code)
	}
}

// =============================================================================
// Test: Insecure mode with not-yet-valid cert — status=warn, code=TLS_CERT_NOT_YET_VALID
// (handshake succeeds because insecure, but NotBefore is in the future)
// =============================================================================

func TestTLSInsecureNotYetValidViaFake(t *testing.T) {
	now := time.Now()
	notBeforeFuture := now.Add(24 * time.Hour) // cert not valid until tomorrow

	leaf := &x509.Certificate{
		NotBefore: notBeforeFuture,
		NotAfter:  notBeforeFuture.Add(365 * 24 * time.Hour),
		DNSNames:  []string{"localhost"},
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	layer := New(&fakeHandshaker{
		state:      state,
		durationMS: 3.0,
	})

	pctx := makePctx("localhost", 443, "127.0.0.1", true) // insecure=true
	result := layer.Probe(pctx)

	if result.Status != core.StatusWarn {
		t.Errorf("status = %q, want warn (insecure mode with not-yet-valid cert)", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_NOT_YET_VALID" {
		t.Errorf("error = %v, want TLS_CERT_NOT_YET_VALID warning", result.Error)
	}
}

// =============================================================================
// Test: Not-yet-valid cert via real TLS server — status=fail, code=TLS_CERT_NOT_YET_VALID
// =============================================================================

func TestTLSNotYetValidCert(t *testing.T) {
	now := time.Now()
	cert, pool := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(1 * time.Hour), // not valid yet
		notAfter:  now.Add(365 * 24 * time.Hour),
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	handshaker := &realHandshakerWithRoots{roots: pool}
	layer := New(handshaker)

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_NOT_YET_VALID" {
		t.Errorf("error = %v, want TLS_CERT_NOT_YET_VALID", result.Error)
	}
}

// =============================================================================
// Test: Insecure mode with not-yet-valid cert via real TLS server — status=warn
// =============================================================================

// =============================================================================
// Test: MinVersion TLS 1.2 enforcement
// =============================================================================

// configCaptor captures the *tls.Config passed to the Handshaker.
type configCaptor struct {
	captured *tls.Config
	inner    Handshaker
}

func (c *configCaptor) Handshake(ctx context.Context, addr string, cfg *tls.Config) (*tls.ConnectionState, float64, error) {
	c.captured = cfg
	return c.inner.Handshake(ctx, addr, cfg)
}

func TestTLSMinVersionEnforced(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		DNSNames:  []string{"localhost"},
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	captor := &configCaptor{inner: &fakeHandshaker{state: state, durationMS: 1.0}}
	layer := New(captor)

	pctx := makePctx("localhost", 443, "127.0.0.1", false)
	layer.Probe(pctx)

	if captor.captured == nil {
		t.Fatal("config was not captured")
	}
	if captor.captured.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = 0x%04x, want 0x%04x (TLS 1.2)", captor.captured.MinVersion, tls.VersionTLS12)
	}
}

func TestTLSMinVersionEnforcedInsecure(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		DNSNames:  []string{"localhost"},
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	captor := &configCaptor{inner: &fakeHandshaker{state: state, durationMS: 1.0}}
	layer := New(captor)

	pctx := makePctx("localhost", 443, "127.0.0.1", true) // insecure=true
	layer.Probe(pctx)

	if captor.captured == nil {
		t.Fatal("config was not captured")
	}
	if captor.captured.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = 0x%04x, want 0x%04x (TLS 1.2) even with insecure=true", captor.captured.MinVersion, tls.VersionTLS12)
	}
}

// =============================================================================
// scanHandshaker returns different results based on MinVersion in tls.Config.
// =============================================================================

type scanHandshaker struct {
	// normalState is returned for the normal probe (MinVersion != MaxVersion).
	normalState *tls.ConnectionState
	normalDur   float64
	normalErr   error
	// versionResults maps TLS version constant to handshake result.
	versionResults map[uint16]struct {
		state *tls.ConnectionState
		dur   float64
		err   error
	}
}

func (s *scanHandshaker) Handshake(_ context.Context, _ string, cfg *tls.Config) (*tls.ConnectionState, float64, error) {
	// If MinVersion == MaxVersion, this is a scan call for a specific version.
	if cfg.MinVersion == cfg.MaxVersion && cfg.MinVersion != 0 {
		if r, ok := s.versionResults[cfg.MinVersion]; ok {
			return r.state, r.dur, r.err
		}
		return nil, 0, errors.New("unexpected version in scan")
	}
	// Normal probe call.
	return s.normalState, s.normalDur, s.normalErr
}

func newScanHandshaker(normalState *tls.ConnectionState) *scanHandshaker {
	return &scanHandshaker{
		normalState: normalState,
		normalDur:   5.0,
		versionResults: map[uint16]struct {
			state *tls.ConnectionState
			dur   float64
			err   error
		}{
			tls.VersionTLS10: {state: &tls.ConnectionState{Version: tls.VersionTLS10}, dur: 12.0},
			tls.VersionTLS11: {dur: 5.1, err: errors.New("tls: protocol version not supported")},
			tls.VersionTLS12: {state: &tls.ConnectionState{Version: tls.VersionTLS12}, dur: 8.3},
			tls.VersionTLS13: {state: &tls.ConnectionState{Version: tls.VersionTLS13}, dur: 7.4},
		},
	}
}

func makeScanPctx() *core.ProbeContext {
	// insecure=true: scan tests use fake certs that can't pass x509.Verify;
	// these tests focus on TLS version scanning, not cert chain validation.
	pctx := makePctx("localhost", 443, "127.0.0.1", true)
	pctx.TLSScan = true
	return pctx
}

func validLeaf() *x509.Certificate {
	now := time.Now()
	return &x509.Certificate{
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		DNSNames:  []string{"localhost"},
	}
}

func validState() *tls.ConnectionState {
	return &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{validLeaf()},
	}
}

// =============================================================================
// TLS Scan Tests
// =============================================================================

func TestTLSScanAllVersions(t *testing.T) {
	h := newScanHandshaker(validState())
	layer := New(h)
	pctx := makeScanPctx()

	result := layer.Probe(pctx)

	tlsObs, ok := result.Observations.(*core.TLSObservations)
	if !ok {
		t.Fatalf("observations type = %T, want *core.TLSObservations", result.Observations)
	}
	if tlsObs.TLSScan == nil {
		t.Fatal("missing tls_scan in observations")
	}
	scan := tlsObs.TLSScan

	if scan["performed"] != true {
		t.Errorf("performed = %v, want true", scan["performed"])
	}

	attempts, ok := scan["attempts"].([]map[string]any)
	if !ok {
		t.Fatalf("attempts is not []map[string]any: %T", scan["attempts"])
	}
	if len(attempts) != 4 {
		t.Fatalf("len(attempts) = %d, want 4", len(attempts))
	}

	// TLSv1.0 → supported
	if attempts[0]["version"] != "TLSv1.0" || attempts[0]["supported"] != true {
		t.Errorf("attempts[0] = %v, want TLSv1.0 supported", attempts[0])
	}
	// TLSv1.1 → not supported
	if attempts[1]["version"] != "TLSv1.1" || attempts[1]["supported"] != false {
		t.Errorf("attempts[1] = %v, want TLSv1.1 not supported", attempts[1])
	}
	// TLSv1.2 → supported
	if attempts[2]["version"] != "TLSv1.2" || attempts[2]["supported"] != true {
		t.Errorf("attempts[2] = %v, want TLSv1.2 supported", attempts[2])
	}
	// TLSv1.3 → supported
	if attempts[3]["version"] != "TLSv1.3" || attempts[3]["supported"] != true {
		t.Errorf("attempts[3] = %v, want TLSv1.3 supported", attempts[3])
	}

	// supported_versions
	sv, ok := scan["supported_versions"].([]string)
	if !ok {
		t.Fatalf("supported_versions is not []string: %T", scan["supported_versions"])
	}
	expected := []string{"TLSv1.0", "TLSv1.2", "TLSv1.3"}
	if len(sv) != len(expected) {
		t.Fatalf("supported_versions = %v, want %v", sv, expected)
	}
	for i, v := range expected {
		if sv[i] != v {
			t.Errorf("supported_versions[%d] = %q, want %q", i, sv[i], v)
		}
	}

	// deprecated_versions_enabled
	dv, ok := scan["deprecated_versions_enabled"].([]string)
	if !ok {
		t.Fatalf("deprecated_versions_enabled is not []string: %T", scan["deprecated_versions_enabled"])
	}
	if len(dv) != 1 || dv[0] != "TLSv1.0" {
		t.Errorf("deprecated_versions_enabled = %v, want [TLSv1.0]", dv)
	}
}

func TestTLSScanDeprecatedWarning(t *testing.T) {
	h := newScanHandshaker(validState())
	layer := New(h)
	pctx := makeScanPctx()

	result := layer.Probe(pctx)

	// Normal probe was ok, but deprecated TLSv1.0 is supported → warn
	if result.Status != core.StatusWarn {
		t.Errorf("status = %q, want warn", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_DEPRECATED_VERSION_ENABLED" {
		t.Errorf("error = %v, want TLS_DEPRECATED_VERSION_ENABLED", result.Error)
	}
}

func TestTLSScanNotPerformedByDefault(t *testing.T) {
	h := &fakeHandshaker{
		state:      validState(),
		durationMS: 5.0,
	}
	layer := New(h)
	pctx := makePctx("localhost", 443, "127.0.0.1", false)
	// TLSScan is false by default

	result := layer.Probe(pctx)

	if tlsObs, ok := result.Observations.(*core.TLSObservations); ok && tlsObs.TLSScan != nil {
		t.Error("tls_scan should not be in observations when --tls-scan is not set")
	}
}

func TestTLSScanWithFailedNormalProbe(t *testing.T) {
	h := &scanHandshaker{
		normalDur: 5.0,
		normalErr: x509.UnknownAuthorityError{Cert: &x509.Certificate{}},
		versionResults: map[uint16]struct {
			state *tls.ConnectionState
			dur   float64
			err   error
		}{
			tls.VersionTLS10: {state: &tls.ConnectionState{Version: tls.VersionTLS10}, dur: 12.0},
			tls.VersionTLS11: {dur: 5.1, err: errors.New("tls: protocol version not supported")},
			tls.VersionTLS12: {state: &tls.ConnectionState{Version: tls.VersionTLS12}, dur: 8.3},
			tls.VersionTLS13: {state: &tls.ConnectionState{Version: tls.VersionTLS13}, dur: 7.4},
		},
	}
	layer := New(h)
	pctx := makeScanPctx()

	result := layer.Probe(pctx)

	// Status stays fail (normal probe failed)
	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_UNTRUSTED_CHAIN" {
		t.Errorf("error = %v, want TLS_UNTRUSTED_CHAIN", result.Error)
	}

	// Scan results should still be in observations
	tlsObs, ok := result.Observations.(*core.TLSObservations)
	if !ok {
		t.Fatalf("observations type = %T, want *core.TLSObservations", result.Observations)
	}
	if tlsObs.TLSScan == nil {
		t.Fatal("missing tls_scan in observations even with failed normal probe")
	}
	if tlsObs.TLSScan["performed"] != true {
		t.Errorf("performed = %v, want true", tlsObs.TLSScan["performed"])
	}
}

func TestTLSScanNoDeprecated(t *testing.T) {
	h := &scanHandshaker{
		normalState: validState(),
		normalDur:   5.0,
		versionResults: map[uint16]struct {
			state *tls.ConnectionState
			dur   float64
			err   error
		}{
			tls.VersionTLS10: {dur: 5.0, err: errors.New("tls: protocol version not supported")},
			tls.VersionTLS11: {dur: 5.0, err: errors.New("tls: protocol version not supported")},
			tls.VersionTLS12: {state: &tls.ConnectionState{Version: tls.VersionTLS12}, dur: 8.3},
			tls.VersionTLS13: {state: &tls.ConnectionState{Version: tls.VersionTLS13}, dur: 7.4},
		},
	}
	layer := New(h)
	pctx := makeScanPctx()

	result := layer.Probe(pctx)

	// No deprecated versions → status stays ok
	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok (no deprecated versions)", result.Status)
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	scan := result.Observations.(*core.TLSObservations).TLSScan
	dv := scan["deprecated_versions_enabled"].([]string)
	if len(dv) != 0 {
		t.Errorf("deprecated_versions_enabled = %v, want empty", dv)
	}
}

func TestTLSInsecureNotYetValidCert(t *testing.T) {
	now := time.Now()
	cert, _ := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(1 * time.Hour), // not valid yet
		notAfter:  now.Add(365 * 24 * time.Hour),
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	layer := NewDefault()
	pctx := makePctx("localhost", port, host, true) // insecure=true
	result := layer.Probe(pctx)

	if result.Status != core.StatusWarn {
		t.Errorf("status = %q, want warn (insecure mode should report not-yet-valid); error = %v", result.Status, result.Error)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_NOT_YET_VALID" {
		t.Errorf("error = %v, want TLS_CERT_NOT_YET_VALID", result.Error)
	}
}

// =============================================================================
// Test: No certificates presented — status=fail, code=TLS_NO_CERTIFICATES
// =============================================================================

func TestTLSNoCertificatesViaFakeHandshaker(t *testing.T) {
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: nil, // no certs
	}

	layer := New(&fakeHandshaker{
		state:      state,
		durationMS: 2.5,
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_NO_CERTIFICATES" {
		t.Fatalf("error = %v, want TLS_NO_CERTIFICATES", result.Error)
	}
	if result.DurationMS != 2.5 {
		t.Fatalf("duration_ms = %v, want 2.5", result.DurationMS)
	}
}

// =============================================================================
// Tests: Cert error scenarios should still return observations (2-phase handshake)
// =============================================================================

// assertHasObservations checks that result.Observations contains the key TLS fields
// even when the overall status is fail.
func assertHasObservations(t *testing.T, result *core.LayerResult) {
	t.Helper()

	obs, ok := result.Observations.(*core.TLSObservations)
	if !ok {
		t.Fatalf("observations type = %T, want *core.TLSObservations", result.Observations)
	}
	if obs.Version == "" {
		t.Error("missing 'version' in observations")
	}
	if obs.CipherSuite == "" {
		t.Error("missing 'cipher_suite' in observations")
	}
	if obs.CertDaysUntilExpiry == nil {
		t.Error("missing 'cert_days_until_expiry' in observations")
	}
	if obs.CertHostnameMatch == nil {
		t.Error("missing 'cert_hostname_match' in observations")
	}
}

func TestTLSExpiredCertHasObservations(t *testing.T) {
	now := time.Now()
	cert, pool := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(-48 * time.Hour),
		notAfter:  now.Add(-1 * time.Hour), // expired
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	handshaker := &realHandshakerWithRoots{roots: pool}
	layer := New(handshaker)

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_CERT_EXPIRED" {
		t.Errorf("error = %v, want TLS_CERT_EXPIRED", result.Error)
	}

	// Key assertion: even on cert error, observations must be populated
	assertHasObservations(t, result)
}

func TestTLSHostnameMismatchHasObservations(t *testing.T) {
	now := time.Now()
	cert, pool := generateCert(t, certOpts{
		hosts:     []string{"other.example.com"}, // SAN does not match "localhost"
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(365 * 24 * time.Hour),
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	handshaker := &realHandshakerWithRoots{roots: pool}
	layer := New(handshaker)

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_HOSTNAME_MISMATCH" {
		t.Errorf("error = %v, want TLS_HOSTNAME_MISMATCH", result.Error)
	}

	// Key assertion: even on cert error, observations must be populated
	assertHasObservations(t, result)
}

// =============================================================================
// V0.2 Tests: TLS observations expansion and error classification
// =============================================================================

func TestTLSObservationsV02(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		Subject:   pkix.Name{CommonName: "example.com"},
		Issuer:    pkix.Name{CommonName: "Test CA"},
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		DNSNames:  []string{"example.com", "www.example.com"},
	}
	intermediate := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "Intermediate CA"},
		Issuer:   pkix.Name{CommonName: "Root CA"},
		NotAfter: now.Add(10 * 365 * 24 * time.Hour),
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf, intermediate},
	}

	obs := buildObservations(state, "example.com", true)

	// Existing fields
	if obs.Version != "TLSv1.3" {
		t.Errorf("version = %v, want TLSv1.3", obs.Version)
	}

	// New v0.1 fields
	if obs.CertVerified == nil || !*obs.CertVerified {
		t.Errorf("cert_verified = %v, want true", obs.CertVerified)
	}
	if obs.CertSubject == nil || *obs.CertSubject != "example.com" {
		t.Errorf("cert_subject = %v, want example.com", obs.CertSubject)
	}
	if obs.CertSAN == nil {
		t.Fatal("cert_san is nil")
	}
	san := *obs.CertSAN
	if len(san) != 2 || san[0] != "example.com" || san[1] != "www.example.com" {
		t.Errorf("cert_san = %v, want [example.com www.example.com]", san)
	}
	if obs.CertIssuer == nil || *obs.CertIssuer != "Test CA" {
		t.Errorf("cert_issuer = %v, want Test CA", obs.CertIssuer)
	}
	if obs.CertNotAfter == nil || *obs.CertNotAfter != leaf.NotAfter.UTC().Format(time.RFC3339) {
		t.Errorf("cert_not_after = %v, want %v", obs.CertNotAfter, leaf.NotAfter.UTC().Format(time.RFC3339))
	}
	if obs.CertNotBefore == nil || *obs.CertNotBefore != leaf.NotBefore.UTC().Format(time.RFC3339) {
		t.Errorf("cert_not_before = %v, want %v", obs.CertNotBefore, leaf.NotBefore.UTC().Format(time.RFC3339))
	}

	// cert_chain
	if len(obs.CertChain) != 2 {
		t.Fatalf("cert_chain length = %d, want 2", len(obs.CertChain))
	}
	if obs.CertChain[0].Subject != "example.com" || obs.CertChain[0].Issuer != "Test CA" {
		t.Errorf("cert_chain[0] = %+v", obs.CertChain[0])
	}
	if obs.CertChain[1].Subject != "Intermediate CA" || obs.CertChain[1].Issuer != "Root CA" {
		t.Errorf("cert_chain[1] = %+v", obs.CertChain[1])
	}
}

func TestTLSCertSanEmpty(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		Subject:   pkix.Name{CommonName: "example.com"},
		Issuer:    pkix.Name{CommonName: "Test CA"},
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		DNSNames:  nil, // no SANs
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	obs := buildObservations(state, "example.com", true)

	if obs.CertSAN == nil {
		t.Fatal("cert_san is nil, want non-nil pointer to empty slice")
	}
	if len(*obs.CertSAN) != 0 {
		t.Errorf("cert_san = %v, want empty slice", *obs.CertSAN)
	}
}

func TestTLSCertVerifiedTrue(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		Subject:   pkix.Name{CommonName: "example.com"},
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		DNSNames:  []string{"example.com"},
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	obs := buildObservations(state, "example.com", true) // verified=true
	if obs.CertVerified == nil || *obs.CertVerified != true {
		t.Errorf("cert_verified = %v, want true", obs.CertVerified)
	}
}

func TestTLSCertVerifiedFalse(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		Subject:   pkix.Name{CommonName: "example.com"},
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		DNSNames:  []string{"example.com"},
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	obs := buildObservations(state, "example.com", false) // verified=false (InsecureSkipVerify path)
	if obs.CertVerified == nil || *obs.CertVerified != false {
		t.Errorf("cert_verified = %v, want false", obs.CertVerified)
	}
}

func TestTLSCertChainSummary(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "leaf.example.com"},
		Issuer:   pkix.Name{CommonName: "Intermediate CA"},
		NotAfter: now.Add(365 * 24 * time.Hour),
	}
	inter := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "Intermediate CA"},
		Issuer:   pkix.Name{CommonName: "Root CA"},
		NotAfter: now.Add(5 * 365 * 24 * time.Hour),
	}
	root := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "Root CA"},
		Issuer:   pkix.Name{CommonName: "Root CA"},
		NotAfter: now.Add(10 * 365 * 24 * time.Hour),
	}
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf, inter, root},
	}

	obs := buildObservations(state, "leaf.example.com", true)
	chain := obs.CertChain
	if len(chain) != 3 {
		t.Fatalf("cert_chain length = %d, want 3", len(chain))
	}

	// Verify each entry has subject, issuer, not_after
	for i, entry := range chain {
		if entry.Subject == "" {
			t.Errorf("cert_chain[%d] missing subject", i)
		}
		if entry.Issuer == "" {
			t.Errorf("cert_chain[%d] missing issuer", i)
		}
		if entry.NotAfter == "" {
			t.Errorf("cert_chain[%d] missing not_after", i)
		}
	}

	if chain[0].Subject != "leaf.example.com" {
		t.Errorf("chain[0].Subject = %q, want leaf.example.com", chain[0].Subject)
	}
	if chain[2].Subject != "Root CA" {
		t.Errorf("chain[2].Subject = %q, want Root CA", chain[2].Subject)
	}
}

func TestTLSHostnameMismatchTypeAssertion(t *testing.T) {
	hostErr := x509.HostnameError{
		Host: "wrong.example.com",
		Certificate: &x509.Certificate{
			DNSNames: []string{"example.com"},
		},
	}
	result := classifyTLSError(hostErr)
	if result.Code != "TLS_HOSTNAME_MISMATCH" {
		t.Errorf("code = %q, want TLS_HOSTNAME_MISMATCH", result.Code)
	}
}

func TestTLSUntrustedChainTypeAssertion(t *testing.T) {
	unknownAuth := x509.UnknownAuthorityError{
		Cert: &x509.Certificate{},
	}
	result := classifyTLSError(unknownAuth)
	if result.Code != "TLS_UNTRUSTED_CHAIN" {
		t.Errorf("code = %q, want TLS_UNTRUSTED_CHAIN", result.Code)
	}
}

func TestTLSNoCertificates(t *testing.T) {
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: nil, // empty
	}

	layer := New(&fakeHandshaker{
		state:      state,
		durationMS: 2.0,
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_NO_CERTIFICATES" {
		t.Fatalf("error = %v, want TLS_NO_CERTIFICATES", result.Error)
	}
}

func TestTLSProtocolErrorOnPlainHTTP(t *testing.T) {
	// When TLS connects to a plain HTTP server, Go returns this error.
	layer := New(&fakeHandshaker{
		durationMS: 1.0,
		err:        errors.New("tls: first record does not look like a TLS handshake"),
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_PROTOCOL_ERROR" {
		t.Fatalf("error code = %q, want TLS_PROTOCOL_ERROR", result.Error.Code)
	}
}

func TestTLSUntrustedChainHasObservations(t *testing.T) {
	now := time.Now()
	cert, _ := generateCert(t, certOpts{
		hosts:     []string{"localhost"},
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(365 * 24 * time.Hour),
	})

	addr, cleanup := startTLSServer(t, cert)
	defer cleanup()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	// Use DefaultHandshaker (no custom root pool) → untrusted chain
	layer := NewDefault()

	pctx := makePctx("localhost", port, host, false)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TLS_UNTRUSTED_CHAIN" {
		t.Errorf("error = %v, want TLS_UNTRUSTED_CHAIN", result.Error)
	}

	// Key assertion: even on cert error, observations must be populated
	assertHasObservations(t, result)
}
