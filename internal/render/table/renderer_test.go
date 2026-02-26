package table

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/muras3/probe/internal/core"
)

// allOKResult returns a Result where every layer succeeds.
func allOKResult() *core.Result {
	return &core.Result{
		SchemaVersion: "v0.1",
		StartedAt:     time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
		Target:        "https://api.example.com/health",
		Layers: map[string]*core.LayerResult{
			"dns": {
				Status:     core.StatusOK,
				DurationMS: 9,
				Observations: map[string]any{
					"query_name": "api.example.com",
					"answers":    []any{"203.0.113.10"},
				},
				Error: nil,
			},
			"tcp": {
				Status:     core.StatusOK,
				DurationMS: 16,
				Observations: map[string]any{
					"remote_ip":   "203.0.113.10",
					"remote_port": 443,
				},
				Error: nil,
			},
			"tls": {
				Status:     core.StatusOK,
				DurationMS: 31,
				Observations: map[string]any{
					"version":                "TLSv1.3",
					"cipher_suite":           "TLS_AES_256_GCM_SHA384",
					"cert_days_until_expiry": 90,
				},
				Error: nil,
			},
			"http": {
				Status:     core.StatusOK,
				DurationMS: 57,
				Observations: map[string]any{
					"method":      "GET",
					"protocol":    "HTTP/2",
					"status_code": 200,
					"status_text": "OK",
				},
				Error: nil,
			},
		},
		Summary: &core.Summary{
			WallClockMS:     122,
			FirstNonOKLayer: "",
			ExitCode:        0,
		},
	}
}

// mixedResult returns a Result with tls warn and http fail.
func mixedResult() *core.Result {
	return &core.Result{
		SchemaVersion: "v0.1",
		StartedAt:     time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
		Target:        "https://api.example.com/health",
		Layers: map[string]*core.LayerResult{
			"dns": {
				Status:     core.StatusOK,
				DurationMS: 9,
				Observations: map[string]any{
					"query_name": "api.example.com",
					"answers":    []any{"203.0.113.10"},
				},
				Error: nil,
			},
			"tcp": {
				Status:     core.StatusOK,
				DurationMS: 16,
				Observations: map[string]any{
					"remote_ip":   "203.0.113.10",
					"remote_port": 443,
				},
				Error: nil,
			},
			"tls": {
				Status:     core.StatusWarn,
				DurationMS: 31,
				Observations: map[string]any{
					"version":                "TLSv1.3",
					"cipher_suite":           "TLS_AES_256_GCM_SHA384",
					"cert_days_until_expiry": 5,
				},
				Error: &core.ProbeError{Code: "TLS_CERT_EXPIRING_SOON", Message: "Certificate expires in 5 days"},
			},
			"http": {
				Status:     core.StatusFail,
				DurationMS: 57,
				Observations: map[string]any{
					"method":      "GET",
					"protocol":    "HTTP/2",
					"status_code": 503,
					"status_text": "Service Unavailable",
				},
				Error: &core.ProbeError{Code: "HTTP_503", Message: "503 Service Unavailable"},
			},
		},
		Summary: &core.Summary{
			WallClockMS:     122,
			FirstNonOKLayer: "tls",
			ExitCode:        1,
		},
	}
}

// dnsFailResult returns a Result where DNS fails and the rest are skipped.
func dnsFailResult() *core.Result {
	return &core.Result{
		SchemaVersion: "v0.1",
		StartedAt:     time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
		Target:        "https://nonexistent.example.com/health",
		Layers: map[string]*core.LayerResult{
			"dns": {
				Status:       core.StatusFail,
				DurationMS:   12,
				Observations: map[string]any{"query_name": "nonexistent.example.com"},
				Error:        &core.ProbeError{Code: "DNS_NXDOMAIN", Message: "NXDOMAIN"},
			},
			"tcp": {
				Status:       core.StatusSkip,
				DurationMS:   0,
				Observations: map[string]any{},
				Error:        nil,
			},
			"tls": {
				Status:       core.StatusSkip,
				DurationMS:   0,
				Observations: map[string]any{},
				Error:        nil,
			},
			"http": {
				Status:       core.StatusSkip,
				DurationMS:   0,
				Observations: map[string]any{},
				Error:        nil,
			},
		},
		Summary: &core.Summary{
			WallClockMS:     12,
			FirstNonOKLayer: "dns",
			ExitCode:        1,
		},
	}
}

