package tcp

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/muras3/probe/internal/core"
	"github.com/muras3/probe/internal/testkit"
)

func makeCtx(host string, port int, resolvedIPs []string) *core.ProbeContext {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = cancel
	return &core.ProbeContext{
		Context:     ctx,
		Target:      core.Target{Host: host, Port: port, Scheme: "https"},
		ResolvedIPs: resolvedIPs,
	}
}

func TestTCPName(t *testing.T) {
	layer := New(&testkit.FakeDialer{})
	if layer.Name() != "tcp" {
		t.Errorf("Name() = %q, want tcp", layer.Name())
	}
}

func TestTCPSuccess(t *testing.T) {
	// Start a real TCP listener on localhost with a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Accept connections in background so the dial succeeds.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	addr := ln.Addr().(*net.TCPAddr)
	pctx := makeCtx("127.0.0.1", addr.Port, []string{"127.0.0.1"})

	layer := NewDefault()
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

	remoteIP, ok := result.Observations["remote_ip"]
	if !ok {
		t.Fatal("missing remote_ip observation")
	}
	if remoteIP != "127.0.0.1" {
		t.Errorf("remote_ip = %v, want 127.0.0.1", remoteIP)
	}

	remotePort, ok := result.Observations["remote_port"]
	if !ok {
		t.Fatal("missing remote_port observation")
	}
	if remotePort != addr.Port {
		t.Errorf("remote_port = %v, want %d", remotePort, addr.Port)
	}
}

func TestTCPConnectionRefused(t *testing.T) {
	// Find a port that is not listening by binding and immediately closing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // Port is now free and should refuse connections.

	pctx := makeCtx("127.0.0.1", port, []string{"127.0.0.1"})

	layer := NewDefault()
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TCP_REFUSED" {
		t.Errorf("error code = %v, want TCP_REFUSED", result.Error)
	}
}

func TestTCPTimeout(t *testing.T) {
	// Use FakeDialer to simulate a timeout error.
	timeoutErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &timeoutError{msg: "i/o timeout"},
	}
	layer := New(&testkit.FakeDialer{Err: timeoutErr})
	pctx := makeCtx("192.0.2.1", 443, []string{"192.0.2.1"})

	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.DurationMS >= 0 {
		// duration can be 0 for fake operations
	}
	if result.Error == nil || result.Error.Code != "TCP_TIMEOUT" {
		t.Errorf("error code = %v, want TCP_TIMEOUT", result.Error)
	}
}

func TestTCPGenericError(t *testing.T) {
	layer := New(&testkit.FakeDialer{Err: errors.New("something went wrong")})
	pctx := makeCtx("192.0.2.1", 443, []string{"192.0.2.1"})

	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TCP_ERROR" {
		t.Errorf("error code = %v, want TCP_ERROR", result.Error)
	}
}

func TestTCPUsesResolvedIP(t *testing.T) {
	// Verify that when ResolvedIPs is set, the dialer is called with that IP.
	var dialedAddress string
	dialer := &capturingDialer{
		inner: &testkit.FakeDialer{Err: errors.New("test")},
		onDial: func(addr string) {
			dialedAddress = addr
		},
	}

	pctx := makeCtx("example.com", 443, []string{"93.184.216.34"})
	layer := New(dialer)
	layer.Probe(pctx)

	if dialedAddress != "93.184.216.34:443" {
		t.Errorf("dialed address = %q, want 93.184.216.34:443", dialedAddress)
	}
}

func TestTCPFallsBackToHostPort(t *testing.T) {
	// Verify that when ResolvedIPs is empty, HostPort() is used.
	var dialedAddress string
	dialer := &capturingDialer{
		inner: &testkit.FakeDialer{Err: errors.New("test")},
		onDial: func(addr string) {
			dialedAddress = addr
		},
	}

	pctx := makeCtx("example.com", 443, nil) // no resolved IPs
	layer := New(dialer)
	layer.Probe(pctx)

	if dialedAddress != "example.com:443" {
		t.Errorf("dialed address = %q, want example.com:443", dialedAddress)
	}
}

func TestTCPConnectionRefusedFake(t *testing.T) {
	// Use FakeDialer with a "connection refused" error to verify classification
	// without relying on actual port availability.
	refusedErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "connect",
			Err:     errors.New("connection refused"),
		},
	}
	layer := New(&testkit.FakeDialer{Err: refusedErr})
	pctx := makeCtx("127.0.0.1", 9999, []string{"127.0.0.1"})

	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "TCP_REFUSED" {
		t.Errorf("error code = %v, want TCP_REFUSED", result.Error)
	}
}

// timeoutError implements net.Error with Timeout() == true.
type timeoutError struct {
	msg string
}

func (e *timeoutError) Error() string   { return e.msg }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return false }

// capturingDialer wraps a Dialer and captures the address passed to DialContext.
type capturingDialer struct {
	inner  Dialer
	onDial func(address string)
}

func (d *capturingDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d.onDial != nil {
		d.onDial(address)
	}
	return d.inner.DialContext(ctx, network, address)
}

// Ensure tests compile references are used.
var _ = strings.Contains
