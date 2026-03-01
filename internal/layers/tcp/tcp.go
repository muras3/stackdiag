package tcp

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Layer performs TCP connection checks.
type Layer struct {
	dialer Dialer
}

// New creates a TCP Layer with the given dialer.
func New(dialer Dialer) *Layer {
	return &Layer{dialer: dialer}
}

// NewDefault creates a TCP Layer using the standard net.Dialer.
func NewDefault() *Layer {
	return &Layer{dialer: &net.Dialer{}}
}

func (l *Layer) Name() string { return "tcp" }

func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	address := net.JoinHostPort(pctx.Target.Host, strconv.Itoa(pctx.Target.Port))
	if len(pctx.ResolvedIPs) > 0 {
		address = net.JoinHostPort(pctx.ResolvedIPs[0], strconv.Itoa(pctx.Target.Port))
	}

	start := time.Now()
	conn, err := l.dialer.DialContext(pctx.Context, "tcp", address)
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: &core.TCPObservations{},
			Error:        classifyTCPError(err),
		}
	}
	defer conn.Close()

	remoteIP := ""
	remotePort := 0
	remoteAddr := conn.RemoteAddr()
	if tcpAddr, ok := remoteAddr.(*net.TCPAddr); ok {
		remoteIP = tcpAddr.IP.String()
		remotePort = tcpAddr.Port
	} else {
		host, port, splitErr := net.SplitHostPort(remoteAddr.String())
		if splitErr == nil {
			remoteIP = host
			parsedPort, parseErr := strconv.Atoi(port)
			if parseErr == nil {
				remotePort = parsedPort
			}
		}
	}

	return &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: durationMS,
		Observations: &core.TCPObservations{
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
		},
		Error: nil,
	}
}

func classifyTCPError(err error) *core.ProbeError {
	// Check for timeout via net.Error interface.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &core.ProbeError{Code: "TCP_TIMEOUT", Message: "connection timed out"}
	}

	// Use syscall errno for robust, OS-independent classification.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			if errors.Is(sysErr.Err, syscall.ECONNREFUSED) {
				return &core.ProbeError{Code: "TCP_REFUSED", Message: "connection refused"}
			}
			if errors.Is(sysErr.Err, syscall.EHOSTUNREACH) {
				return &core.ProbeError{Code: "TCP_HOST_UNREACHABLE", Message: "host unreachable"}
			}
			if errors.Is(sysErr.Err, syscall.ENETUNREACH) {
				return &core.ProbeError{Code: "TCP_NETWORK_UNREACHABLE", Message: "network unreachable"}
			}
		}
	}

	return &core.ProbeError{Code: "TCP_ERROR", Message: err.Error()}
}
