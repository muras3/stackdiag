package runner

import (
	"context"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

// fakeLayer returns a fixed LayerResult.
type fakeLayer struct {
	name   string
	result *core.LayerResult
	delay  time.Duration
}

func (f *fakeLayer) Name() string { return f.name }

func (f *fakeLayer) Probe(_ *core.ProbeContext) *core.LayerResult {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.result
}

func okResult() *core.LayerResult {
	return &core.LayerResult{
		Status:       core.StatusOK,
		DurationMS:   5,
		Observations: map[string]any{},
		Error:        nil,
	}
}

func warnResult() *core.LayerResult {
	return &core.LayerResult{
		Status:       core.StatusWarn,
		DurationMS:   10,
		Observations: map[string]any{},
		Error:        &core.ProbeError{Code: "TLS_CERT_EXPIRING_SOON", Message: "expires soon"},
	}
}

func failResult(code string) *core.LayerResult {
	return &core.LayerResult{
		Status:       core.StatusFail,
		DurationMS:   3,
		Observations: map[string]any{},
		Error:        &core.ProbeError{Code: code, Message: "failed"},
	}
}

func TestRunAllSuccess(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayer{name: "tcp", result: okResult()},
		&fakeLayer{name: "tls", result: okResult()},
		&fakeLayer{name: "http", result: okResult()},
	}

	r := New(layers)
	pctx := &core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
	}

	result := r.Run(pctx)

	if result.Summary.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.Summary.ExitCode)
	}
	if result.Summary.FirstNonOKLayer != "" {
		t.Errorf("FirstNonOKLayer = %q, want empty", result.Summary.FirstNonOKLayer)
	}
	for _, name := range []string{"dns", "tcp", "tls", "http"} {
		lr, ok := result.Layers[name]
		if !ok {
			t.Errorf("missing layer %q", name)
			continue
		}
		if lr.Status != core.StatusOK {
			t.Errorf("layer %q status = %q, want ok", name, lr.Status)
		}
	}
	// Reachability layer should be present as skip (not in runner's layer list).
	if result.Layers["reachability"] == nil {
		t.Error("missing reachability layer")
	}
}

func TestRunDNSFail(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: failResult("DNS_NXDOMAIN")},
		&fakeLayer{name: "tcp", result: okResult()},
		&fakeLayer{name: "tls", result: okResult()},
		&fakeLayer{name: "http", result: okResult()},
	}

	r := New(layers)
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
	})

	if result.Layers["dns"].Status != core.StatusFail {
		t.Errorf("dns status = %q, want fail", result.Layers["dns"].Status)
	}
	if result.Summary.FirstNonOKLayer != "dns" {
		t.Errorf("FirstNonOKLayer = %q, want dns", result.Summary.FirstNonOKLayer)
	}
	// Remaining layers should be skip.
	for _, name := range []string{"tcp", "tls", "http"} {
		if result.Layers[name].Status != core.StatusSkip {
			t.Errorf("layer %q status = %q, want skip", name, result.Layers[name].Status)
		}
	}
}

func TestRunTCPFail(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayer{name: "tcp", result: failResult("TCP_REFUSED")},
		&fakeLayer{name: "tls", result: okResult()},
		&fakeLayer{name: "http", result: okResult()},
	}

	r := New(layers)
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
	})

	if result.Layers["dns"].Status != core.StatusOK {
		t.Errorf("dns status = %q, want ok", result.Layers["dns"].Status)
	}
	if result.Layers["tcp"].Status != core.StatusFail {
		t.Errorf("tcp status = %q, want fail", result.Layers["tcp"].Status)
	}
	if result.Summary.FirstNonOKLayer != "tcp" {
		t.Errorf("FirstNonOKLayer = %q, want tcp", result.Summary.FirstNonOKLayer)
	}
	for _, name := range []string{"tls", "http"} {
		if result.Layers[name].Status != core.StatusSkip {
			t.Errorf("layer %q status = %q, want skip", name, result.Layers[name].Status)
		}
	}
}

