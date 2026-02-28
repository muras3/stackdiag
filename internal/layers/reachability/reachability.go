package reachability

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

// Pinger abstracts ICMP echo request/reply.
type Pinger interface {
	Ping(ctx context.Context, addr string) (rtt time.Duration, err error)
}

// Layer performs ICMP reachability checks.
type Layer struct {
	pinger Pinger
}

// New creates a reachability Layer with the given pinger.
func New(pinger Pinger) *Layer {
	return &Layer{pinger: pinger}
}

// NewDefault creates a reachability Layer using raw ICMP sockets.
func NewDefault() *Layer {
	return &Layer{pinger: &icmpPinger{}}
}

func (l *Layer) Name() string { return "reachability" }

func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	addr := pctx.Target.Host
	if len(pctx.ResolvedIPs) > 0 {
		addr = pctx.ResolvedIPs[0]
	}

	start := time.Now()
	rtt, err := l.pinger.Ping(pctx.Context, addr)
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		if reason := skipReason(err); reason != "" {
			return &core.LayerResult{
				Status:     core.StatusSkip,
				DurationMS: durationMS,
				Observations: map[string]any{
					"probe_method": "none",
					"reachable":    nil,
					"rtt_ms":       nil,
					"skip_reason":  reason,
				},
			}
		}

		code := "REACHABILITY_ERROR"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = "REACHABILITY_TIMEOUT"
		}

		return &core.LayerResult{
			Status:     core.StatusFail,
			DurationMS: durationMS,
			Observations: map[string]any{
				"probe_method": "icmp",
				"reachable":    false,
				"rtt_ms":       nil,
				"skip_reason":  nil,
			},
			Error: &core.ProbeError{Code: code, Message: err.Error()},
		}
	}

	rttMS := float64(rtt.Microseconds()) / 1000.0
	return &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: durationMS,
		Observations: map[string]any{
			"probe_method": "icmp",
			"reachable":    true,
			"rtt_ms":       rttMS,
			"skip_reason":  nil,
		},
	}
}

// skipReason returns a non-empty reason string if the error indicates the
// reachability probe should be skipped (non-blocking). Returns "" if the
// error represents a genuine reachability failure.
func skipReason(err error) string {
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return "permission_denied"
	}
	var opErr *os.PathError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.EPERM) || errors.Is(opErr.Err, syscall.EACCES) {
			return "permission_denied"
		}
	}
	// IPv6 addresses with ip4:icmp dial produce "no suitable address found".
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "unsupported_address"
	}
	if strings.Contains(err.Error(), "no suitable address found") {
		return "unsupported_address"
	}
	return ""
}

// icmpPinger sends ICMP echo requests using raw sockets.
type icmpPinger struct{}

func (p *icmpPinger) Ping(ctx context.Context, addr string) (time.Duration, error) {
	conn, err := net.Dial("ip4:icmp", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Build ICMP Echo Request (type=8, code=0)
	msg := buildICMPEchoRequest(0, 1)

	start := time.Now()
	if _, err := conn.Write(msg); err != nil {
		return 0, err
	}

	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return 0, err
		}

		// Skip IP header (usually 20 bytes) to get ICMP payload
		if n < 20 {
			continue
		}
		ipHeaderLen := int(buf[0]&0x0f) << 2
		if n < ipHeaderLen+8 {
			continue
		}
		icmpData := buf[ipHeaderLen:n]

		// Check for Echo Reply (type=0, code=0)
		if icmpData[0] == 0 && icmpData[1] == 0 {
			return time.Since(start), nil
		}
	}
}

func buildICMPEchoRequest(id, seq uint16) []byte {
	msg := make([]byte, 8)
	msg[0] = 8 // Type: Echo Request
	msg[1] = 0 // Code
	// Checksum at [2:4], computed below
	binary.BigEndian.PutUint16(msg[4:6], id)
	binary.BigEndian.PutUint16(msg[6:8], seq)

	// Compute checksum
	csum := icmpChecksum(msg)
	binary.BigEndian.PutUint16(msg[2:4], csum)
	return msg
}

func icmpChecksum(data []byte) uint16 {
	var sum uint32
	length := len(data)
	for i := 0; i < length-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if length%2 == 1 {
		sum += uint32(data[length-1]) << 8
	}
	sum = (sum >> 16) + (sum & 0xffff)
	sum += sum >> 16
	return uint16(^sum)
}

// Ensure Layer implements core.Layer.
var _ core.Layer = (*Layer)(nil)

// Ensure icmpPinger implements Pinger.
var _ Pinger = (*icmpPinger)(nil)

// Ensure FakePinger from testkit satisfies Pinger at compile time.
// (Checked in tests via New(&testkit.FakePinger{}))
