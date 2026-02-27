package tcp

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/muras3/probe/internal/core"
	"github.com/muras3/probe/internal/testkit"
)

// Layer performs TCP connection checks.
type Layer struct {
	dialer testkit.Dialer
}

// New creates a TCP Layer with the given dialer.
func New(dialer testkit.Dialer) *Layer {
	return &Layer{dialer: dialer}
}

// NewDefault creates a TCP Layer using the standard net.Dialer.
func NewDefault() *Layer {
	return &Layer{dialer: &net.Dialer{}}
}

func (l *Layer) Name() string { return "tcp" }

func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	address := pctx.Target.HostPort()
	if len(pctx.ResolvedIPs) > 0 {
		address = fmt.Sprintf("%s:%d", pctx.ResolvedIPs[0], pctx.Target.Port)
	}

	start := time.Now()
	conn, err := l.dialer.DialContext(pctx.Context, "tcp", address)
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: map[string]any{},
			Error:        classifyTCPError(err),
		}
	}
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
	return &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: durationMS,
		Observations: map[string]any{
			"remote_ip":   remoteAddr.IP.String(),
			"remote_port": remoteAddr.Port,
		},
		Error: nil,
	}
}

func classifyTCPError(err error) *core.ProbeError {
	// Check for timeout via net.Error interface.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &core.ProbeError{Code: "TCP_TIMEOUT", Message: err.Error()}
	}

	// Check for connection refused via OpError wrapping a SyscallError.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			if strings.Contains(sysErr.Err.Error(), "connection refused") {
				return &core.ProbeError{Code: "TCP_REFUSED", Message: err.Error()}
			}
		}
		// Also check the raw error string for "connection refused" (varies by OS).
		if strings.Contains(err.Error(), "connection refused") {
			return &core.ProbeError{Code: "TCP_REFUSED", Message: err.Error()}
		}
	}

	// Fallback: check error string for connection refused (covers edge cases).
	if strings.Contains(err.Error(), "connection refused") {
		return &core.ProbeError{Code: "TCP_REFUSED", Message: err.Error()}
	}

	return &core.ProbeError{Code: "TCP_ERROR", Message: err.Error()}
}
