package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// LayerOrder defines the canonical execution order for layers.
// JSON output must always follow this order.
var LayerOrder = []string{"dns", "tcp", "tls", "http"}

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

// marshalNoEscape marshals v without HTML escaping.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encoder.Encode appends a newline; trim it.
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}

// MarshalJSON implements json.Marshaler to guarantee layers appear in execution
// order (dns→tcp→tls→http) rather than Go's default alphabetical map key order.
func (r *Result) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"schema_version":`)
	b, err := marshalNoEscape(r.SchemaVersion)
	if err != nil {
		return nil, err
	}
	buf.Write(b)

	buf.WriteString(`,"started_at":`)
	b, err = marshalNoEscape(r.StartedAt)
	if err != nil {
		return nil, err
	}
	buf.Write(b)

	buf.WriteString(`,"target":`)
	b, err = marshalNoEscape(r.Target)
	if err != nil {
		return nil, err
	}
	buf.Write(b)

	buf.WriteString(`,"layers":{`)
	first := true
	for _, name := range LayerOrder {
		lr, ok := r.Layers[name]
		if !ok {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		b, err = marshalNoEscape(name)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
		buf.WriteByte(':')
		b, err = marshalNoEscape(lr)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte('}')

	buf.WriteString(`,"summary":`)
	b, err = marshalNoEscape(r.Summary)
	if err != nil {
		return nil, err
	}
	buf.Write(b)

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ProbeContext carries shared state through the layer execution pipeline.
type ProbeContext struct {
	Context     context.Context
	Target      Target
	ResolvedIPs []string
	Insecure    bool
	Redact      bool
	Method      string
	Headers     map[string]string
}

// Layer is the interface that each network layer must implement.
type Layer interface {
	Name() string
	Probe(ctx *ProbeContext) *LayerResult
}
