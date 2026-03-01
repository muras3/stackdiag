package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStatusValues(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusOK, "ok"},
		{StatusWarn, "warn"},
		{StatusFail, "fail"},
		{StatusSkip, "skip"},
	}
	for _, tt := range tests {
		if got := string(tt.status); got != tt.want {
			t.Errorf("Status = %q, want %q", got, tt.want)
		}
	}
}

func TestStatusIsOK(t *testing.T) {
	if !StatusOK.IsOK() {
		t.Error("StatusOK.IsOK() should be true")
	}
	if StatusFail.IsOK() {
		t.Error("StatusFail.IsOK() should be false")
	}
	if StatusWarn.IsOK() {
		t.Error("StatusWarn.IsOK() should be false")
	}
	if StatusSkip.IsOK() {
		t.Error("StatusSkip.IsOK() should be false")
	}
}

func TestStatusShouldContinue(t *testing.T) {
	if !StatusOK.ShouldContinue() {
		t.Error("StatusOK should continue")
	}
	if !StatusWarn.ShouldContinue() {
		t.Error("StatusWarn should continue")
	}
	if StatusFail.ShouldContinue() {
		t.Error("StatusFail should not continue")
	}
	if !StatusSkip.ShouldContinue() {
		t.Error("StatusSkip should continue (skip is non-blocking)")
	}
}

func TestProbeError(t *testing.T) {
	pe := &ProbeError{Code: "DNS_NXDOMAIN", Message: "domain not found"}
	if pe.Code != "DNS_NXDOMAIN" {
		t.Errorf("Code = %q, want DNS_NXDOMAIN", pe.Code)
	}
	if pe.Error() != "DNS_NXDOMAIN: domain not found" {
		t.Errorf("Error() = %q", pe.Error())
	}
}

func TestLayerResult(t *testing.T) {
	lr := &LayerResult{
		Status:       StatusOK,
		DurationMS:   9,
		Observations: map[string]any{"answers": []string{"1.2.3.4"}},
		Error:        nil,
	}
	if lr.Status != StatusOK {
		t.Errorf("Status = %q, want ok", lr.Status)
	}
	if lr.DurationMS != 9 {
		t.Errorf("DurationMS = %v, want 9", lr.DurationMS)
	}
}

func TestResult(t *testing.T) {
	now := time.Now().UTC()
	r := &Result{
		SchemaVersion: "v0.1",
		StartedAt:     now,
		Target:        "https://example.com",
		Layers:        make(map[string]*LayerResult),
		Summary: &Summary{
			WallClockMS:     100,
			FirstNonOKLayer: "",
			ExitCode:        0,
		},
	}
	if r.SchemaVersion != "v0.1" {
		t.Errorf("SchemaVersion = %q", r.SchemaVersion)
	}
	if r.Summary.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", r.Summary.ExitCode)
	}
}

func TestAttemptResultMarshalJSON(t *testing.T) {
	ar := &AttemptResult{
		Attempt:   1,
		StartedAt: time.Date(2026, 2, 27, 8, 0, 0, 0, time.UTC),
		Layers: map[string]*LayerResult{
			"http":         {Status: StatusOK, DurationMS: 57, Observations: map[string]any{}, Error: nil},
			"dns":          {Status: StatusOK, DurationMS: 9, Observations: map[string]any{}, Error: nil},
			"reachability": {Status: StatusOK, DurationMS: 4, Observations: map[string]any{}, Error: nil},
			"tls":          {Status: StatusOK, DurationMS: 31, Observations: map[string]any{}, Error: nil},
			"tcp":          {Status: StatusOK, DurationMS: 16, Observations: map[string]any{}, Error: nil},
		},
		Summary: &Summary{WallClockMS: 122, ExitCode: 0},
	}

	b, err := json.Marshal(ar)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	// Verify layer order: dns→reachability→tcp→tls→http
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	s := string(b)
	dnsIdx := indexOf(s, `"dns"`)
	reachIdx := indexOf(s, `"reachability"`)
	tcpIdx := indexOf(s, `"tcp"`)
	tlsIdx := indexOf(s, `"tls"`)
	httpIdx := indexOf(s, `"http"`)

	if dnsIdx >= reachIdx || reachIdx >= tcpIdx || tcpIdx >= tlsIdx || tlsIdx >= httpIdx {
		t.Errorf("layers not in order dns→reachability→tcp→tls→http in JSON: %s", s)
	}
}

