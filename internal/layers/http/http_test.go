package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/muras3/probe/internal/core"
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
	layer := NewDefault(false)
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

	sc, ok := result.Observations["status_code"]
	if !ok {
		t.Fatal("missing status_code observation")
	}
	if sc.(int) != 200 {
		t.Errorf("status_code = %v, want 200", sc)
	}
	method, ok := result.Observations["method"]
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
	if result.Error == nil || result.Error.Code != "HTTP_5XX" {
		t.Errorf("error = %v, want HTTP_5XX", result.Error)
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
	if m, ok := result.Observations["method"]; !ok || m.(string) != "POST" {
		t.Errorf("method observation = %v, want POST", result.Observations["method"])
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
