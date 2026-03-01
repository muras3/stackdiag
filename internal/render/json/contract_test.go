package json

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
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
			"reachability": {
				Status:       core.StatusOK,
				DurationMS:   2,
				Observations: map[string]any{"method": "icmp"},
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

	requiredLayers := []string{"dns", "reachability", "tcp", "tls", "http"}
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

// contractResultWithSkip returns a Result with skipped layers for contract testing.
func contractResultWithSkip() *core.Result {
	return &core.Result{
		SchemaVersion: "v0.1",
		StartedAt:     time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
		Target:        "tcp://db.example.com:5432",
		Layers: map[string]*core.LayerResult{
			"dns": {
				Status:       core.StatusOK,
				DurationMS:   9,
				Observations: map[string]any{"query_name": "db.example.com", "answers": []string{"203.0.113.10"}},
				Error:        nil,
			},
			"reachability": {
				Status:       core.StatusOK,
				DurationMS:   2,
				Observations: map[string]any{"method": "icmp"},
				Error:        nil,
			},
			"tcp": {
				Status:       core.StatusOK,
				DurationMS:   16,
				Observations: map[string]any{"remote_ip": "203.0.113.10", "remote_port": 5432},
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
			WallClockMS:     25,
			FirstNonOKLayer: "",
			ExitCode:        0,
		},
	}
}

func TestContractSkipLayerShape(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResultWithSkip()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers := m["layers"].(map[string]any)

	// Schema contract rule 2: skip uses status:"skip", not null.
	for _, name := range []string{"tls", "http"} {
		layer, ok := layers[name].(map[string]any)
		if !ok {
			t.Fatalf("skipped layer %q is missing or null (schema contract rule 2 violation)", name)
		}
		status, ok := layer["status"].(string)
		if !ok || status != "skip" {
			t.Errorf("skipped layer %q status = %v, want \"skip\" (schema contract rule 2)", name, layer["status"])
		}
		// Skipped layers must still have all required fields.
		for _, field := range []string{"status", "duration_ms", "observations", "error"} {
			if _, exists := layer[field]; !exists {
				t.Errorf("skipped layer %q missing field %q (schema contract rule 4: same structure even on skip)", name, field)
			}
		}
		// duration_ms should be 0 for skipped layers.
		if dur, ok := layer["duration_ms"].(float64); ok && dur != 0 {
			t.Errorf("skipped layer %q duration_ms = %v, want 0", name, dur)
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

func TestContractErrorFieldStructure(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers := m["layers"].(map[string]any)
	for name, raw := range layers {
		layer := raw.(map[string]any)
		errField := layer["error"]
		if errField == nil {
			continue
		}
		errObj, ok := errField.(map[string]any)
		if !ok {
			t.Errorf("layer %q error is not an object: %T", name, errField)
			continue
		}
		if _, ok := errObj["code"]; !ok {
			t.Errorf("layer %q error missing 'code' field", name)
		}
		if _, ok := errObj["message"]; !ok {
			t.Errorf("layer %q error missing 'message' field", name)
		}
	}
}

func TestContractStartedAtFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	startedAt, ok := m["started_at"].(string)
	if !ok {
		t.Fatal("started_at is not a string")
	}
	if _, err := time.Parse(time.RFC3339, startedAt); err != nil {
		t.Errorf("started_at %q is not valid RFC3339: %v", startedAt, err)
	}
}

func TestContractDurationMsType(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers := m["layers"].(map[string]any)
	for name, raw := range layers {
		layer := raw.(map[string]any)
		dur, ok := layer["duration_ms"]
		if !ok {
			t.Errorf("layer %q missing duration_ms", name)
			continue
		}
		if _, ok := dur.(float64); !ok {
			t.Errorf("layer %q duration_ms is %T, want number", name, dur)
		}
	}
}

func TestContractLayerOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	// The raw JSON must have layers in execution order: dns, reachability, tcp, tls, http.
	raw := buf.String()
	expectedOrder := []string{"dns", "reachability", "tcp", "tls", "http"}
	lastIdx := -1
	for _, name := range expectedOrder {
		key := `"` + name + `"`
		// Find the key inside the "layers" object.
		idx := strings.Index(raw, `"layers"`)
		if idx < 0 {
			t.Fatal("missing layers key in JSON")
		}
		layersJSON := raw[idx:]
		pos := strings.Index(layersJSON, key)
		if pos < 0 {
			t.Fatalf("layer %q not found in JSON output", name)
		}
		if pos <= lastIdx {
			t.Errorf("layer %q (pos %d) appears before previous layer (pos %d); want execution order: dns→tcp→tls→http", name, pos, lastIdx)
		}
		lastIdx = pos
	}
}

// contractCountResult returns a full CountResult with all layers for contract testing.
func contractCountResult() *core.CountResult {
	p50dns := 9.0
	p95dns := 12.0
	p50reach := 2.0
	p95reach := 3.0
	p50tcp := 16.0
	p95tcp := 20.0
	p50tls := 30.0
	p95tls := 35.0
	p50http := 55.0
	p95http := 60.0
	return &core.CountResult{
		SchemaVersion: "v0.1",
		Target:        "https://api.example.com/health",
		Count:         2,
		ExitCode:      1,
		Attempts: []*core.AttemptResult{
			{
				Attempt:   1,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusOK, DurationMS: 9, Observations: map[string]any{"query_name": "api.example.com"}, Error: nil},
					"reachability": {Status: core.StatusOK, DurationMS: 2, Observations: map[string]any{"method": "icmp"}, Error: nil},
					"tcp":          {Status: core.StatusOK, DurationMS: 16, Observations: map[string]any{"remote_ip": "203.0.113.10"}, Error: nil},
					"tls":          {Status: core.StatusOK, DurationMS: 31, Observations: map[string]any{"version": "TLSv1.3"}, Error: nil},
					"http":         {Status: core.StatusFail, DurationMS: 57, Observations: map[string]any{"status_code": 503}, Error: &core.ProbeError{Code: "HTTP_503", Message: "503 Service Unavailable"}},
				},
				Summary: &core.Summary{WallClockMS: 122, FirstNonOKLayer: "http", ExitCode: 1},
			},
			{
				Attempt:   2,
				StartedAt: time.Date(2026, 2, 26, 18, 42, 4, 0, time.UTC),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusOK, DurationMS: 12, Observations: map[string]any{"query_name": "api.example.com"}, Error: nil},
					"reachability": {Status: core.StatusOK, DurationMS: 3, Observations: map[string]any{"method": "icmp"}, Error: nil},
					"tcp":          {Status: core.StatusOK, DurationMS: 20, Observations: map[string]any{"remote_ip": "203.0.113.10"}, Error: nil},
					"tls":          {Status: core.StatusOK, DurationMS: 35, Observations: map[string]any{"version": "TLSv1.3"}, Error: nil},
					"http":         {Status: core.StatusOK, DurationMS: 60, Observations: map[string]any{"status_code": 200}, Error: nil},
				},
				Summary: &core.Summary{WallClockMS: 127, FirstNonOKLayer: "", ExitCode: 0},
			},
		},
		Statistics: map[string]*core.LayerStatistics{
			"dns":          {P50MS: &p50dns, P95MS: &p95dns, SuccessCount: 2, FailCount: 0, SkipCount: 0, SampleCount: 2, LossRatio: 0},
			"reachability": {P50MS: &p50reach, P95MS: &p95reach, SuccessCount: 2, FailCount: 0, SkipCount: 0, SampleCount: 2, LossRatio: 0},
			"tcp":          {P50MS: &p50tcp, P95MS: &p95tcp, SuccessCount: 2, FailCount: 0, SkipCount: 0, SampleCount: 2, LossRatio: 0},
			"tls":          {P50MS: &p50tls, P95MS: &p95tls, SuccessCount: 2, FailCount: 0, SkipCount: 0, SampleCount: 2, LossRatio: 0},
			"http":         {P50MS: &p50http, P95MS: &p95http, SuccessCount: 1, FailCount: 1, SkipCount: 0, SampleCount: 2, LossRatio: 0.5},
		},
	}
}