func TestRenderAllOK(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, allOKResult(), true)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()

	// Layer order must be dns, tcp, tls, http.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 5 { // 4 layer lines + 1 blank + 1 summary (at minimum)
		t.Fatalf("expected at least 5 lines, got %d:\n%s", len(lines), out)
	}

	// Check each layer line contains the expected status symbol and description.
	assertContains(t, lines[0], "dns")
	assertContains(t, lines[0], "\u2713") // ✓
	assertContains(t, lines[0], "9ms")
	assertContains(t, lines[0], "api.example.com")
	assertContains(t, lines[0], "203.0.113.10")

	assertContains(t, lines[1], "tcp")
	assertContains(t, lines[1], "\u2713")
	assertContains(t, lines[1], "16ms")
	assertContains(t, lines[1], ":443")

	assertContains(t, lines[2], "tls")
	assertContains(t, lines[2], "\u2713")
	assertContains(t, lines[2], "31ms")
	assertContains(t, lines[2], "TLSv1.3")
	assertContains(t, lines[2], "90d")

	assertContains(t, lines[3], "http")
	assertContains(t, lines[3], "\u2713")
	assertContains(t, lines[3], "57ms")
	assertContains(t, lines[3], "200")

	// Summary line: all ok.
	summaryLine := lines[len(lines)-1]
	assertContains(t, summaryLine, "122ms total")
	assertContains(t, summaryLine, "all ok")
}

func TestRenderMixed(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, mixedResult(), true)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// dns and tcp should be ok.
	assertContains(t, lines[0], "dns")
	assertContains(t, lines[0], "\u2713")

	assertContains(t, lines[1], "tcp")
	assertContains(t, lines[1], "\u2713")

	// tls should be warn.
	assertContains(t, lines[2], "tls")
	assertContains(t, lines[2], "\u26a0") // ⚠
	assertContains(t, lines[2], "31ms")
	assertContains(t, lines[2], "TLSv1.3")
	assertContains(t, lines[2], "5d")

	// http should be fail.
	assertContains(t, lines[3], "http")
	assertContains(t, lines[3], "\u2717") // ✗
	assertContains(t, lines[3], "57ms")
	assertContains(t, lines[3], "503")
	assertContains(t, lines[3], "Service Unavailable")

	// Summary line: first issue is tls, exit 1.
	summaryLine := lines[len(lines)-1]
	assertContains(t, summaryLine, "122ms total")
	assertContains(t, summaryLine, "first issue: tls")
	assertContains(t, summaryLine, "exit 1")
}

func TestRenderDNSFail(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, dnsFailResult(), true)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// dns should be fail with error message.
	assertContains(t, lines[0], "dns")
	assertContains(t, lines[0], "\u2717") // ✗
	assertContains(t, lines[0], "12ms")
	assertContains(t, lines[0], "NXDOMAIN")

	// tcp, tls, http should be skip.
	for _, i := range []int{1, 2, 3} {
		assertContains(t, lines[i], "-") // dim dash for skip
	}

	// Summary line: first issue is dns.
	summaryLine := lines[len(lines)-1]
	assertContains(t, summaryLine, "12ms total")
	assertContains(t, summaryLine, "first issue: dns")
	assertContains(t, summaryLine, "exit 1")
}

func TestRenderASCIIFallback(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, mixedResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()

	// Must NOT contain ANSI escape codes.
	if strings.Contains(out, "\033[") {
		t.Error("ASCII mode must not contain ANSI escape codes")
	}

	// Must NOT contain Unicode symbols.
	if strings.Contains(out, "\u2713") || strings.Contains(out, "\u26a0") || strings.Contains(out, "\u2717") {
		t.Error("ASCII mode must not contain Unicode symbols")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")

	// dns/tcp should use [ok].
	assertContains(t, lines[0], "[ok]")
	assertContains(t, lines[1], "[ok]")

	// tls should use [!!].
	assertContains(t, lines[2], "[!!]")

	// http should use [FAIL].
	assertContains(t, lines[3], "[FAIL]")
}

func TestRenderASCIIFallbackSkip(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, dnsFailResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// dns should use [FAIL].
	assertContains(t, lines[0], "[FAIL]")

	// tcp, tls, http should use [skip].
	for _, i := range []int{1, 2, 3} {
		assertContains(t, lines[i], "[skip]")
	}
}

func TestRenderLayerOrder(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, allOKResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 layer lines, got %d", len(lines))
	}

	expected := []string{"dns", "tcp", "tls", "http"}
	for i, name := range expected {
		if !strings.Contains(lines[i], name) {
			t.Errorf("line %d: expected layer %q, got %q", i, name, lines[i])
		}
	}
}

func TestRenderTimingAlignment(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, allOKResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Verify that all layer lines contain timing values.
	// And that the character immediately after the "ms" suffix is a space (consistent padding).
	for i := 0; i < 4 && i < len(lines); i++ {
		idx := strings.Index(lines[i], "ms")
		if idx < 0 {
			t.Errorf("line %d: missing 'ms' timing", i)
			continue
		}
		// After "ms" there should be spaces (separator before description).
		endIdx := idx + 2
		if endIdx < len(lines[i]) && lines[i][endIdx] != ' ' {
			t.Errorf("line %d: expected space after 'ms' at position %d, got %q",
				i, endIdx, lines[i][endIdx])
		}
	}
}

// assertContains is a test helper that checks if s contains substr.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
