package core

import (
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
	if StatusSkip.ShouldContinue() {
		t.Error("StatusSkip should not continue")
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
