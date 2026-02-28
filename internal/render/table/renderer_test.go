package table

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

// allOKResult returns a Result where every layer succeeds.
func allOKResult() *core.Result {
	return &core.Result{
		SchemaVersion: "v0.2",
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
			"reachability": {
				Status:       core.StatusOK,
				DurationMS:   2,
				Observations: map[string]any{"method": "icmp"},
				Error:        nil,
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
		SchemaVersion: "v0.2",
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
			"reachability": {
				Status:       core.StatusOK,
				DurationMS:   2,
				Observations: map[string]any{"method": "icmp"},
				Error:        nil,
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
		SchemaVersion: "v0.2",
		StartedAt:     time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
		Target:        "https://nonexistent.example.com/health",
		Layers: map[string]*core.LayerResult{
			"dns": {
				Status:       core.StatusFail,
				DurationMS:   12,
				Observations: map[string]any{"query_name": "nonexistent.example.com"},
				Error:        &core.ProbeError{Code: "DNS_NXDOMAIN", Message: "NXDOMAIN"},
			},
			"reachability": {
				Status:       core.StatusSkip,
				DurationMS:   0,
				Observations: map[string]any{},
				Error:        nil,
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

	// Layer order must be dns, reachability, tcp, tls, http.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 6 { // 5 layer lines + 1 blank + 1 summary (at minimum)
		t.Fatalf("expected at least 6 lines, got %d:\n%s", len(lines), out)
	}

	// Check each layer line contains the expected status symbol and description.
	assertContains(t, lines[0], "dns")
	assertContains(t, lines[0], "\u2713") // ✓
	assertContains(t, lines[0], "9ms")
	assertContains(t, lines[0], "api.example.com")
	assertContains(t, lines[0], "203.0.113.10")

	assertContains(t, lines[1], "reac")
	assertContains(t, lines[1], "\u2713")
	assertContains(t, lines[1], "2ms")

	assertContains(t, lines[2], "tcp")
	assertContains(t, lines[2], "\u2713")
	assertContains(t, lines[2], "16ms")
	assertContains(t, lines[2], ":443")

	assertContains(t, lines[3], "tls")
	assertContains(t, lines[3], "\u2713")
	assertContains(t, lines[3], "31ms")
	assertContains(t, lines[3], "TLSv1.3")
	assertContains(t, lines[3], "90d")

	assertContains(t, lines[4], "http")
	assertContains(t, lines[4], "\u2713")
	assertContains(t, lines[4], "57ms")
	assertContains(t, lines[4], "200")

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

	// dns, reachability, tcp should be ok.
	assertContains(t, lines[0], "dns")
	assertContains(t, lines[0], "\u2713")

	assertContains(t, lines[1], "reac")
	assertContains(t, lines[1], "\u2713")

	assertContains(t, lines[2], "tcp")
	assertContains(t, lines[2], "\u2713")

	// tls should be warn.
	assertContains(t, lines[3], "tls")
	assertContains(t, lines[3], "\u26a0") // ⚠
	assertContains(t, lines[3], "31ms")
	assertContains(t, lines[3], "TLSv1.3")
	assertContains(t, lines[3], "5d")

	// http should be fail.
	assertContains(t, lines[4], "http")
	assertContains(t, lines[4], "\u2717") // ✗
	assertContains(t, lines[4], "57ms")
	assertContains(t, lines[4], "503")
	assertContains(t, lines[4], "Service Unavailable")

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

	// reachability, tcp, tls, http should be skip.
	for _, i := range []int{1, 2, 3, 4} {
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
	if strings.Contains(out, "\u2713") || strings.Contains(out, "\u26a0") || strings.Contains(out, "\u2717") || strings.Contains(out, "\u2192") {
		t.Error("ASCII mode must not contain Unicode symbols")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")

	// dns/reachability/tcp should use [ok].
	assertContains(t, lines[0], "[ok]")
	assertContains(t, lines[1], "[ok]")
	assertContains(t, lines[2], "[ok]")

	// tls should use [!!].
	assertContains(t, lines[3], "[!!]")

	// http should use [FAIL].
	assertContains(t, lines[4], "[FAIL]")
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

	// reachability, tcp, tls, http should use [skip].
	for _, i := range []int{1, 2, 3, 4} {
		assertContains(t, lines[i], "[skip]")
	}
}

func TestRenderASCIIArrowFallback(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, allOKResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// DNS line should use ASCII arrow "->", not Unicode "→".
	assertContains(t, lines[0], "->")
}

func TestRenderLayerOrder(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, allOKResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) < 5 {
		t.Fatalf("expected at least 5 layer lines, got %d", len(lines))
	}

	expected := []string{"dns", "reac", "tcp", "tls", "http"}
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
	for i := 0; i < 5 && i < len(lines); i++ {
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

// tlsScanResult returns a Result with tls_scan observations.
func tlsScanResult() *core.Result {
	r := allOKResult()
	r.Layers["tls"].Observations["tls_scan"] = map[string]any{
		"performed": true,
		"attempts": []any{
			map[string]any{"version": "TLSv1.0", "supported": true},
			map[string]any{"version": "TLSv1.1", "supported": false},
			map[string]any{"version": "TLSv1.2", "supported": true},
			map[string]any{"version": "TLSv1.3", "supported": true},
		},
		"supported_versions":          []any{"TLSv1.0", "TLSv1.2", "TLSv1.3"},
		"deprecated_versions_enabled": []any{"TLSv1.0"},
	}
	return r
}

// basicCountResult returns a CountResult with 3 attempts, all OK.
func basicCountResult() *core.CountResult {
	p50dns := 9.0
	p95dns := 12.0
	p50reach := 2.0
	p95reach := 3.0
	p50tcp := 16.0
	p95tcp := 18.0
	p50tls := 31.0
	p95tls := 33.0
	p50http := 55.0
	p95http := 60.0

	return &core.CountResult{
		SchemaVersion: "v0.2",
		Target:        "https://api.example.com/health",
		Count:         3,
		ExitCode:      0,
		Attempts: []*core.AttemptResult{
			{
				Attempt:   1,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusOK, DurationMS: 9, Observations: map[string]any{"query_name": "api.example.com", "answers": []any{"1.2.3.4"}}},
					"reachability": {Status: core.StatusOK, DurationMS: 2, Observations: map[string]any{"method": "icmp"}},
					"tcp":          {Status: core.StatusOK, DurationMS: 16, Observations: map[string]any{"remote_port": 443}},
					"tls":          {Status: core.StatusOK, DurationMS: 31, Observations: map[string]any{"version": "TLSv1.3"}},
					"http":         {Status: core.StatusOK, DurationMS: 57, Observations: map[string]any{"status_code": 200, "status_text": "OK"}},
				},
				Summary: &core.Summary{WallClockMS: 113, ExitCode: 0},
			},
			{
				Attempt:   2,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 4, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusOK, DurationMS: 10, Observations: map[string]any{"query_name": "api.example.com", "answers": []any{"1.2.3.4"}}},
					"reachability": {Status: core.StatusOK, DurationMS: 2, Observations: map[string]any{"method": "icmp"}},
					"tcp":          {Status: core.StatusOK, DurationMS: 15, Observations: map[string]any{"remote_port": 443}},
					"tls":          {Status: core.StatusOK, DurationMS: 30, Observations: map[string]any{"version": "TLSv1.3"}},
					"http":         {Status: core.StatusOK, DurationMS: 55, Observations: map[string]any{"status_code": 200, "status_text": "OK"}},
				},
				Summary: &core.Summary{WallClockMS: 110, ExitCode: 0},
			},
			{
				Attempt:   3,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 5, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusOK, DurationMS: 8, Observations: map[string]any{"query_name": "api.example.com", "answers": []any{"1.2.3.4"}}},
					"reachability": {Status: core.StatusOK, DurationMS: 3, Observations: map[string]any{"method": "icmp"}},
					"tcp":          {Status: core.StatusOK, DurationMS: 17, Observations: map[string]any{"remote_port": 443}},
					"tls":          {Status: core.StatusOK, DurationMS: 32, Observations: map[string]any{"version": "TLSv1.3"}},
					"http":         {Status: core.StatusOK, DurationMS: 60, Observations: map[string]any{"status_code": 200, "status_text": "OK"}},
				},
				Summary: &core.Summary{WallClockMS: 117, ExitCode: 0},
			},
		},
		Statistics: map[string]*core.LayerStatistics{
			"dns":          {P50MS: &p50dns, P95MS: &p95dns, SuccessCount: 3, FailCount: 0, SkipCount: 0, SampleCount: 3, LossRatio: 0.0},
			"reachability": {P50MS: &p50reach, P95MS: &p95reach, SuccessCount: 3, FailCount: 0, SkipCount: 0, SampleCount: 3, LossRatio: 0.0},
			"tcp":          {P50MS: &p50tcp, P95MS: &p95tcp, SuccessCount: 3, FailCount: 0, SkipCount: 0, SampleCount: 3, LossRatio: 0.0},
			"tls":          {P50MS: &p50tls, P95MS: &p95tls, SuccessCount: 3, FailCount: 0, SkipCount: 0, SampleCount: 3, LossRatio: 0.0},
			"http":         {P50MS: &p50http, P95MS: &p95http, SuccessCount: 3, FailCount: 0, SkipCount: 0, SampleCount: 3, LossRatio: 0.0},
		},
	}
}

// allFailCountResult returns a CountResult where all attempts fail at DNS.
func allFailCountResult() *core.CountResult {
	return &core.CountResult{
		SchemaVersion: "v0.2",
		Target:        "https://nonexistent.example.com/health",
		Count:         2,
		ExitCode:      1,
		Attempts: []*core.AttemptResult{
			{
				Attempt:   1,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusFail, DurationMS: 12, Observations: map[string]any{"query_name": "nonexistent.example.com"}, Error: &core.ProbeError{Code: "DNS_NXDOMAIN", Message: "NXDOMAIN"}},
					"reachability": {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
					"tcp":          {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
					"tls":          {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
					"http":         {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
				},
				Summary: &core.Summary{WallClockMS: 12, FirstNonOKLayer: "dns", ExitCode: 1},
			},
			{
				Attempt:   2,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 4, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusFail, DurationMS: 15, Observations: map[string]any{"query_name": "nonexistent.example.com"}, Error: &core.ProbeError{Code: "DNS_NXDOMAIN", Message: "NXDOMAIN"}},
					"reachability": {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
					"tcp":          {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
					"tls":          {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
					"http":         {Status: core.StatusSkip, DurationMS: 0, Observations: map[string]any{}},
				},
				Summary: &core.Summary{WallClockMS: 15, FirstNonOKLayer: "dns", ExitCode: 1},
			},
		},
		Statistics: map[string]*core.LayerStatistics{
			"dns":          {P50MS: nil, P95MS: nil, SuccessCount: 0, FailCount: 2, SkipCount: 0, SampleCount: 2, LossRatio: 1.0},
			"reachability": {P50MS: nil, P95MS: nil, SuccessCount: 0, FailCount: 0, SkipCount: 2, SampleCount: 2, LossRatio: 0.0},
			"tcp":          {P50MS: nil, P95MS: nil, SuccessCount: 0, FailCount: 0, SkipCount: 2, SampleCount: 2, LossRatio: 0.0},
			"tls":          {P50MS: nil, P95MS: nil, SuccessCount: 0, FailCount: 0, SkipCount: 2, SampleCount: 2, LossRatio: 0.0},
			"http":         {P50MS: nil, P95MS: nil, SuccessCount: 0, FailCount: 0, SkipCount: 2, SampleCount: 2, LossRatio: 0.0},
		},
	}
}

func TestRenderTLSScanLine(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, tlsScanResult(), true)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Find the scan sub-line (should be right after the tls line).
	var scanLine string
	for i, line := range lines {
		if strings.Contains(line, "tls") && strings.Contains(line, "ms") {
			if i+1 < len(lines) {
				scanLine = lines[i+1]
			}
			break
		}
	}

	if scanLine == "" {
		t.Fatalf("expected scan sub-line after tls line, got none:\n%s", out)
	}

	assertContains(t, scanLine, "scan:")
	assertContains(t, scanLine, "TLSv1.0")
	assertContains(t, scanLine, "\u26a0") // ⚠ for deprecated+supported
	assertContains(t, scanLine, "TLSv1.1")
	assertContains(t, scanLine, "\u2717") // ✗ for unsupported
	assertContains(t, scanLine, "TLSv1.2")
	assertContains(t, scanLine, "TLSv1.3")
}

func TestRenderTLSScanLineASCII(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, tlsScanResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()

	// Must NOT contain ANSI escape codes or Unicode symbols.
	if strings.Contains(out, "\033[") {
		t.Error("ASCII mode must not contain ANSI escape codes")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Find the scan sub-line.
	var scanLine string
	for i, line := range lines {
		if strings.Contains(line, "tls") && strings.Contains(line, "ms") {
			if i+1 < len(lines) {
				scanLine = lines[i+1]
			}
			break
		}
	}

	if scanLine == "" {
		t.Fatalf("expected scan sub-line after tls line, got none:\n%s", out)
	}

	assertContains(t, scanLine, "scan:")
	assertContains(t, scanLine, "[!!]")   // deprecated+supported
	assertContains(t, scanLine, "[FAIL]") // unsupported
	assertContains(t, scanLine, "[ok]")   // supported+not deprecated
}

func TestRenderNoTLSScanLine(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, allOKResult(), true)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()

	if strings.Contains(out, "scan:") {
		t.Error("expected no scan sub-line when tls_scan is absent")
	}
}

func TestRenderCountBasic(t *testing.T) {
	var buf bytes.Buffer
	err := RenderCount(&buf, basicCountResult(), true)
	if err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	out := buf.String()

	// Verify attempt headers.
	assertContains(t, out, "Attempt 1/3")
	assertContains(t, out, "Attempt 2/3")
	assertContains(t, out, "Attempt 3/3")

	// Verify statistics section.
	assertContains(t, out, "Statistics (3 attempts)")

	// Verify p50/p95 values appear.
	assertContains(t, out, "p50:")
	assertContains(t, out, "p95:")
	assertContains(t, out, "loss:")

	// Verify exit code.
	assertContains(t, out, "exit 0")

	// Verify layer lines appear for each attempt.
	lines := strings.Split(out, "\n")
	dnsCount := 0
	for _, line := range lines {
		if strings.Contains(line, "dns") && strings.Contains(line, "ms") {
			dnsCount++
		}
	}
	if dnsCount < 3 {
		t.Errorf("expected at least 3 dns layer lines (one per attempt), got %d", dnsCount)
	}
}

func TestRenderCountASCII(t *testing.T) {
	var buf bytes.Buffer
	err := RenderCount(&buf, basicCountResult(), false)
	if err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	out := buf.String()

	// Must NOT contain ANSI escape codes or Unicode symbols.
	if strings.Contains(out, "\033[") {
		t.Error("ASCII mode must not contain ANSI escape codes")
	}
	if strings.Contains(out, "\u2713") || strings.Contains(out, "\u26a0") || strings.Contains(out, "\u2717") {
		t.Error("ASCII mode must not contain Unicode symbols")
	}

	assertContains(t, out, "Attempt 1/3")
	assertContains(t, out, "[ok]")
	assertContains(t, out, "Statistics (3 attempts)")
}

func TestRenderCountAllFail(t *testing.T) {
	var buf bytes.Buffer
	err := RenderCount(&buf, allFailCountResult(), true)
	if err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	out := buf.String()

	assertContains(t, out, "Attempt 1/2")
	assertContains(t, out, "Attempt 2/2")
	assertContains(t, out, "Statistics (2 attempts)")

	// p50/p95 should show "-" for DNS (all failed).
	// Find the statistics section.
	statsIdx := strings.Index(out, "Statistics")
	if statsIdx < 0 {
		t.Fatal("expected Statistics section")
	}
	statsSection := out[statsIdx:]

	// DNS line in statistics should contain "-" for p50/p95.
	statsLines := strings.Split(statsSection, "\n")
	var dnsStatsLine string
	for _, line := range statsLines {
		if strings.Contains(line, "dns") {
			dnsStatsLine = line
			break
		}
	}
	if dnsStatsLine == "" {
		t.Fatal("expected dns line in statistics section")
	}
	assertContains(t, dnsStatsLine, "-")

	// Loss ratio for DNS should show 100.0%.
	assertContains(t, dnsStatsLine, "100.0%")

	// Exit code.
	assertContains(t, out, "exit 1")
}

// reachabilityOKResult returns a Result where reachability succeeds with full observations.
func reachabilityOKResult() *core.Result {
	r := allOKResult()
	r.Layers["reachability"] = &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: 3,
		Observations: map[string]any{
			"probe_method": "icmp",
			"reachable":    true,
			"rtt_ms":       2.5,
		},
	}
	return r
}

func TestRenderReachabilityOK(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, reachabilityOKResult(), false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Reachability is line index 1 (after dns).
	assertContains(t, lines[1], "reachability")
	assertContains(t, lines[1], "[ok]")
	assertContains(t, lines[1], "3ms")
	assertContains(t, lines[1], "icmp")
}

func TestRenderReachabilitySkip(t *testing.T) {
	r := allOKResult()
	r.Layers["reachability"] = &core.LayerResult{
		Status:       core.StatusSkip,
		DurationMS:   0,
		Observations: map[string]any{"skip_reason": "permission_denied"},
	}

	var buf bytes.Buffer
	err := Render(&buf, r, false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	assertContains(t, lines[1], "reachability")
	assertContains(t, lines[1], "[skip]")
	assertContains(t, lines[1], "permission denied")
}

func TestRenderReachabilityFail(t *testing.T) {
	r := allOKResult()
	r.Layers["reachability"] = &core.LayerResult{
		Status:       core.StatusFail,
		DurationMS:   5,
		Observations: map[string]any{},
		Error:        &core.ProbeError{Code: "REACHABILITY_TIMEOUT", Message: "host unreachable (timeout)"},
	}

	var buf bytes.Buffer
	err := Render(&buf, r, false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	assertContains(t, lines[1], "reachability")
	assertContains(t, lines[1], "[FAIL]")
	assertContains(t, lines[1], "host unreachable")
}

func TestRenderTLSExpandedObservations(t *testing.T) {
	r := allOKResult()
	r.Layers["tls"] = &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: 31,
		Observations: map[string]any{
			"version":                "TLSv1.3",
			"cipher_suite":           "TLS_AES_256_GCM_SHA384",
			"cert_days_until_expiry": 90,
			"cert_verified":          true,
			"cert_subject":           "CN=api.example.com",
			"cert_issuer":            "CN=Let's Encrypt Authority X3",
		},
	}

	var buf bytes.Buffer
	err := Render(&buf, r, false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	assertContains(t, out, "TLSv1.3")
	assertContains(t, out, "CN=api.example.com")
	assertContains(t, out, "CN=Let's Encrypt Authority X3")
}

func TestRenderTLSUnverifiedCert(t *testing.T) {
	r := allOKResult()
	r.Layers["tls"] = &core.LayerResult{
		Status:     core.StatusWarn,
		DurationMS: 31,
		Observations: map[string]any{
			"version":                "TLSv1.3",
			"cert_days_until_expiry": 90,
			"cert_verified":          false,
			"cert_subject":           "CN=api.example.com",
		},
		Error: &core.ProbeError{Code: "TLS_CERT_UNVERIFIED", Message: "certificate not verified"},
	}

	var buf bytes.Buffer
	err := Render(&buf, r, false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	assertContains(t, out, "unverified")
}

func TestRenderHTTPResponseHeaders(t *testing.T) {
	r := allOKResult()
	r.Layers["http"] = &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: 57,
		Observations: map[string]any{
			"method":      "GET",
			"status_code": 200,
			"status_text": "OK",
			"response_headers": map[string]any{
				"Content-Type": "application/json",
				"Server":       "nginx",
			},
		},
	}

	var buf bytes.Buffer
	err := Render(&buf, r, false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	assertContains(t, out, "Content-Type: application/json")
	assertContains(t, out, "Server: nginx")
}

func TestRenderDNSErrorHint(t *testing.T) {
	r := allOKResult()
	r.Layers["dns"] = &core.LayerResult{
		Status:     core.StatusWarn,
		DurationMS: 12,
		Observations: map[string]any{
			"query_name":     "api.example.com",
			"answers":        []any{"203.0.113.10"},
			"dns_error_hint": "servfail",
		},
		Error: &core.ProbeError{Code: "DNS_WARN", Message: "partial failure"},
	}

	var buf bytes.Buffer
	err := Render(&buf, r, false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	out := buf.String()
	assertContains(t, out, "hint: servfail")
}

// assertContains is a test helper that checks if s contains substr.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
