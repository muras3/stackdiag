package json

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

func testResult() *core.Result {
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
					"answers":    []string{"203.0.113.10"},
				},
				Error: nil,
			},
			"tcp": {
				Status:       core.StatusOK,
				DurationMS:   16,
				Observations: map[string]any{"remote_ip": "203.0.113.10", "remote_port": 443},
				Error:        nil,
			},
		},
		Summary: &core.Summary{
			WallClockMS:     122,
			FirstNonOKLayer: "",
			ExitCode:        0,
		},
	}
}

func TestRender(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, testResult())
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	// Must be valid JSON.
	var raw json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}

	// Must be compact (single line + trailing newline).
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) != 1 {
		t.Errorf("expected compact single-line JSON, got %d lines", len(lines))
	}
}

func TestRenderIncludesRequestHeaders(t *testing.T) {
	r := testResult()
	r.Layers["http"] = &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: 12,
		Observations: map[string]any{
			"method":          "GET",
			"request_headers": map[string]string{"X-Custom": "abc"},
		},
	}

	var buf bytes.Buffer
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	layers := m["layers"].(map[string]any)
	httpLayer := layers["http"].(map[string]any)
	obs := httpLayer["observations"].(map[string]any)
	if _, ok := obs["request_headers"]; !ok {
		t.Fatal("missing layers.http.observations.request_headers")
	}
}

func TestRenderContainsRedactedTokenNotRawSecret(t *testing.T) {
	r := testResult()
	r.Layers["http"] = &core.LayerResult{
		Status:     core.StatusOK,
		DurationMS: 12,
		Observations: map[string]any{
			"request_headers": map[string]string{
				"Authorization": "[REDACTED]",
			},
		},
	}

	var buf bytes.Buffer
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatal("missing [REDACTED] in JSON")
	}
	if strings.Contains(out, "secret-token") {
		t.Fatal("raw secret leaked in JSON")
	}
}

func testCountResult() *core.CountResult {
	p50dns := 9.0
	p95dns := 12.0
	p50tcp := 16.0
	p95tcp := 20.0
	return &core.CountResult{
		SchemaVersion: "v0.2",
		Target:        "https://api.example.com/health",
		Count:         2,
		ExitCode:      1,
		Attempts: []*core.AttemptResult{
			{
				Attempt:   1,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns": {Status: core.StatusOK, DurationMS: 9, Observations: map[string]any{"query_name": "api.example.com"}, Error: nil},
					"tcp": {Status: core.StatusOK, DurationMS: 16, Observations: map[string]any{"remote_ip": "203.0.113.10"}, Error: nil},
				},
				Summary: &core.Summary{WallClockMS: 25, FirstNonOKLayer: "", ExitCode: 0},
			},
			{
				Attempt:   2,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 4, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns": {Status: core.StatusOK, DurationMS: 12, Observations: map[string]any{"query_name": "api.example.com"}, Error: nil},
					"tcp": {Status: core.StatusFail, DurationMS: 20, Observations: map[string]any{}, Error: &core.ProbeError{Code: "TCP_TIMEOUT", Message: "connection timed out"}},
				},
				Summary: &core.Summary{WallClockMS: 32, FirstNonOKLayer: "tcp", ExitCode: 1},
			},
		},
		Statistics: map[string]*core.LayerStatistics{
			"dns": {P50MS: &p50dns, P95MS: &p95dns, SuccessCount: 2, FailCount: 0, SkipCount: 0, SampleCount: 2, LossRatio: 0},
			"tcp": {P50MS: &p50tcp, P95MS: &p95tcp, SuccessCount: 1, FailCount: 1, SkipCount: 0, SampleCount: 2, LossRatio: 0.5},
		},
	}
}

func TestRenderCount(t *testing.T) {
	var buf bytes.Buffer
	err := RenderCount(&buf, testCountResult())
	if err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	// Must be valid JSON.
	var raw json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}

	// Must be compact (single line + trailing newline).
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) != 1 {
		t.Errorf("expected compact single-line JSON, got %d lines", len(lines))
	}

	// Must have expected top-level structure.
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	for _, key := range []string{"schema_version", "target", "count", "exit_code", "attempts", "statistics"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing top-level field: %q", key)
		}
	}
}

func TestRenderPretty(t *testing.T) {
	var buf bytes.Buffer
	err := RenderPretty(&buf, testResult())
	if err != nil {
		t.Fatalf("RenderPretty error: %v", err)
	}

	// Must be valid JSON.
	var raw json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}

	// Must be multi-line (indented).
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) <= 1 {
		t.Error("expected multi-line pretty-printed JSON")
	}
}

func TestRenderCountPretty(t *testing.T) {
	var buf bytes.Buffer
	err := RenderCountPretty(&buf, testCountResult())
	if err != nil {
		t.Fatalf("RenderCountPretty error: %v", err)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}

	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) <= 1 {
		t.Error("expected multi-line pretty-printed JSON")
	}
}

func TestRenderToolErrorPretty(t *testing.T) {
	var buf bytes.Buffer
	err := RenderToolErrorPretty(&buf, "INVALID_ARGS", "missing target", 1)
	if err != nil {
		t.Fatalf("RenderToolErrorPretty error: %v", err)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) <= 1 {
		t.Error("expected multi-line pretty-printed tool error JSON")
	}
}

func TestRenderNoHTMLEscape(t *testing.T) {
	r := testResult()
	r.Target = "https://example.com?a=1&b=2"

	var buf bytes.Buffer
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	// & should NOT be escaped to \u0026.
	if bytes.Contains(buf.Bytes(), []byte(`\u0026`)) {
		t.Error("HTML escaping should be disabled")
	}
}
