package core

import (
	"context"
	"fmt"
	"time"
)

// Status represents the outcome of a layer probe.
type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// IsOK returns true only for StatusOK.
func (s Status) IsOK() bool { return s == StatusOK }

// ShouldContinue returns true if the runner should proceed to the next layer.
func (s Status) ShouldContinue() bool { return s == StatusOK || s == StatusWarn }

// ProbeError is a structured error with a machine-readable code.
type ProbeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ProbeError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// LayerResult holds the outcome of a single layer probe.
type LayerResult struct {
	Status       Status         `json:"status"`
	DurationMS   float64        `json:"duration_ms"`
	Observations map[string]any `json:"observations"`
	Error        *ProbeError    `json:"error"`
}

// Summary holds aggregate information about the probe run.
type Summary struct {
	WallClockMS     float64 `json:"wall_clock_ms"`
	FirstNonOKLayer string  `json:"first_non_ok_layer"`
	ExitCode        int     `json:"exit_code"`
}

// Result is the top-level probe output. Internal data model = JSON output.
type Result struct {
	SchemaVersion string                  `json:"schema_version"`
	StartedAt     time.Time               `json:"started_at"`
	Target        string                  `json:"target"`
	Layers        map[string]*LayerResult `json:"layers"`
	Summary       *Summary                `json:"summary"`
}

// ProbeContext carries shared state through the layer execution pipeline.
type ProbeContext struct {
	Context     context.Context
	Target      Target
	ResolvedIPs []string
	Insecure    bool
	Method      string
	Headers     map[string]string
}

// Layer is the interface that each network layer must implement.
type Layer interface {
	Name() string
	Probe(ctx *ProbeContext) *LayerResult
}