func TestContractCountResultTopLevelFields(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderCount(&buf, contractCountResult()); err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	required := []string{"count", "exit_code", "attempts", "statistics"}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			t.Errorf("missing required top-level field: %q", key)
		}
	}
}

func TestContractCountResultAttemptFields(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderCount(&buf, contractCountResult()); err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	attempts, ok := m["attempts"].([]any)
	if !ok {
		t.Fatal("attempts is not an array")
	}
	if len(attempts) == 0 {
		t.Fatal("attempts array is empty")
	}

	for i, raw := range attempts {
		attempt, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("attempt[%d] is not an object", i)
			continue
		}
		for _, field := range []string{"attempt", "started_at", "layers", "summary"} {
			if _, exists := attempt[field]; !exists {
				t.Errorf("attempt[%d] missing field: %q", i, field)
			}
		}
	}
}

func TestContractCountResultLayerOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderCount(&buf, contractCountResult()); err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	raw := buf.String()
	// Find each attempt's layers and verify order.
	expectedOrder := []string{"dns", "reachability", "tcp", "tls", "http"}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	attempts := m["attempts"].([]any)
	for i := range attempts {
		// Find the i-th attempt block in raw JSON and check layer order within it.
		searchFrom := raw
		for skip := 0; skip <= i; skip++ {
			idx := strings.Index(searchFrom, `"attempt"`)
			if idx < 0 {
				t.Fatalf("could not find attempt %d in JSON", i)
			}
			if skip < i {
				searchFrom = searchFrom[idx+len(`"attempt"`):]
			} else {
				searchFrom = searchFrom[idx:]
			}
		}

		lastIdx := -1
		for _, name := range expectedOrder {
			key := `"` + name + `"`
			pos := strings.Index(searchFrom, key)
			if pos < 0 {
				t.Fatalf("attempt[%d]: layer %q not found", i, name)
			}
			if pos <= lastIdx {
				t.Errorf("attempt[%d]: layer %q (pos %d) appears before previous layer (pos %d); want dns→tcp→tls→http", i, name, pos, lastIdx)
			}
			lastIdx = pos
		}
	}
}

