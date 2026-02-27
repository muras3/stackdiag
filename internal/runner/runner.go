package runner

import (
	"time"

	"github.com/muras3/probe/internal/core"
	"github.com/muras3/probe/internal/exitcode"
)

// Runner orchestrates sequential layer execution.
type Runner struct {
	layers []core.Layer
}

// New creates a Runner with the given layers (executed in order).
func New(layers []core.Layer) *Runner {
	return &Runner{layers: layers}
}

// Run executes layers sequentially, stopping on fail, continuing on warn.
func (r *Runner) Run(pctx *core.ProbeContext) *core.Result {
	start := time.Now()
	allLayerNames := core.LayerOrder
	result := &core.Result{
		SchemaVersion: "v0.1",
		StartedAt:     start.UTC(),
		Target:        pctx.Target.Original,
		Layers:        make(map[string]*core.LayerResult),
		Summary:       &core.Summary{},
	}

	stopped := false
	firstNonOK := ""

	for _, layer := range r.layers {
		name := layer.Name()

		// If a previous layer failed or context is done, skip.
		if stopped || pctx.Context.Err() != nil {
			result.Layers[name] = &core.LayerResult{
				Status:       core.StatusSkip,
				DurationMS:   0,
				Observations: map[string]any{},
				Error:        nil,
			}
			continue
		}

		lr := layer.Probe(pctx)
		result.Layers[name] = lr

		// Track first non-ok layer.
		if firstNonOK == "" && !lr.Status.IsOK() {
			firstNonOK = name
		}

		// Stop on fail (but continue on warn).
		if !lr.Status.ShouldContinue() {
			stopped = true
		}
	}

	// Ensure schema contract: always include all known layer names.
	for _, name := range allLayerNames {
		if _, ok := result.Layers[name]; ok {
			continue
		}
		result.Layers[name] = &core.LayerResult{
			Status:       core.StatusSkip,
			DurationMS:   0,
			Observations: map[string]any{},
			Error:        nil,
		}
	}

	wallClock := float64(time.Since(start).Microseconds()) / 1000.0
	result.Summary.WallClockMS = wallClock
	result.Summary.FirstNonOKLayer = firstNonOK
	result.Summary.ExitCode = exitcode.FromResult(result)

	return result
}
