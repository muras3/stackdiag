package table

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/muras3/stackdiag/internal/core"
)

// ANSI escape codes for colored output.
const (
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
	colorReset  = "\033[0m"
)

// layerOrder defines the fixed display order for layers.
var layerOrder = []string{"dns", "tcp", "tls", "http"}

// Render writes the Result as a human-readable table to w.
// When useColor is true, output uses ANSI escape codes and Unicode symbols.
// When useColor is false, output uses ASCII-only fallback symbols.
func Render(w io.Writer, r *core.Result, useColor bool) error {
	// Compute the max width of the duration column for right-alignment.
	// Duration is formatted as integer milliseconds, e.g. "9ms", "57ms".
	maxDurLen := 0
	for _, name := range layerOrder {
		lr := r.Layers[name]
		if lr == nil {
			continue
		}
		dur := formatDuration(lr.DurationMS)
		if len(dur) > maxDurLen {
			maxDurLen = len(dur)
		}
	}

	// Render each layer line.
	for _, name := range layerOrder {
		lr := r.Layers[name]
		if lr == nil {
			continue
		}

		sym := statusSymbol(lr.Status, useColor)
		dur := formatDuration(lr.DurationMS)
		desc := layerDescription(name, lr, useColor)

		// Right-align the duration field.
		paddedDur := fmt.Sprintf("%*s", maxDurLen, dur)

		line := fmt.Sprintf("  %-4s  %s  %s  %s", name, sym, paddedDur, desc)
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}

	// Blank line before summary.
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	// Summary line.
	summary := formatSummary(r.Summary)
	if _, err := fmt.Fprintf(w, "  %s\n", summary); err != nil {
		return err
	}

	return nil
}

// statusSymbol returns the display symbol for a given status.
func statusSymbol(s core.Status, useColor bool) string {
	if useColor {
		switch s {
		case core.StatusOK:
			return colorGreen + "\u2713" + colorReset
		case core.StatusWarn:
			return colorYellow + "\u26a0" + colorReset
		case core.StatusFail:
			return colorRed + "\u2717" + colorReset
		case core.StatusSkip:
			return colorDim + "-" + colorReset
		default:
			return "?"
		}
	}

	// ASCII fallback — all padded to 6 chars for alignment.
	switch s {
	case core.StatusOK:
		return "[ok]  "
	case core.StatusWarn:
		return "[!!]  "
	case core.StatusFail:
		return "[FAIL]"
	case core.StatusSkip:
		return "[skip]"
	default:
		return "[??]  "
	}
}

// formatDuration formats a duration in milliseconds as an integer string with "ms" suffix.
func formatDuration(ms float64) string {
	return fmt.Sprintf("%dms", int(math.Round(ms)))
}

// layerDescription builds the description string for a layer line.
func layerDescription(name string, lr *core.LayerResult, useColor bool) string {
	// If the layer failed or warned and has an error, use error message for fail.
	// For warn, we still show observations but fall through to error if no observations.
	if lr.Status == core.StatusFail && lr.Error != nil {
		return lr.Error.Message
	}

	if lr.Status == core.StatusSkip {
		return ""
	}

	obs := lr.Observations

	switch name {
	case "dns":
		return dnsDescription(obs, lr, useColor)
	case "tcp":
		return tcpDescription(obs, lr)
	case "tls":
		return tlsDescription(obs, lr)
	case "http":
		return httpDescription(obs, lr)
	default:
		return ""
	}
}

// dnsDescription builds: "{query_name} → {first_answer}" or error message.
func dnsDescription(obs map[string]any, lr *core.LayerResult, useColor bool) string {
	queryName, _ := obs["query_name"].(string)
	var firstAnswer string

	if answers, ok := obs["answers"]; ok {
		switch a := answers.(type) {
		case []any:
			if len(a) > 0 {
				firstAnswer = fmt.Sprintf("%v", a[0])
			}
		case []string:
			if len(a) > 0 {
				firstAnswer = a[0]
			}
		}
	}

	arrow := " \u2192 "
	if !useColor {
		arrow = " -> "
	}

	if queryName != "" && firstAnswer != "" {
		return queryName + arrow + firstAnswer
	}
	if queryName != "" {
		return queryName
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
}

// tcpDescription builds: ":{port}" or error message.
func tcpDescription(obs map[string]any, lr *core.LayerResult) string {
	if port, ok := obs["remote_port"]; ok {
		return fmt.Sprintf(":%v", toInt(port))
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
}

// tlsDescription builds: "{version}, cert expires in {days}d" or error message.
func tlsDescription(obs map[string]any, lr *core.LayerResult) string {
	version, _ := obs["version"].(string)
	days := getCertDays(obs)

	var parts []string
	if version != "" {
		parts = append(parts, version)
	}
	if days >= 0 {
		parts = append(parts, fmt.Sprintf("cert expires in %dd", days))
	}

	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
}

// httpDescription builds: "{status_code} {status_text}" or error message.
func httpDescription(obs map[string]any, lr *core.LayerResult) string {
	code, hasCode := obs["status_code"]
	text, _ := obs["status_text"].(string)

	if hasCode {
		codeInt := toInt(code)
		if text != "" {
			return fmt.Sprintf("%d %s", codeInt, text)
		}
		return fmt.Sprintf("%d", codeInt)
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
}

// getCertDays extracts cert_days_until_expiry from observations.
// Returns -1 if not found.
func getCertDays(obs map[string]any) int {
	v, ok := obs["cert_days_until_expiry"]
	if !ok {
		return -1
	}
	return toInt(v)
}

// toInt converts a numeric value to int, handling int, float64, and other numeric types.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(math.Round(n))
	case float32:
		return int(math.Round(float64(n)))
	default:
		return 0
	}
}

// formatSummary builds the summary line.
func formatSummary(s *core.Summary) string {
	wallClock := fmt.Sprintf("%dms total", int(math.Round(s.WallClockMS)))

	if s.FirstNonOKLayer == "" {
		return wallClock + " | all ok"
	}

	return fmt.Sprintf("%s | first issue: %s | exit %d", wallClock, s.FirstNonOKLayer, s.ExitCode)
}
