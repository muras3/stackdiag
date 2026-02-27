package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muras3/probe/internal/core"
)

// Layer performs HTTP request diagnostics.
// Measurement: duration = TTFB (request sent → first response header byte).
type Layer struct {
	client *http.Client
}

// New creates an HTTP Layer with the given http.Client.
func New(client *http.Client) *Layer {
	return &Layer{client: client}
}

// NewDefault creates an HTTP Layer with a transport tuned for probing:
// no keep-alives, no auto-redirect, configurable TLS.
func NewDefault(insecure bool) *Layer {
	transport := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: insecure},
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Layer{client: client}
}

func (l *Layer) Name() string { return "http" }

// Probe sends an HTTP request and measures TTFB.
func (l *Layer) Probe(pctx *core.ProbeContext) *core.LayerResult {
	url := buildURL(pctx)

	method := pctx.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(pctx.Context, method, url, nil)
	if err != nil {
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   0,
			Observations: map[string]any{"method": method},
			Error:        &core.ProbeError{Code: "HTTP_ERROR", Message: err.Error()},
		}
	}

	// When using a resolved IP, set Host header to the original hostname
	// so virtual-host routing and TLS SNI work correctly.
	if len(pctx.ResolvedIPs) > 0 {
		req.Host = pctx.Target.Host
	}

	for k, v := range pctx.Headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := l.client.Do(req)
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: map[string]any{"method": method},
			Error:        classifyHTTPError(err),
		}
	}
	defer resp.Body.Close()

	obs := map[string]any{
		"method":      method,
		"protocol":    resp.Proto,
		"status_code": resp.StatusCode,
	}

	if resp.StatusCode >= 400 {
		return &core.LayerResult{
			Status:       core.StatusFail,
			DurationMS:   durationMS,
			Observations: obs,
			Error:        classifyStatusCode(resp.StatusCode),
		}
	}

	return &core.LayerResult{
		Status:       core.StatusOK,
		DurationMS:   durationMS,
		Observations: obs,
		Error:        nil,
	}
}

func buildURL(pctx *core.ProbeContext) string {
	scheme := pctx.Target.Scheme
	host := pctx.Target.Host
	port := pctx.Target.Port
	path := pctx.Target.Path

	// Use resolved IP if available, with original host in Host header.
	if len(pctx.ResolvedIPs) > 0 {
		host = pctx.ResolvedIPs[0]
	}

	hostPort := host
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
			hostPort = fmt.Sprintf("[%s]", host)
		}
	} else {
		hostPort = net.JoinHostPort(host, strconv.Itoa(port))
	}

	return fmt.Sprintf("%s://%s%s", scheme, hostPort, path)
}

func classifyHTTPError(err error) *core.ProbeError {
	if isTimeout(err) {
		return &core.ProbeError{Code: "HTTP_TIMEOUT", Message: err.Error()}
	}
	return &core.ProbeError{Code: "HTTP_ERROR", Message: err.Error()}
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// Check wrapped errors.
	if strings.Contains(err.Error(), "context deadline exceeded") {
		return true
	}
	return false
}

func classifyStatusCode(code int) *core.ProbeError {
	msg := fmt.Sprintf("%d %s", code, http.StatusText(code))

	// Specific codes get their own error code.
	switch code {
	case 401:
		return &core.ProbeError{Code: "HTTP_401", Message: msg}
	case 403:
		return &core.ProbeError{Code: "HTTP_403", Message: msg}
	case 404:
		return &core.ProbeError{Code: "HTTP_404", Message: msg}
	case 429:
		return &core.ProbeError{Code: "HTTP_429", Message: msg}
	case 502:
		return &core.ProbeError{Code: "HTTP_502", Message: msg}
	case 503:
		return &core.ProbeError{Code: "HTTP_503", Message: msg}
	case 504:
		return &core.ProbeError{Code: "HTTP_504", Message: msg}
	}

	// Generic categories.
	if code >= 500 {
		return &core.ProbeError{Code: "HTTP_5XX", Message: msg}
	}
	return &core.ProbeError{Code: fmt.Sprintf("HTTP_%d", code), Message: msg}
}