func TestRunTLSWarnHTTPFail(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayer{name: "tcp", result: okResult()},
		&fakeLayer{name: "tls", result: warnResult()},
		&fakeLayer{name: "http", result: failResult("HTTP_503")},
	}

	r := New(layers)
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
	})

	if result.Layers["tls"].Status != core.StatusWarn {
		t.Errorf("tls status = %q, want warn", result.Layers["tls"].Status)
	}
	if result.Layers["http"].Status != core.StatusFail {
		t.Errorf("http status = %q, want fail", result.Layers["http"].Status)
	}
	// First non-ok is tls (warn).
	if result.Summary.FirstNonOKLayer != "tls" {
		t.Errorf("FirstNonOKLayer = %q, want tls", result.Summary.FirstNonOKLayer)
	}
}

func TestRunTCPOnly(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayer{name: "tcp", result: okResult()},
	}

	r := New(layers)
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "tcp://example.com:8080", Scheme: "tcp", Host: "example.com", Port: 8080},
	})

	if len(result.Layers) != 5 {
		t.Errorf("layers count = %d, want 5", len(result.Layers))
	}
	if result.Layers["tls"].Status != core.StatusSkip {
		t.Errorf("tls status = %q, want skip", result.Layers["tls"].Status)
	}
	if result.Layers["http"].Status != core.StatusSkip {
		t.Errorf("http status = %q, want skip", result.Layers["http"].Status)
	}
	if result.Summary.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.Summary.ExitCode)
	}
}

func TestRunDNSTCPIncludesSkippedTLSHTTP(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayer{name: "tcp", result: okResult()},
	}

	r := New(layers)
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "tcp://example.com:8080", Scheme: "tcp", Host: "example.com", Port: 8080},
	})

	for _, name := range []string{"dns", "reachability", "tcp", "tls", "http"} {
		if _, ok := result.Layers[name]; !ok {
			t.Fatalf("missing layer %q", name)
		}
	}
	if result.Layers["reachability"].Status != core.StatusSkip {
		t.Errorf("reachability status = %q, want skip", result.Layers["reachability"].Status)
	}
	if result.Layers["tls"].Status != core.StatusSkip {
		t.Errorf("tls status = %q, want skip", result.Layers["tls"].Status)
	}
	if result.Layers["http"].Status != core.StatusSkip {
		t.Errorf("http status = %q, want skip", result.Layers["http"].Status)
	}
}

func TestRunTimeoutBudgetExhausted(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult(), delay: 200 * time.Millisecond},
		&fakeLayer{name: "tcp", result: okResult()},
		&fakeLayer{name: "tls", result: okResult()},
		&fakeLayer{name: "http", result: okResult()},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	r := New(layers)
	result := r.Run(&core.ProbeContext{
		Context: ctx,
		Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
	})

	// DNS should run (and may ok or fail depending on timing).
	// At least some subsequent layers should be skip due to budget exhaustion.
	skipCount := 0
	for _, lr := range result.Layers {
		if lr.Status == core.StatusSkip {
			skipCount++
		}
	}
	if skipCount == 0 {
		t.Error("expected at least one skipped layer due to timeout budget")
	}
}

func TestRunSchemaVersion(t *testing.T) {
	r := New([]core.Layer{&fakeLayer{name: "dns", result: okResult()}})
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "tcp://example.com:80", Scheme: "tcp", Host: "example.com", Port: 80},
	})

	if result.SchemaVersion != "v0.2" {
		t.Errorf("SchemaVersion = %q, want v0.2", result.SchemaVersion)
	}
	if result.Target != "tcp://example.com:80" {
		t.Errorf("Target = %q", result.Target)
	}
}

func TestRunHTTPScheme(t *testing.T) {
	// http:// target should run dns+tcp+http, tls=skip.
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayer{name: "tcp", result: okResult()},
		&fakeLayer{name: "http", result: okResult()},
	}

	r := New(layers)
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "http://example.com", Scheme: "http", Host: "example.com", Port: 80, Path: "/"},
	})

	if result.Layers["dns"].Status != core.StatusOK {
		t.Errorf("dns status = %q, want ok", result.Layers["dns"].Status)
	}
	if result.Layers["tcp"].Status != core.StatusOK {
		t.Errorf("tcp status = %q, want ok", result.Layers["tcp"].Status)
	}
	if result.Layers["http"].Status != core.StatusOK {
		t.Errorf("http status = %q, want ok", result.Layers["http"].Status)
	}
	if result.Layers["tls"].Status != core.StatusSkip {
		t.Errorf("tls status = %q, want skip", result.Layers["tls"].Status)
	}
	if result.Summary.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.Summary.ExitCode)
	}
}