func TestCountResultMarshalJSON(t *testing.T) {
	p50 := 9.5
	cr := &CountResult{
		SchemaVersion: "v0.1",
		Target:        "https://example.com",
		Count:         1,
		ExitCode:      0,
		Attempts: []*AttemptResult{
			{
				Attempt:   1,
				StartedAt: time.Date(2026, 2, 27, 8, 0, 0, 0, time.UTC),
				Layers: map[string]*LayerResult{
					"dns": {Status: StatusOK, DurationMS: 9, Observations: map[string]any{}, Error: nil},
				},
				Summary: &Summary{WallClockMS: 9, ExitCode: 0},
			},
		},
		Statistics: map[string]*LayerStatistics{
			"dns": {P50MS: &p50, SuccessCount: 1, SampleCount: 1},
		},
	}

	b, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	// Verify it contains expected fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	for _, key := range []string{"schema_version", "target", "count", "exit_code", "attempts", "statistics"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing key %q in JSON output", key)
		}
	}
}

func TestCountResultStatisticsOrder(t *testing.T) {
	cr := &CountResult{
		SchemaVersion: "v0.1",
		Target:        "https://example.com",
		Count:         1,
		ExitCode:      0,
		Attempts:      []*AttemptResult{},
		Statistics: map[string]*LayerStatistics{
			"http":         {SampleCount: 1},
			"dns":          {SampleCount: 1},
			"reachability": {SampleCount: 1},
			"tls":          {SampleCount: 1},
			"tcp":          {SampleCount: 1},
		},
	}

	b, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	s := string(b)
	dnsIdx := indexOf(s, `"dns"`)
	reachIdx := indexOf(s, `"reachability"`)
	tcpIdx := indexOf(s, `"tcp"`)
	tlsIdx := indexOf(s, `"tls"`)
	httpIdx := indexOf(s, `"http"`)

	if dnsIdx >= reachIdx || reachIdx >= tcpIdx || tcpIdx >= tlsIdx || tlsIdx >= httpIdx {
		t.Errorf("statistics not in order dns→reachability→tcp→tls→http in JSON: %s", s)
	}
}

func TestLayerStatisticsNilPercentilesEmitNull(t *testing.T) {
	ls := &LayerStatistics{
		SuccessCount: 0,
		FailCount:    3,
		SampleCount:  3,
		LossRatio:    1.0,
	}

	b, err := json.Marshal(ls)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	s := string(b)
	if !strings.Contains(s, `"p50_ms":null`) {
		t.Errorf("nil p50_ms should emit null, got: %s", s)
	}
	if !strings.Contains(s, `"p95_ms":null`) {
		t.Errorf("nil p95_ms should emit null, got: %s", s)
	}
}

func TestSchemaVersionV02(t *testing.T) {
	if SchemaVersion != "v0.1" {
		t.Errorf("SchemaVersion = %q, want v0.1", SchemaVersion)
	}
}

func TestLayerOrderV02(t *testing.T) {
	expected := []string{"dns", "reachability", "tcp", "tls", "http"}
	if len(LayerOrder) != len(expected) {
		t.Fatalf("LayerOrder length = %d, want %d", len(LayerOrder), len(expected))
	}
	for i, name := range expected {
		if LayerOrder[i] != name {
			t.Errorf("LayerOrder[%d] = %q, want %q", i, LayerOrder[i], name)
		}
	}
}

func TestResultMarshalJSONV02(t *testing.T) {
	now := time.Now().UTC()
	r := &Result{
		SchemaVersion: "v0.1",
		StartedAt:     now,
		Target:        "https://example.com",
		Layers: map[string]*LayerResult{
			"http":         {Status: StatusOK, DurationMS: 57, Observations: map[string]any{}, Error: nil},
			"dns":          {Status: StatusOK, DurationMS: 9, Observations: map[string]any{}, Error: nil},
			"reachability": {Status: StatusOK, DurationMS: 4, Observations: map[string]any{}, Error: nil},
			"tls":          {Status: StatusOK, DurationMS: 31, Observations: map[string]any{}, Error: nil},
			"tcp":          {Status: StatusOK, DurationMS: 16, Observations: map[string]any{}, Error: nil},
		},
		Summary: &Summary{WallClockMS: 122, ExitCode: 0},
	}

	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	s := string(b)
	dnsIdx := indexOf(s, `"dns"`)
	reachIdx := indexOf(s, `"reachability"`)
	tcpIdx := indexOf(s, `"tcp"`)
	tlsIdx := indexOf(s, `"tls"`)
	httpIdx := indexOf(s, `"http"`)

	if dnsIdx >= reachIdx || reachIdx >= tcpIdx || tcpIdx >= tlsIdx || tlsIdx >= httpIdx {
		t.Errorf("layers not in order dns→reachability→tcp→tls→http in JSON: %s", s)
	}
}

// indexOf returns the position of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
