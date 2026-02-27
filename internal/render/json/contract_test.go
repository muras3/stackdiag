package json

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/muras3/probe/internal/core"
)

// contractResult returns a full Result with all layers for contract testing.
func contractResult() *core.Result {
	return &core.Result{
		SchemaVersion: "v0.1",
		StartedAt:     time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
		Target:        "https://api.example.com/health",
		Layers: map[string]*core.LayerResult{
			"dns": {
				Status:       core.StatusOK,
				DurationMS:   9,
				Observations: map[string]any{"query_name": "api.example.com", "answers": []string{"203.0.113.10"}},
				Error:        nil,
			},
			"tcp": {
				Status:       core.StatusOK,
				DurationMS:   16,
				Observations: map[string]any{"remote_ip": "203.0.113.10", "remote_port": 443},
				Error:        nil,
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

func TestContractTopLevelFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	required := []string{"schema_version", "started_at", "target", "layers", "summary"}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			t.Errorf("missing required top-level field: %q", key)
		}
	}
}

func TestContractLayerFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers, ok := m["layers"].(map[string]any)
	if !ok {
		t.Fatal("layers is not an object")
	}

	requiredLayers := []string{"dns", "tcp", "tls", "http"}
	for _, name := range requiredLayers {
		layer, ok := layers[name].(map[string]any)
		if !ok {
			t.Errorf("missing layer: %q", name)
			continue
		}
		for _, field := range []string{"status", "duration_ms", "observations", "error"} {
			if _, exists := layer[field]; !exists {
				t.Errorf("layer %q missing field: %q", name, field)
			}
		}
	}
}

func TestContractStatusValues(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers := m["layers"].(map[string]any)
	validStatuses := map[string]bool{"ok": true, "warn": true, "fail": true, "skip": true}

	for name, raw := range layers {
		layer := raw.(map[string]any)
		status, ok := layer["status"].(string)
		if !ok {
			t.Errorf("layer %q status is not a string", name)
			continue
		}
		if !validStatuses[status] {
			t.Errorf("layer %q has invalid status: %q", name, status)
		}
	}
}

func TestContractSummaryFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	summary, ok := m["summary"].(map[string]any)
	if !ok {
		t.Fatal("summary is not an object")
	}

	for _, field := range []string{"wall_clock_ms", "first_non_ok_layer", "exit_code"} {
		if _, exists := summary[field]; !exists {
			t.Errorf("summary missing field: %q", field)
		}
	}
}
