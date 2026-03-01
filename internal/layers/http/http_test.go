package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

func makePctx(url string) *core.ProbeContext {
	return &core.ProbeContext{
		Context:  context.Background(),
		Target:   core.Target{Host: "localhost", Port: 80, Scheme: "http", Path: "/"},
		Method:   "GET",
		Headers:  map[string]string{},
		Insecure: false,
	}
}

func TestHTTPName(t *testing.T) {
	layer := NewDefault(false, "")
	if layer.Name() != "http" {
		t.Errorf("Name() = %q, want http", layer.Name())
	}
}

func TestHTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
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

	sc, ok := result.Observations.(map[string]any)["status_code"]
	if !ok {
		t.Fatal("missing status_code observation")
	}
	if sc.(int) != 200 {
		t.Errorf("status_code = %v, want 200", sc)
	}
	method, ok := result.Observations.(map[string]any)["method"]
	if !ok {
		t.Fatal("missing method observation")
	}
	if method.(string) != "GET" {
		t.Errorf("method = %v, want GET", method)
	}
}

func TestHTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_401" {
		t.Errorf("error = %v, want HTTP_401", result.Error)
	}
}

func TestHTTP403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_403" {
		t.Errorf("error = %v, want HTTP_403", result.Error)
	}
}

func TestHTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_404" {
		t.Errorf("error = %v, want HTTP_404", result.Error)
	}
}

func TestHTTP429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_429" {
		t.Errorf("error = %v, want HTTP_429", result.Error)
	}
}

func TestHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_500" {
		t.Errorf("error = %v, want HTTP_500", result.Error)
	}
}

func TestHTTP502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_502" {
		t.Errorf("error = %v, want HTTP_502", result.Error)
	}
}

func TestHTTP503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_503" {
		t.Errorf("error = %v, want HTTP_503", result.Error)
	}
}

func TestHTTP504(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(504)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_504" {
		t.Errorf("error = %v, want HTTP_504", result.Error)
	}
}

func TestHTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	pctx := &core.ProbeContext{
		Context:  ctx,
		Target:   targetFromURL(t, srv.URL),
		Method:   "GET",
		Headers:  map[string]string{},
		Insecure: false,
	}
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_TIMEOUT" {
		t.Errorf("error = %v, want HTTP_TIMEOUT", result.Error)
	}
}

func TestHTTPConnectionReset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("hijack not supported")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
}

func TestHTTPCustomMethodAndHeaders(t *testing.T) {
	var receivedMethod string
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedHeader = r.Header.Get("X-Custom")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := &core.ProbeContext{
		Context:  context.Background(),
		Target:   targetFromURL(t, srv.URL),
		Method:   "POST",
		Headers:  map[string]string{"X-Custom": "test-value"},
		Insecure: false,
	}
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok; error = %v", result.Status, result.Error)
	}
	if receivedMethod != "POST" {
		t.Errorf("server received method = %q, want POST", receivedMethod)
	}
	if receivedHeader != "test-value" {
		t.Errorf("server received X-Custom = %q, want test-value", receivedHeader)
	}
	if m, ok := result.Observations.(map[string]any)["method"]; !ok || m.(string) != "POST" {
		t.Errorf("method observation = %v, want POST", result.Observations.(map[string]any)["method"])
	}
}

func TestBuildURLWithResolvedIPv6(t *testing.T) {
	pctx := &core.ProbeContext{
		Target: core.Target{
			Scheme: "http",
			Host:   "example.com",
			Port:   8080,
			Path:   "/health",
		},
		ResolvedIPs: []string{"2001:db8::1"},
	}

	got := buildURL(pctx)
	want := "http://[2001:db8::1]:8080/health"
	if got != want {
		t.Fatalf("buildURL() = %q, want %q", got, want)
	}
}

func TestProbeResolvedIPSetsURLHostAndHostHeader(t *testing.T) {
	var gotURLHost string
	var gotHostHeader string

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotURLHost = req.URL.Host
			gotHostHeader = req.Host
			return &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}),
	}

	layer := New(client)
	pctx := &core.ProbeContext{
		Context: context.Background(),
		Target: core.Target{
			Scheme: "http",
			Host:   "example.com",
			Port:   8080,
			Path:   "/path",
		},
		Method:      "GET",
		Headers:     map[string]string{},
		ResolvedIPs: []string{"127.0.0.1"},
	}

	result := layer.Probe(pctx)
	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok; error = %v", result.Status, result.Error)
	}
	if gotURLHost != "127.0.0.1:8080" {
		t.Fatalf("request URL host = %q, want %q", gotURLHost, "127.0.0.1:8080")
	}
	if gotHostHeader != "example.com" {
		t.Fatalf("request Host header = %q, want %q", gotHostHeader, "example.com")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTP3xxStatusOk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(301)
	}))
	defer srv.Close()

	client := srv.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	layer := New(client)
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Errorf("status = %q, want ok for 3xx; error = %v", result.Status, result.Error)
	}
	sc, ok := result.Observations.(map[string]any)["status_code"]
	if !ok {
		t.Fatal("missing status_code")
	}
	if sc.(int) != 301 {
		t.Errorf("status_code = %v, want 301", sc)
	}
}