func TestRunEmptyLayers(t *testing.T) {
	r := New([]core.Layer{})
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
	})

	// All layers should be skip.
	for _, name := range []string{"dns", "reachability", "tcp", "tls", "http"} {
		lr, ok := result.Layers[name]
		if !ok {
			t.Errorf("missing layer %q", name)
			continue
		}
		if lr.Status != core.StatusSkip {
			t.Errorf("layer %q status = %q, want skip", name, lr.Status)
		}
	}
	if result.Summary.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.Summary.ExitCode)
	}
}

func TestRunCountBasic(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayer{name: "tcp", result: okResult()},
		&fakeLayer{name: "tls", result: okResult()},
		&fakeLayer{name: "http", result: okResult()},
	}

	r := New(layers)
	cr := r.RunCount(3, func(attempt int) (*core.ProbeContext, context.CancelFunc) {
		return &core.ProbeContext{
			Context: context.Background(),
			Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
		}, func() {}
	})

	if len(cr.Attempts) != 3 {
		t.Fatalf("Attempts = %d, want 3", len(cr.Attempts))
	}
	if cr.Count != 3 {
		t.Errorf("Count = %d, want 3", cr.Count)
	}
	if cr.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", cr.ExitCode)
	}
	for i, a := range cr.Attempts {
		if a.Attempt != i+1 {
			t.Errorf("Attempt[%d].Attempt = %d, want %d", i, a.Attempt, i+1)
		}
	}
	// Executed layers should have statistics with 0 loss.
	for _, name := range []string{"dns", "tcp", "tls", "http"} {
		ls, ok := cr.Statistics[name]
		if !ok {
			t.Errorf("missing statistics for %q", name)
			continue
		}
		if ls.SuccessCount != 3 {
			t.Errorf("%s SuccessCount = %d, want 3", name, ls.SuccessCount)
		}
		if ls.LossRatio != 0.0 {
			t.Errorf("%s LossRatio = %f, want 0", name, ls.LossRatio)
		}
		if ls.P50MS == nil {
			t.Errorf("%s P50MS should not be nil", name)
		}
	}
	// Reachability layer should have statistics (skip counts since not in runner).
	if _, ok := cr.Statistics["reachability"]; !ok {
		t.Error("missing statistics for reachability")
	}
}

func TestRunCountWithFailure(t *testing.T) {
	callCount := 0
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult()},
		&fakeLayerFunc{name: "tcp", fn: func() *core.LayerResult {
			callCount++
			if callCount == 2 {
				return failResult("TCP_REFUSED")
			}
			return okResult()
		}},
		&fakeLayer{name: "tls", result: okResult()},
		&fakeLayer{name: "http", result: okResult()},
	}

	r := New(layers)
	cr := r.RunCount(3, func(attempt int) (*core.ProbeContext, context.CancelFunc) {
		return &core.ProbeContext{
			Context: context.Background(),
			Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
		}, func() {}
	})

	if cr.ExitCode != 20 {
		t.Errorf("ExitCode = %d, want 20 (worst)", cr.ExitCode)
	}
	tcpStats := cr.Statistics["tcp"]
	if tcpStats.FailCount != 1 {
		t.Errorf("tcp FailCount = %d, want 1", tcpStats.FailCount)
	}
	if tcpStats.SuccessCount != 2 {
		t.Errorf("tcp SuccessCount = %d, want 2", tcpStats.SuccessCount)
	}
}

