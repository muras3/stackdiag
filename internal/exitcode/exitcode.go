// Package exitcode maps a stackdiag Result to a process exit code.
//
// Exit code semantics (from product-design.md):
//
//	0  = all ok
//	1  = tool error (INVALID_TARGET, INVALID_ARGS)
//	2  = warn (any layer has warn status, none failed)
//	10 = DNS failure
//	15 = Reachability failure
//	20 = TCP failure
//	30 = TLS failure
//	40 = HTTP failure
package exitcode

import "github.com/muras3/stackdiag/internal/core"

// layerOrder defines the evaluation order for layers.
// The first failing layer in this order determines the exit code.
var layerOrder = []string{"dns", "reachability", "tcp", "tls", "http"}

// layerCodes maps layer names to their exit codes.
var layerCodes = map[string]int{
	"dns":          10,
	"reachability": 15,
	"tcp":          20,
	"tls":          30,
	"http":         40,
}

// toolErrorCodes are error codes that indicate a tool-level error (exit 1).
var toolErrorCodes = map[string]bool{
	"INVALID_TARGET": true,
	"INVALID_ARGS":   true,
}

// FromResult maps a *core.Result to a process exit code.
//
// It inspects r.Layers to find the first non-ok layer and returns:
//   - 1 if any layer has a tool error (INVALID_TARGET, INVALID_ARGS)
//   - the layer-specific code (10/20/30/40) for the first failing layer
//   - 2 if any layer has warn status but none failed
//   - 0 if all layers are ok or skip
func FromResult(r *core.Result) int {
	if len(r.Layers) == 0 {
		return 0
	}

	hasWarn := false

	for _, name := range layerOrder {
		lr, ok := r.Layers[name]
		if !ok {
			continue
		}

		if lr.Status == core.StatusFail {
			// Check for tool-level errors first.
			if lr.Error != nil && toolErrorCodes[lr.Error.Code] {
				return 1
			}
			return layerCodes[name]
		}

		if lr.Status == core.StatusWarn {
			hasWarn = true
		}
	}

	if hasWarn {
		return 2
	}

	return 0
}

// WorstExitCode returns the maximum exit code from a slice.
// Returns 0 for an empty slice.
func WorstExitCode(codes []int) int {
	worst := 0
	for _, c := range codes {
		if c > worst {
			worst = c
		}
	}
	return worst
}