func TestHTTPGeneric5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(501)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusFail {
		t.Errorf("status = %q, want fail", result.Status)
	}
	if result.Error == nil || result.Error.Code != "HTTP_5XX" {
		t.Errorf("error = %v, want HTTP_5XX (501 is not a named status code)", result.Error)
	}
}

func TestHTTPProtocolObservation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok; error = %v", result.Status, result.Error)
	}
	proto, ok := result.Observations.(map[string]any)["protocol"]
	if !ok {
		t.Fatal("missing protocol observation")
	}
	p := proto.(string)
	if !strings.HasPrefix(p, "HTTP/") {
		t.Errorf("protocol = %q, want HTTP/... prefix", p)
	}
}

// requestHeadersObs extracts request_headers from observations as map[string]string.
func requestHeadersObs(t *testing.T, result *core.LayerResult) map[string]string {
	t.Helper()
	raw, ok := result.Observations.(map[string]any)["request_headers"]
	if !ok {
		t.Fatal("missing request_headers observation")
	}
	switch h := raw.(type) {
	case map[string]string:
		return h
	case map[string]any:
		out := make(map[string]string, len(h))
		for k, v := range h {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("request_headers[%q] has non-string type %T", k, v)
			}
			out[k] = s
		}
		return out
	default:
		t.Fatalf("request_headers has unexpected type %T", raw)
		return nil
	}
}

func TestHTTPRequestHeadersPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	pctx.Headers = map[string]string{"X-Custom": "value"}
	pctx.Redact = true

	result := layer.Probe(pctx)
	got := requestHeadersObs(t, result)
	if got["X-Custom"] != "value" {
		t.Errorf("X-Custom = %q, want %q", got["X-Custom"], "value")
	}
}

func TestHTTPRequestHeadersRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tests := []struct {
		name    string
		headers map[string]string
		redact  bool
		wantKey string
		wantVal string
	}{
		{
			name:    "Authorization redacted",
			headers: map[string]string{"Authorization": "Bearer secret-token"},
			redact:  true,
			wantKey: "Authorization",
			wantVal: "[REDACTED]",
		},
		{
			name:    "Cookie redacted",
			headers: map[string]string{"Cookie": "session=abc123"},
			redact:  true,
			wantKey: "Cookie",
			wantVal: "[REDACTED]",
		},
		{
			name:    "Proxy-Authorization redacted",
			headers: map[string]string{"Proxy-Authorization": "Basic dXNlcjpwYXNz"},
			redact:  true,
			wantKey: "Proxy-Authorization",
			wantVal: "[REDACTED]",
		},
		{
			name:    "X-API-Key redacted",
			headers: map[string]string{"X-API-Key": "super-secret"},
			redact:  true,
			wantKey: "X-API-Key",
			wantVal: "[REDACTED]",
		},
		{
			name:    "X-Auth-Token redacted",
			headers: map[string]string{"X-Auth-Token": "super-secret"},
			redact:  true,
			wantKey: "X-Auth-Token",
			wantVal: "[REDACTED]",
		},
		{
			name:    "Set-Cookie redacted",
			headers: map[string]string{"Set-Cookie": "session=abc123"},
			redact:  true,
			wantKey: "Set-Cookie",
			wantVal: "[REDACTED]",
		},
		{
			name:    "case-insensitive authorization",
			headers: map[string]string{"authorization": "Bearer token"},
			redact:  true,
			wantKey: "authorization",
			wantVal: "[REDACTED]",
		},
		{
			name:    "no-redact shows raw Authorization",
			headers: map[string]string{"Authorization": "Bearer secret-token"},
			redact:  false,
			wantKey: "Authorization",
			wantVal: "Bearer secret-token",
		},
		{
			name:    "non-sensitive header unmasked when redact=true",
			headers: map[string]string{"X-Custom": "value"},
			redact:  true,
			wantKey: "X-Custom",
			wantVal: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer := New(srv.Client())
			pctx := makePctx(srv.URL)
			pctx.Target = targetFromURL(t, srv.URL)
			pctx.Headers = tt.headers
			pctx.Redact = tt.redact

			result := layer.Probe(pctx)
			got := requestHeadersObs(t, result)
			if got[tt.wantKey] != tt.wantVal {
				t.Errorf("%s = %q, want %q", tt.wantKey, got[tt.wantKey], tt.wantVal)
			}
		})
	}
}

func TestNewDefaultMinVersionTLS12(t *testing.T) {
	layer := NewDefault(false, "example.com")
	transport, ok := layer.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", layer.client.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if transport.TLSClientConfig.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = 0x%04x, want 0x0303 (TLS 1.2)", transport.TLSClientConfig.MinVersion)
	}
}

func TestNewDefaultMinVersionTLS12Insecure(t *testing.T) {
	layer := NewDefault(true, "example.com")
	transport, ok := layer.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", layer.client.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if transport.TLSClientConfig.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = 0x%04x, want 0x0303 (TLS 1.2) even with insecure=true", transport.TLSClientConfig.MinVersion)
	}
}

