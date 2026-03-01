package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SchemaVersion is the current schema version for stackdiag output.
const SchemaVersion = "v0.1"

// LayerOrder defines the canonical execution order for layers.
// JSON output must always follow this order.
var LayerOrder = []string{"dns", "reachability", "tcp", "tls", "http"}

// Status represents the outcome of a layer check.
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
// ok, warn, and skip all allow continuation; only fail stops the pipeline.
func (s Status) ShouldContinue() bool { return s != StatusFail }

// ProbeError is a structured error with a machine-readable code.
type ProbeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ProbeError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// LayerResult holds the outcome of a single layer check.
type LayerResult struct {
	Status       Status      `json:"status"`
	DurationMS   float64     `json:"duration_ms"`
	Observations any         `json:"observations"`
	Error        *ProbeError `json:"error"`
}

// Summary holds aggregate information about the stackdiag run.
type Summary struct {
	WallClockMS     float64 `json:"wall_clock_ms"`
	FirstNonOKLayer string  `json:"first_non_ok_layer"`
	ExitCode        int     `json:"exit_code"`
}

// Result is the top-level stackdiag output. Internal data model = JSON output.
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
	buf.Grow(1536)
	buf.WriteByte('{')
	if err := writeField(&buf, true, "schema_version", r.SchemaVersion); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "started_at", r.StartedAt); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "target", r.Target); err != nil {
		return nil, err
	}
	if err := writeOrderedMap(&buf, false, "layers", layerGetter(r.Layers)); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "summary", r.Summary); err != nil {
		return nil, err
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// AttemptResult holds the outcome of a single attempt in a --count N run.
type AttemptResult struct {
	Attempt   int                     `json:"attempt"`
	StartedAt time.Time               `json:"started_at"`
	Layers    map[string]*LayerResult `json:"layers"`
	Summary   *Summary                `json:"summary"`
}

// MarshalJSON implements json.Marshaler to guarantee layers appear in execution
// order (dns→tcp→tls→http).
func (a *AttemptResult) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(1536)
	buf.WriteByte('{')
	if err := writeField(&buf, true, "attempt", a.Attempt); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "started_at", a.StartedAt); err != nil {
		return nil, err
	}
	if err := writeOrderedMap(&buf, false, "layers", layerGetter(a.Layers)); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "summary", a.Summary); err != nil {
		return nil, err
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// LayerStatistics holds aggregate statistics for a single layer across multiple attempts.
type LayerStatistics struct {
	P50MS        *float64 `json:"p50_ms"`
	P95MS        *float64 `json:"p95_ms"`
	SuccessCount int      `json:"success_count"`
	FailCount    int      `json:"fail_count"`
	SkipCount    int      `json:"skip_count"`
	SampleCount  int      `json:"sample_count"`
	LossRatio    float64  `json:"loss_ratio"`
}

// CountResult is the top-level output for --count N runs.
type CountResult struct {
	SchemaVersion string                      `json:"schema_version"`
	Target        string                      `json:"target"`
	Count         int                         `json:"count"`
	ExitCode      int                         `json:"exit_code"`
	Attempts      []*AttemptResult            `json:"attempts"`
	Statistics    map[string]*LayerStatistics `json:"statistics"`
}

// MarshalJSON implements json.Marshaler to guarantee statistics keys appear in
// execution order (dns→tcp→tls→http).
func (c *CountResult) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(1536)
	buf.WriteByte('{')
	if err := writeField(&buf, true, "schema_version", c.SchemaVersion); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "target", c.Target); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "count", c.Count); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "exit_code", c.ExitCode); err != nil {
		return nil, err
	}
	if err := writeField(&buf, false, "attempts", c.Attempts); err != nil {
		return nil, err
	}
	if err := writeOrderedMap(&buf, false, "statistics", func(name string) any {
		if ls, ok := c.Statistics[name]; ok {
			return ls
		}
		return nil
	}); err != nil {
		return nil, err
	}
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
	TLSScan     bool   // --tls-scan: probe TLS version support
	DNSServer   string // --dns-server: custom DNS resolver address
}

// Layer is the interface that each network layer must implement.
type Layer interface {
	Name() string
	Probe(ctx *ProbeContext) *LayerResult
}
