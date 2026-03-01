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
var layerOrder = []string{"dns", "reachability", "tcp", "tls", "http"}

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

		if err := renderLayerLine(w, name, lr, maxDurLen, useColor); err != nil {
			return err
		}

		// Render TLS scan sub-line if present.
		if name == "tls" {
			if err := renderTLSScanLine(w, lr, useColor); err != nil {
				return err
			}
		}

		// Render HTTP response_headers sub-lines if present.
		if name == "http" {
			if err := renderResponseHeadersLines(w, lr); err != nil {
				return err
			}
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

// obsMap extracts a map[string]any from the Observations field.
// Returns nil if Observations is not a map.
func obsMap(lr *core.LayerResult) map[string]any {
	if m, ok := lr.Observations.(map[string]any); ok {
		return m
	}
	return nil
}

// layerDescription builds the description string for a layer line.
func layerDescription(name string, lr *core.LayerResult, useColor bool) string {
	// If the layer failed or warned and has an error, use error message for fail.
	// For warn, we still show observations but fall through to error if no observations.
	if lr.Status == core.StatusFail && lr.Error != nil {
		return lr.Error.Message
	}

	if lr.Status == core.StatusSkip {
		// Reachability skip shows skip_reason.
		if name == "reachability" {
			if obs := obsMap(lr); obs != nil {
				if reason, ok := obs["skip_reason"].(string); ok {
					return strings.ReplaceAll(reason, "_", " ")
				}
			}
		}
		return ""
	}

	obs := obsMap(lr)
	if obs == nil {
		if lr.Error != nil {
			return lr.Error.Message
		}
		return ""
	}

	switch name {
	case "dns":
		return dnsDescription(obs, lr, useColor)
	case "reachability":
		return reachabilityDescription(obs, lr)
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

// reachabilityDescription builds the description for the reachability layer.
func reachabilityDescription(obs map[string]any, lr *core.LayerResult) string {
	method, _ := obs["probe_method"].(string)
	if method == "" {
		method, _ = obs["method"].(string)
	}
	if method != "" {
		return method
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
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

	var desc string
	if queryName != "" && firstAnswer != "" {
		desc = queryName + arrow + firstAnswer
	} else if queryName != "" {
		desc = queryName
	} else if lr.Error != nil {
		desc = lr.Error.Message
	}

	if hint, ok := obs["dns_error_hint"].(string); ok && hint != "" {
		if desc != "" {
			desc += ", hint: " + hint
		} else {
			desc = "hint: " + hint
		}
	}

	return desc
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
	if subject, ok := obs["cert_subject"].(string); ok && subject != "" {
		parts = append(parts, subject)
	}
	if issuer, ok := obs["cert_issuer"].(string); ok && issuer != "" {
		parts = append(parts, "issuer: "+issuer)
	}
	if verified, ok := obs["cert_verified"].(bool); ok && !verified {
		parts = append(parts, "(unverified)")
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

// renderLayerLine writes a single layer line to w.
func renderLayerLine(w io.Writer, name string, lr *core.LayerResult, maxDurLen int, useColor bool) error {
	sym := statusSymbol(lr.Status, useColor)
	dur := formatDuration(lr.DurationMS)
	desc := layerDescription(name, lr, useColor)

	// Right-align the duration field.
	paddedDur := fmt.Sprintf("%*s", maxDurLen, dur)

	line := fmt.Sprintf("  %-12s  %s  %s  %s", name, sym, paddedDur, desc)
	_, err := fmt.Fprintln(w, line)
	return err
}

// deprecatedVersions is the set of TLS versions considered deprecated.
var deprecatedVersions = map[string]bool{
	"TLSv1.0": true,
	"TLSv1.1": true,
}

// renderTLSScanLine writes the TLS scan sub-line if tls_scan data is present.
func renderTLSScanLine(w io.Writer, lr *core.LayerResult, useColor bool) error {
	obs := obsMap(lr)
	if obs == nil {
		return nil
	}
	scanData, ok := obs["tls_scan"]
	if !ok {
		return nil
	}
	scanMap, ok := scanData.(map[string]any)
	if !ok {
		return nil
	}
	attemptsRaw := scanMap["attempts"]
	if attemptsRaw == nil {
		return nil
	}

	// Handle both []any (from JSON round-trip) and []map[string]any (from direct Probe).
	var attemptMaps []map[string]any
	switch a := attemptsRaw.(type) {
	case []any:
		for _, v := range a {
			if m, ok := v.(map[string]any); ok {
				attemptMaps = append(attemptMaps, m)
			}
		}
	case []map[string]any:
		attemptMaps = a
	}
	if len(attemptMaps) == 0 {
		return nil
	}

	var parts []string
	for _, am := range attemptMaps {
		version, _ := am["version"].(string)
		supported, _ := am["supported"].(bool)

		var sym string
		if supported && deprecatedVersions[version] {
			sym = scanStatusSymbol(core.StatusWarn, useColor)
		} else if supported {
			sym = scanStatusSymbol(core.StatusOK, useColor)
		} else {
			sym = scanStatusSymbol(core.StatusFail, useColor)
		}
		parts = append(parts, fmt.Sprintf("%s %s", version, sym))
	}

	line := fmt.Sprintf("    scan: %s", strings.Join(parts, " | "))
	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}

	// Show legend if any deprecated versions were found.
	for _, am := range attemptMaps {
		version, _ := am["version"].(string)
		supported, _ := am["supported"].(bool)
		if supported && deprecatedVersions[version] {
			legend := "deprecated"
			if useColor {
				legend = colorYellow + "\u26a0" + colorReset + " = deprecated"
			} else {
				legend = "[!!] = deprecated"
			}
			if _, err := fmt.Fprintf(w, "           %s\n", legend); err != nil {
				return err
			}
			break
		}
	}

	return nil
}

// scanStatusSymbol returns a compact symbol for the scan sub-line.
func scanStatusSymbol(s core.Status, useColor bool) string {
	if useColor {
		switch s {
		case core.StatusOK:
			return colorGreen + "\u2713" + colorReset
		case core.StatusWarn:
			return colorYellow + "\u26a0" + colorReset
		case core.StatusFail:
			return colorRed + "\u2717" + colorReset
		default:
			return "?"
		}
	}
	switch s {
	case core.StatusOK:
		return "[ok]"
	case core.StatusWarn:
		return "[!!]"
	case core.StatusFail:
		return "[FAIL]"
	default:
		return "[??]"
	}
}

// renderResponseHeadersLines writes HTTP response_headers as sub-lines.
func renderResponseHeadersLines(w io.Writer, lr *core.LayerResult) error {
	obs := obsMap(lr)
	if obs == nil {
		return nil
	}
	headersRaw, ok := obs["response_headers"]
	if !ok {
		return nil
	}

	var headers map[string]string
	switch h := headersRaw.(type) {
	case map[string]any:
		headers = make(map[string]string, len(h))
		for k, v := range h {
			headers[k] = fmt.Sprintf("%v", v)
		}
	case map[string]string:
		headers = h
	default:
		return nil
	}

	if len(headers) == 0 {
		return nil
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sortStrings(keys)

	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "    %s: %s\n", k, headers[k]); err != nil {
			return err
		}
	}
	return nil
}

// sortStrings sorts a slice of strings in place (simple insertion sort to avoid importing sort).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// RenderCount writes the CountResult as a human-readable table to w.
func RenderCount(w io.Writer, r *core.CountResult, useColor bool) error {
	totalAttempts := len(r.Attempts)

	for i, attempt := range r.Attempts {
		// Blank line between attempts.
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}

		// Attempt header.
		if _, err := fmt.Fprintf(w, "  Attempt %d/%d\n", attempt.Attempt, totalAttempts); err != nil {
			return err
		}

		// Compute max duration width for this attempt.
		maxDurLen := 0
		for _, name := range layerOrder {
			lr := attempt.Layers[name]
			if lr == nil {
				continue
			}
			dur := formatDuration(lr.DurationMS)
			if len(dur) > maxDurLen {
				maxDurLen = len(dur)
			}
		}

		// Render layer lines.
		for _, name := range layerOrder {
			lr := attempt.Layers[name]
			if lr == nil {
				continue
			}
			if err := renderLayerLine(w, name, lr, maxDurLen, useColor); err != nil {
				return err
			}
		}
	}

	// Statistics section.
	if _, err := fmt.Fprintf(w, "\n  Statistics (%d attempts):\n", totalAttempts); err != nil {
		return err
	}

	for _, name := range layerOrder {
		ls := r.Statistics[name]
		if ls == nil {
			continue
		}

		p50Str := "-"
		if ls.P50MS != nil {
			p50Str = fmt.Sprintf("%.0fms", *ls.P50MS)
		}
		p95Str := "-"
		if ls.P95MS != nil {
			p95Str = fmt.Sprintf("%.0fms", *ls.P95MS)
		}
		lossStr := fmt.Sprintf("%.1f%%", ls.LossRatio*100)

		line := fmt.Sprintf("    %-12s  p50: %5s  p95: %5s  loss: %s", name, p50Str, p95Str, lossStr)
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}

	// Exit code.
	if _, err := fmt.Fprintf(w, "\n  exit %d\n", r.ExitCode); err != nil {
		return err
	}

	return nil
}

// formatSummary builds the summary line.
func formatSummary(s *core.Summary) string {
	wallClock := fmt.Sprintf("%dms total", int(math.Round(s.WallClockMS)))

	if s.FirstNonOKLayer == "" {
		return wallClock + " | all ok"
	}

	return fmt.Sprintf("%s | first issue: %s | exit %d", wallClock, s.FirstNonOKLayer, s.ExitCode)
}
