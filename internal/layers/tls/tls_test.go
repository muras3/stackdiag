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
	if _, ok := result.Observations["version"]; !ok {
		t.Error("missing 'version' observation")
	}
	if _, ok := result.Observations["cipher_suite"]; !ok {
		t.Error("missing 'cipher_suite' observation")
	}
	days, ok := result.Observations["cert_days_until_expiry"]
	if !ok {
		t.Error("missing 'cert_days_until_expiry' observation")
	}
	if d, ok := days.(int); ok && d < 300 {
		t.Errorf("cert_days_until_expiry = %d, expected > 300", d)
	}
	match, ok := result.Observations["cert_hostname_match"]
	if !ok {
		t.Error("missing 'cert_hostname_match' observation")
	}
	if match != true {
		t.Errorf("cert_hostname_match = %v, want true", match)
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

	days, ok := result.Observations["cert_days_until_expiry"]
	if !ok {
		t.Fatal("missing cert_days_until_expiry observation")
	}
	if d, ok := days.(int); !ok || d < 10 || d > 20 {
		t.Errorf("cert_days_until_expiry = %v, want 14-16", days)
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
		err:        errors.New("x509: certificate signed by unknown authority"),
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
func TestTLSCertExpiredLessThan24h(t *testing.T) {
	now := time.Now()
	expiredAt := now.Add(-1 * time.Hour) // expired 1 hour ago

	leaf := &x509.Certificate{
		NotAfter: expiredAt,
		DNSNames: []string{"localhost"},
	}
	certDER := []byte("fake") // not used by fake handshaker

	_ = certDER
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates: []*x509.Certificate{leaf},
	}

	layer := New(&fakeHandshaker{
		state:      state,
		durationMS: 3.0,
	})

	result := layer.Probe(makePctx("localhost", 443, "127.0.0.1", false))
	if result.Status != core.StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
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
	pctx := makePctx("localhost", 443, "127.0.0.1", false)
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

	scanRaw, ok := result.Observations["tls_scan"]
	if !ok {
		t.Fatal("missing tls_scan in observations")
	}
	scan, ok := scanRaw.(map[string]any)
	if !ok {
		t.Fatalf("tls_scan is not map[string]any: %T", scanRaw)
	}

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

	if _, ok := result.Observations["tls_scan"]; ok {
		t.Error("tls_scan should not be in observations when --tls-scan is not set")
	}
}

func TestTLSScanWithFailedNormalProbe(t *testing.T) {
	h := &scanHandshaker{
		normalDur: 5.0,
		normalErr: errors.New("x509: certificate signed by unknown authority"),
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
	scanRaw, ok := result.Observations["tls_scan"]
	if !ok {
		t.Fatal("missing tls_scan in observations even with failed normal probe")
	}
	scan := scanRaw.(map[string]any)
	if scan["performed"] != true {
		t.Errorf("performed = %v, want true", scan["performed"])
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

	scan := result.Observations["tls_scan"].(map[string]any)
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
