package runner

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/muras3/stackdiag/internal/core"
	"github.com/muras3/stackdiag/internal/exitcode"
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
// This is a convenience wrapper around RunOnce.
func (r *Runner) Run(pctx *core.ProbeContext) *core.Result {
	return r.RunOnce(pctx)
}

// RunOnce executes layers sequentially, stopping on fail, continuing on warn.
func (r *Runner) RunOnce(pctx *core.ProbeContext) *core.Result {
	start := time.Now()
	allLayerNames := core.LayerOrder
	result := &core.Result{
		SchemaVersion: core.SchemaVersion,
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

// RunCount executes the layer pipeline n times, building a CountResult with
// per-layer statistics (p50, p95, loss ratio).
// The factory must return a ProbeContext and its CancelFunc. RunCount calls
// cancel after each attempt to prevent context/timer goroutine leaks.
func (r *Runner) RunCount(n int, factory func(attempt int) (*core.ProbeContext, context.CancelFunc)) *core.CountResult {
	cr := &core.CountResult{
		SchemaVersion: core.SchemaVersion,
		Count:         n,
		Attempts:      make([]*core.AttemptResult, 0, n),
		Statistics:    make(map[string]*core.LayerStatistics),
	}

	exitCodes := make([]int, 0, n)
	// Collect durations per layer for statistics.
	// Key: layer name, value: list of durations for ok/warn attempts.
	durations := make(map[string][]float64)
	counts := make(map[string]*core.LayerStatistics)

	// Initialize stats for all known layers.
	for _, name := range core.LayerOrder {
		counts[name] = &core.LayerStatistics{}
	}

	for i := 1; i <= n; i++ {
		pctx, cancel := factory(i)
		if cr.Target == "" {
			cr.Target = pctx.Target.Original
		}

		result := r.RunOnce(pctx)
		cancel() // Clean up context to prevent timer goroutine leaks.
		exitCodes = append(exitCodes, result.Summary.ExitCode)

		attempt := &core.AttemptResult{
			Attempt:   i,
			StartedAt: result.StartedAt,
			Layers:    result.Layers,
			Summary:   result.Summary,
		}
		cr.Attempts = append(cr.Attempts, attempt)

		// Accumulate statistics per layer.
		for _, name := range core.LayerOrder {
			lr, ok := result.Layers[name]
			if !ok {
				continue
			}
			stats := counts[name]
			switch lr.Status {
			case core.StatusOK, core.StatusWarn:
				stats.SuccessCount++
				durations[name] = append(durations[name], lr.DurationMS)
			case core.StatusFail:
				stats.FailCount++
			case core.StatusSkip:
				stats.SkipCount++
			}
		}
	}

	cr.ExitCode = exitcode.WorstExitCode(exitCodes)

	// Calculate final statistics.
	for _, name := range core.LayerOrder {
		stats := counts[name]
		stats.SampleCount = stats.SuccessCount + stats.FailCount
		if stats.SampleCount > 0 {
			stats.LossRatio = float64(stats.FailCount) / float64(stats.SampleCount)
		}
		if d := durations[name]; len(d) > 0 {
			stats.P50MS = percentile(d, 0.50)
			stats.P95MS = percentile(d, 0.95)
		}
		cr.Statistics[name] = stats
	}

	return cr
}

// percentile calculates the given percentile from a slice of values.
// Returns nil for an empty slice.
func percentile(values []float64, p float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if p == 0.50 {
		// Median
		if n%2 == 1 {
			v := sorted[n/2]
			return &v
		}
		v := (sorted[n/2-1] + sorted[n/2]) / 2
		return &v
	}
	// Nearest rank method
	idx := int(math.Ceil(p*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	v := sorted[idx]
	return &v
}
