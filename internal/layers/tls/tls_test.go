package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/muras3/probe/internal/core"
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
		Subject:               pkix.Name{Organization: []string{"probe-test"}},
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
	state *tls.ConnectionState
	err   error
}

func (f *fakeHandshaker) Handshake(_ context.Context, _ string, _ *tls.Config) (*tls.ConnectionState, error) {
	return f.state, f.err
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

func (h *realHandshakerWithRoots) Handshake(ctx context.Context, addr string, cfg *tls.Config) (*tls.ConnectionState, error) {
	cfg.RootCAs = h.roots
	return (&DefaultHandshaker{}).Handshake(ctx, addr, cfg)
}