func TestContractCountResultStatisticsFields(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderCount(&buf, contractCountResult()); err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	stats, ok := m["statistics"].(map[string]any)
	if !ok {
		t.Fatal("statistics is not an object")
	}

	requiredFields := []string{"p50_ms", "p95_ms", "success_count", "fail_count", "skip_count", "sample_count", "loss_ratio"}
	for layerName, raw := range stats {
		layer, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("statistics[%q] is not an object", layerName)
			continue
		}
		for _, field := range requiredFields {
			if _, exists := layer[field]; !exists {
				t.Errorf("statistics[%q] missing field: %q", layerName, field)
			}
		}
	}
}

func TestContractCountResultStatisticsOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderCount(&buf, contractCountResult()); err != nil {
		t.Fatalf("RenderCount error: %v", err)
	}

	raw := buf.String()
	expectedOrder := []string{"dns", "reachability", "tcp", "tls", "http"}

	// Find the statistics section and verify key order.
	statsIdx := strings.Index(raw, `"statistics"`)
	if statsIdx < 0 {
		t.Fatal("missing statistics key in JSON")
	}
	statsJSON := raw[statsIdx:]

	lastIdx := -1
	for _, name := range expectedOrder {
		key := `"` + name + `"`
		pos := strings.Index(statsJSON, key)
		if pos < 0 {
			t.Fatalf("statistics key %q not found", name)
		}
		if pos <= lastIdx {
			t.Errorf("statistics key %q (pos %d) appears before previous key (pos %d); want dns→tcp→tls→http", name, pos, lastIdx)
		}
		lastIdx = pos
	}
}

func TestContractSchemaVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if v, ok := m["schema_version"].(string); !ok || v != "v0.1" {
		t.Errorf("schema_version = %v, want \"v0.1\"", m["schema_version"])
	}
}

func TestContractFiveLayersPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers := m["layers"].(map[string]any)
	expected := []string{"dns", "reachability", "tcp", "tls", "http"}
	for _, name := range expected {
		if _, ok := layers[name]; !ok {
			t.Errorf("missing layer %q in output", name)
		}
	}
	if len(layers) != 5 {
		t.Errorf("expected 5 layers, got %d", len(layers))
	}
}

// contractResultReachabilitySkip returns a Result where reachability is skipped with permission_denied.
func contractResultReachabilitySkip() *core.Result {
	return &core.Result{
		SchemaVersion: "v0.1",
		StartedAt:     time.Date(2026, 2, 26, 18, 42, 3, 0, time.UTC),
		Target:        "https://api.example.com/health",
		Layers: map[string]*core.LayerResult{
			"dns": {
				Status:       core.StatusOK,
				DurationMS:   9,
				Observations: map[string]any{"query_name": "api.example.com", "answers": []string{"203.0.113.10"}},
			},
			"reachability": {
				Status:       core.StatusSkip,
				DurationMS:   0,
				Observations: map[string]any{"skip_reason": "permission_denied"},
			},
			"tcp": {
				Status:       core.StatusOK,
				DurationMS:   16,
				Observations: map[string]any{"remote_ip": "203.0.113.10", "remote_port": 443},
			},
			"tls": {
				Status:     core.StatusOK,
				DurationMS: 31,
				Observations: map[string]any{
					"version":      "TLSv1.3",
					"cipher_suite": "TLS_AES_256_GCM_SHA384",
				},
			},
			"http": {
				Status:     core.StatusOK,
				DurationMS: 57,
				Observations: map[string]any{
					"method":      "GET",
					"status_code": 200,
				},
			},
		},
		Summary: &core.Summary{
			WallClockMS:     122,
			FirstNonOKLayer: "",
			ExitCode:        0,
		},
	}
}

