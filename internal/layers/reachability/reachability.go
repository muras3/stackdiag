package reachability

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

// ICMP message type constants.
const (
	icmpEchoReply     = 0
	icmpEchoRequest   = 8
	icmpv6EchoRequest = 128
	icmpv6EchoReply   = 129
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
				Observations: &core.ReachabilityObservations{
					ProbeMethod: "none",
					SkipReason:  &reason,
				},
			}
		}

		code := "REACHABILITY_ERROR"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = "REACHABILITY_TIMEOUT"
		}

		f := false
		return &core.LayerResult{
			Status:     core.StatusFail,
			DurationMS: durationMS,
			Observations: &core.ReachabilityObservations{
				ProbeMethod: "icmp",
				Reachable:   &f,
			},
			Error: &core.ProbeError{Code: code, Message: err.Error()},
		}
	}

	rttMS := float64(rtt.Microseconds()) / 1000.0
	tr := true
	return &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: durationMS,
		Observations: &core.ReachabilityObservations{
			ProbeMethod: "icmp",
			Reachable:   &tr,
			RTTMS:       &rttMS,
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
	return ""
}

func buildICMPv6EchoRequest(id, seq uint16) []byte {
	msg := make([]byte, 8)
	msg[0] = icmpv6EchoRequest
	msg[1] = 0 // Code
	// Checksum at [2:4] = 0 (kernel-computed for ICMPv6)
	binary.BigEndian.PutUint16(msg[4:6], id)
	binary.BigEndian.PutUint16(msg[6:8], seq)
	return msg
}

// isIPv6 returns true if addr is an IPv6 address.
func isIPv6(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}

// icmpPinger sends ICMP echo requests using raw sockets.
type icmpPinger struct{}

func (p *icmpPinger) Ping(ctx context.Context, addr string) (time.Duration, error) {
	// Select protocol based on address type.
	network := "ip4:icmp"
	v6 := isIPv6(addr)
	if v6 {
		network = "ip6:ipv6-icmp"
	}

	conn, err := net.Dial(network, addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	// Ensure conn.Read unblocks when the context is canceled or has a deadline.
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	// Watch for context cancellation (handles cancel without deadline).
	// The done channel prevents the goroutine from leaking on success.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	id := uint16(os.Getpid() & 0xffff)
	// seq is always 1: each --count attempt creates a fresh net.Dial connection,
	// so stale replies from prior attempts cannot arrive on a new raw socket.
	var msg []byte
	if v6 {
		msg = buildICMPv6EchoRequest(id, 1)
	} else {
		msg = buildICMPEchoRequest(id, 1)
	}

	start := time.Now()
	if _, err := conn.Write(msg); err != nil {
		return 0, err
	}

	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			// Map close-after-cancel to context error for clean reporting.
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			return 0, err
		}

		if v6 {
			// IPv6 raw sockets do NOT include the IP header.
			if n < 8 {
				continue
			}
			if buf[0] == icmpv6EchoReply && buf[1] == 0 {
				replyID := binary.BigEndian.Uint16(buf[4:6])
				if replyID == id {
					return time.Since(start), nil
				}
			}
		} else {
			// IPv4: skip IP header (usually 20 bytes) to get ICMP payload
			if n < 20 {
				continue
			}
			ipHeaderLen := int(buf[0]&0x0f) << 2
			if n < ipHeaderLen+8 {
				continue
			}
			icmpData := buf[ipHeaderLen:n]
			if icmpData[0] == icmpEchoReply && icmpData[1] == 0 {
				replyID := binary.BigEndian.Uint16(icmpData[4:6])
				if replyID == id {
					return time.Since(start), nil
				}
			}
		}
	}
}

func buildICMPEchoRequest(id, seq uint16) []byte {
	msg := make([]byte, 8)
	msg[0] = icmpEchoRequest
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