func TestNewDefaultSetsResponseHeaderLimit(t *testing.T) {
	layer := NewDefault(false, "example.com")
	transport, ok := layer.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", layer.client.Transport)
	}
	if transport.MaxResponseHeaderBytes != 1<<20 {
		t.Fatalf("MaxResponseHeaderBytes = %d, want %d", transport.MaxResponseHeaderBytes, 1<<20)
	}
}

func TestHTTPRequestHeadersMixedSensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	pctx.Headers = map[string]string{
		"Authorization": "Bearer token",
		"X-Request-Id":  "abc123",
	}
	pctx.Redact = true

	result := layer.Probe(pctx)
	got := requestHeadersObs(t, result)

	if got["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization = %q, want [REDACTED]", got["Authorization"])
	}
	if got["X-Request-Id"] != "abc123" {
		t.Errorf("X-Request-Id = %q, want abc123", got["X-Request-Id"])
	}
}

func TestHTTPRequestHeadersEmptyMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	pctx.Headers = map[string]string{}
	pctx.Redact = true

	result := layer.Probe(pctx)
	got := requestHeadersObs(t, result)
	if len(got) != 0 {
		t.Errorf("expected empty request_headers, got %v", got)
	}
}

// responseHeadersObs extracts response_headers from observations as map[string]string.
func responseHeadersObs(t *testing.T, result *core.LayerResult) map[string]string {
	t.Helper()
	raw, ok := result.Observations.(map[string]any)["response_headers"]
	if !ok {
		t.Fatal("missing response_headers observation")
	}
	switch h := raw.(type) {
	case map[string]string:
		return h
	case map[string]any:
		out := make(map[string]string, len(h))
		for k, v := range h {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("response_headers[%q] has non-string type %T", k, v)
			}
			out[k] = s
		}
		return out
	default:
		t.Fatalf("response_headers has unexpected type %T", raw)
		return nil
	}
}

func TestHTTPResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Server", "test-server")
		w.Header().Set("X-Request-Id", "req-123")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok; error = %v", result.Status, result.Error)
	}

	got := responseHeadersObs(t, result)
	if got["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got["Content-Type"], "application/json")
	}
	if got["Server"] != "test-server" {
		t.Errorf("Server = %q, want %q", got["Server"], "test-server")
	}
	if got["X-Request-Id"] != "req-123" {
		t.Errorf("X-Request-Id = %q, want %q", got["X-Request-Id"], "req-123")
	}
}

func TestHTTPResponseHeadersRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "session=secret123")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	pctx.Redact = true
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok; error = %v", result.Status, result.Error)
	}

	got := responseHeadersObs(t, result)
	// Set-Cookie is sensitive but not in the allowlist, so it should be absent.
	// Content-Type is in the allowlist and not sensitive, so it should be present.
	if got["Content-Type"] != "text/html" {
		t.Errorf("Content-Type = %q, want %q", got["Content-Type"], "text/html")
	}
	if _, ok := got["Set-Cookie"]; ok {
		t.Errorf("Set-Cookie should not be in response_headers (not in allowlist)")
	}
}

func TestHTTPResponseHeadersEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only set headers that are NOT in the allowlist.
		w.Header().Set("X-Custom-Only", "value")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok; error = %v", result.Status, result.Error)
	}

	got := responseHeadersObs(t, result)
	// httptest may set Content-Type automatically, so we check the map exists
	// and does not contain our custom header.
	if _, ok := got["X-Custom-Only"]; ok {
		t.Errorf("X-Custom-Only should not be in response_headers")
	}
}

func TestHTTPResponseHeadersNotInAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "should-be-excluded")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok; error = %v", result.Status, result.Error)
	}

	got := responseHeadersObs(t, result)
	if _, ok := got["X-Custom-Header"]; ok {
		t.Errorf("X-Custom-Header should not be in response_headers (not in allowlist)")
	}
	if got["Content-Type"] != "text/plain" {
		t.Errorf("Content-Type = %q, want %q", got["Content-Type"], "text/plain")
	}
}

func TestHTTPResponseHeadersMultiValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-cache")
		w.Header().Add("Cache-Control", "no-store")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	layer := New(srv.Client())
	pctx := makePctx(srv.URL)
	pctx.Target = targetFromURL(t, srv.URL)
	result := layer.Probe(pctx)

	if result.Status != core.StatusOK {
		t.Fatalf("status = %q, want ok; error = %v", result.Status, result.Error)
	}

	got := responseHeadersObs(t, result)
	val, ok := got["Cache-Control"]
	if !ok {
		t.Fatal("missing Cache-Control in response_headers")
	}
	if val != "no-cache, no-store" {
		t.Errorf("Cache-Control = %q, want %q", val, "no-cache, no-store")
	}
}

// targetFromURL parses an httptest server URL into a Target.
func targetFromURL(t *testing.T, rawURL string) core.Target {
	t.Helper()
	tgt, err := core.ParseTarget(rawURL)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	return tgt
}
