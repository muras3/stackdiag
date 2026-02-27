package json

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/muras3/probe/internal/core"
)

func testResult() *core.Result {
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

	// Must be indented (contains newlines).
	if !bytes.Contains(buf.Bytes(), []byte("\n")) {
		t.Error("expected indented JSON output")
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