func TestContractReachabilitySkipRender(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResultReachabilitySkip()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers := m["layers"].(map[string]any)
	reach := layers["reachability"].(map[string]any)
	if reach["status"] != "skip" {
		t.Errorf("reachability status = %v, want \"skip\"", reach["status"])
	}
	obs := reach["observations"].(map[string]any)
	if obs["skip_reason"] != "permission_denied" {
		t.Errorf("reachability skip_reason = %v, want \"permission_denied\"", obs["skip_reason"])
	}
}

func TestContractNewTLSObservations(t *testing.T) {
	r := contractResult()
	tlsObs := r.Layers["tls"].Observations.(map[string]any)
	tlsObs["cert_verified"] = true
	tlsObs["cert_subject"] = "CN=api.example.com"
	tlsObs["cert_san"] = []string{"api.example.com", "*.example.com"}
	tlsObs["cert_issuer"] = "CN=Let's Encrypt Authority X3"
	tlsObs["cert_not_after"] = "2026-03-05T00:00:00Z"
	tlsObs["cert_not_before"] = "2025-12-05T00:00:00Z"
	tlsObs["cert_chain"] = []string{"CN=api.example.com", "CN=Let's Encrypt Authority X3", "CN=DST Root CA X3"}

	var buf bytes.Buffer
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	parsedTLSObs := m["layers"].(map[string]any)["tls"].(map[string]any)["observations"].(map[string]any)
	for _, field := range []string{"cert_verified", "cert_subject", "cert_san", "cert_issuer", "cert_not_after", "cert_not_before", "cert_chain"} {
		if _, ok := parsedTLSObs[field]; !ok {
			t.Errorf("tls observations missing %q", field)
		}
	}
}

func TestContractDNSErrorHint(t *testing.T) {
	r := contractResult()
	r.Layers["dns"].Observations.(map[string]any)["dns_error_hint"] = "servfail"

	var buf bytes.Buffer
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	dnsObs := m["layers"].(map[string]any)["dns"].(map[string]any)["observations"].(map[string]any)
	if v, ok := dnsObs["dns_error_hint"]; !ok || v != "servfail" {
		t.Errorf("dns_error_hint = %v, want \"servfail\"", v)
	}
}

func TestContractHTTPResponseHeaders(t *testing.T) {
	r := contractResult()
	r.Layers["http"].Observations.(map[string]any)["response_headers"] = map[string]string{
		"Content-Type": "application/json",
		"Server":       "nginx",
	}

	var buf bytes.Buffer
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	httpObs := m["layers"].(map[string]any)["http"].(map[string]any)["observations"].(map[string]any)
	headers, ok := httpObs["response_headers"].(map[string]any)
	if !ok {
		t.Fatal("http observations missing response_headers or not an object")
	}
	if headers["Content-Type"] != "application/json" {
		t.Errorf("response_headers Content-Type = %v", headers["Content-Type"])
	}
	if headers["Server"] != "nginx" {
		t.Errorf("response_headers Server = %v", headers["Server"])
	}
}

func TestContractObservations(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, contractResult()); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	layers := m["layers"].(map[string]any)

	// DNS should have query_name.
	dnsObs := layers["dns"].(map[string]any)["observations"].(map[string]any)
	if _, ok := dnsObs["query_name"]; !ok {
		t.Error("dns observations missing query_name")
	}

	// TCP should have remote_ip and remote_port.
	tcpObs := layers["tcp"].(map[string]any)["observations"].(map[string]any)
	for _, field := range []string{"remote_ip", "remote_port"} {
		if _, ok := tcpObs[field]; !ok {
			t.Errorf("tcp observations missing %q", field)
		}
	}

	// TLS should have version, cipher_suite, cert_days_until_expiry.
	tlsObs := layers["tls"].(map[string]any)["observations"].(map[string]any)
	for _, field := range []string{"version", "cipher_suite", "cert_days_until_expiry"} {
		if _, ok := tlsObs[field]; !ok {
			t.Errorf("tls observations missing %q", field)
		}
	}

	// HTTP should have method, protocol, status_code.
	httpObs := layers["http"].(map[string]any)["observations"].(map[string]any)
	for _, field := range []string{"method", "protocol", "status_code"} {
		if _, ok := httpObs[field]; !ok {
			t.Errorf("http observations missing %q", field)
		}
	}
}