func TestRunCountContinuesAfterFail(t *testing.T) {
	attemptsSeen := 0
	layers := []core.Layer{
		&fakeLayerFunc{name: "dns", fn: func() *core.LayerResult {
			attemptsSeen++
			if attemptsSeen == 1 {
				return failResult("DNS_NXDOMAIN")
			}
			return okResult()
		}},
		&fakeLayer{name: "tcp", result: okResult()},
	}

	r := New(layers)
	cr := r.RunCount(2, func(attempt int) (*core.ProbeContext, context.CancelFunc) {
		return &core.ProbeContext{
			Context: context.Background(),
			Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
		}, func() {}
	})

	if len(cr.Attempts) != 2 {
		t.Fatalf("Attempts = %d, want 2", len(cr.Attempts))
	}
	if cr.Attempts[0].Layers["dns"].Status != core.StatusFail {
		t.Errorf("attempt 1 dns should be fail")
	}
	if cr.Attempts[1].Layers["dns"].Status != core.StatusOK {
		t.Errorf("attempt 2 dns should be ok")
	}
}

func TestRunCountStatisticsP50P95(t *testing.T) {
	// 5 attempts with known durations: 10, 20, 30, 40, 50
	callIdx := 0
	durations := []float64{10, 20, 30, 40, 50}
	layers := []core.Layer{
		&fakeLayerFunc{name: "dns", fn: func() *core.LayerResult {
			d := durations[callIdx]
			callIdx++
			return &core.LayerResult{
				Status:       core.StatusOK,
				DurationMS:   d,
				Observations: map[string]any{},
			}
		}},
	}

	r := New(layers)
	cr := r.RunCount(5, func(attempt int) (*core.ProbeContext, context.CancelFunc) {
		return &core.ProbeContext{
			Context: context.Background(),
			Target:  core.Target{Original: "tcp://example.com:80", Scheme: "tcp", Host: "example.com", Port: 80},
		}, func() {}
	})

	dns := cr.Statistics["dns"]
	if dns.P50MS == nil {
		t.Fatal("P50MS should not be nil")
	}
	// p50 of [10,20,30,40,50] = 30 (median, odd count)
	if *dns.P50MS != 30 {
		t.Errorf("P50MS = %f, want 30", *dns.P50MS)
	}
	// p95 of 5 values: index = ceil(0.95*5)-1 = 5-1 = 4 → value 50
	if dns.P95MS == nil {
		t.Fatal("P95MS should not be nil")
	}
	if *dns.P95MS != 50 {
		t.Errorf("P95MS = %f, want 50", *dns.P95MS)
	}
}

func TestRunCountNoSuccessOmitsPercentiles(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: failResult("DNS_NXDOMAIN")},
		&fakeLayer{name: "tcp", result: okResult()},
	}

	r := New(layers)
	cr := r.RunCount(3, func(attempt int) (*core.ProbeContext, context.CancelFunc) {
		return &core.ProbeContext{
			Context: context.Background(),
			Target:  core.Target{Original: "https://example.com", Scheme: "https", Host: "example.com", Port: 443, Path: "/"},
		}, func() {}
	})

	dns := cr.Statistics["dns"]
	if dns.P50MS != nil {
		t.Errorf("dns P50MS should be nil when all fail, got %f", *dns.P50MS)
	}
	if dns.P95MS != nil {
		t.Errorf("dns P95MS should be nil when all fail, got %f", *dns.P95MS)
	}
	if dns.FailCount != 3 {
		t.Errorf("dns FailCount = %d, want 3", dns.FailCount)
	}
}

// fakeLayerFunc allows per-call dynamic results.
type fakeLayerFunc struct {
	name string
	fn   func() *core.LayerResult
}

func (f *fakeLayerFunc) Name() string                                 { return f.name }
func (f *fakeLayerFunc) Probe(_ *core.ProbeContext) *core.LayerResult { return f.fn() }

func TestRunStartedAtAndWallClock(t *testing.T) {
	layers := []core.Layer{
		&fakeLayer{name: "dns", result: okResult(), delay: 10 * time.Millisecond},
	}

	r := New(layers)
	before := time.Now()
	result := r.Run(&core.ProbeContext{
		Context: context.Background(),
		Target:  core.Target{Original: "tcp://example.com:80", Scheme: "tcp", Host: "example.com", Port: 80},
	})
	after := time.Now()

	if result.StartedAt.Before(before) || result.StartedAt.After(after) {
		t.Errorf("StartedAt = %v, expected between %v and %v", result.StartedAt, before, after)
	}
	if result.Summary.WallClockMS <= 0 {
		t.Errorf("WallClockMS = %v, want > 0", result.Summary.WallClockMS)
	}
}
