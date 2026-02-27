package exitcode_test

import (
	"testing"

	"github.com/muras3/stackdiag/internal/core"
	"github.com/muras3/stackdiag/internal/exitcode"
)

func TestFromResult(t *testing.T) {
	tests := []struct {
		name   string
		result *core.Result
		want   int
	}{
		{
			name: "all ok returns 0",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusOK},
					"tcp":  {Status: core.StatusOK},
					"tls":  {Status: core.StatusOK},
					"http": {Status: core.StatusOK},
				},
			},
			want: 0,
		},
		{
			name: "all skip returns 0",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusOK},
					"tcp":  {Status: core.StatusOK},
					"tls":  {Status: core.StatusSkip},
					"http": {Status: core.StatusSkip},
				},
			},
			want: 0,
		},
		{
			name: "dns fail returns 10",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusFail},
					"tcp":  {Status: core.StatusSkip},
					"tls":  {Status: core.StatusSkip},
					"http": {Status: core.StatusSkip},
				},
			},
			want: 10,
		},
		{
			name: "tcp fail returns 20",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusOK},
					"tcp":  {Status: core.StatusFail},
					"tls":  {Status: core.StatusSkip},
					"http": {Status: core.StatusSkip},
				},
			},
			want: 20,
		},
		{
			name: "tls fail returns 30",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusOK},
					"tcp":  {Status: core.StatusOK},
					"tls":  {Status: core.StatusFail},
					"http": {Status: core.StatusSkip},
				},
			},
			want: 30,
		},
		{
			name: "http fail returns 40",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusOK},
					"tcp":  {Status: core.StatusOK},
					"tls":  {Status: core.StatusOK},
					"http": {Status: core.StatusFail},
				},
			},
			want: 40,
		},
		{
			name: "warn with no fail returns 2",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusOK},
					"tcp":  {Status: core.StatusOK},
					"tls":  {Status: core.StatusWarn},
					"http": {Status: core.StatusOK},
				},
			},
			want: 2,
		},
		{
			name: "tool error INVALID_TARGET returns 1",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns": {
						Status: core.StatusFail,
						Error:  &core.ProbeError{Code: "INVALID_TARGET", Message: "bad target"},
					},
				},
			},
			want: 1,
		},
		{
			name: "tool error INVALID_ARGS returns 1",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns": {
						Status: core.StatusFail,
						Error:  &core.ProbeError{Code: "INVALID_ARGS", Message: "bad args"},
					},
				},
			},
			want: 1,
		},
		{
			name: "warn and fail returns fail exit code",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{
					"dns":  {Status: core.StatusOK},
					"tcp":  {Status: core.StatusOK},
					"tls":  {Status: core.StatusWarn},
					"http": {Status: core.StatusFail},
				},
			},
			want: 40,
		},
		{
			name: "nil layers returns 0",
			result: &core.Result{
				Layers: nil,
			},
			want: 0,
		},
		{
			name: "empty layers returns 0",
			result: &core.Result{
				Layers: map[string]*core.LayerResult{},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exitcode.FromResult(tt.result)
			if got != tt.want {
				t.Errorf("FromResult() = %d, want %d", got, tt.want)
			}
		})
	}
}
