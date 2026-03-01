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

// layerDescription builds the description string for a layer line.
func layerDescription(name string, lr *core.LayerResult, useColor bool) string {
	// If the layer failed and has an error, use error message.
	if lr.Status == core.StatusFail && lr.Error != nil {
		return lr.Error.Message
	}

	if lr.Status == core.StatusSkip {
		if name == "reachability" {
			if obs, ok := lr.Observations.(*core.ReachabilityObservations); ok && obs.SkipReason != nil {
				return strings.ReplaceAll(*obs.SkipReason, "_", " ")
			}
		}
		return ""
	}

	switch obs := lr.Observations.(type) {
	case *core.DNSObservations:
		return dnsDescription(obs, lr, useColor)
	case *core.ReachabilityObservations:
		return reachabilityDescription(obs, lr)
	case *core.TCPObservations:
		return tcpDescription(obs, lr)
	case *core.TLSObservations:
		return tlsDescription(obs, lr)
	case *core.HTTPObservations:
		return httpDescription(obs, lr)
	default:
		if lr.Error != nil {
			return lr.Error.Message
		}
		return ""
	}
}

// reachabilityDescription builds the description for the reachability layer.
func reachabilityDescription(obs *core.ReachabilityObservations, lr *core.LayerResult) string {
	if obs.ProbeMethod != "" {
		return obs.ProbeMethod
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
}

// dnsDescription builds: "{query_name} → {first_answer}" or error message.
func dnsDescription(obs *core.DNSObservations, lr *core.LayerResult, useColor bool) string {
	queryName := obs.QueryName
	var firstAnswer string
	if len(obs.Answers) > 0 {
		firstAnswer = obs.Answers[0]
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

	if obs.DNSErrorHint != nil && *obs.DNSErrorHint != "" {
		if desc != "" {
			desc += ", hint: " + *obs.DNSErrorHint
		} else {
			desc = "hint: " + *obs.DNSErrorHint
		}
	}

	return desc
}

// tcpDescription builds: ":{port}" or error message.
func tcpDescription(obs *core.TCPObservations, lr *core.LayerResult) string {
	if obs.RemotePort != 0 {
		return fmt.Sprintf(":%d", obs.RemotePort)
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
}

// tlsDescription builds: "{version}, cert expires in {days}d" or error message.
func tlsDescription(obs *core.TLSObservations, lr *core.LayerResult) string {
	var parts []string
	if obs.Version != "" {
		parts = append(parts, obs.Version)
	}
	if obs.CertDaysUntilExpiry != nil {
		parts = append(parts, fmt.Sprintf("cert expires in %dd", *obs.CertDaysUntilExpiry))
	}
	if obs.CertSubject != nil && *obs.CertSubject != "" {
		parts = append(parts, *obs.CertSubject)
	}
	if obs.CertIssuer != nil && *obs.CertIssuer != "" {
		parts = append(parts, "issuer: "+*obs.CertIssuer)
	}
	if obs.CertVerified != nil && !*obs.CertVerified {
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
func httpDescription(obs *core.HTTPObservations, lr *core.LayerResult) string {
	if obs.StatusCode != 0 {
		if obs.StatusText != "" {
			return fmt.Sprintf("%d %s", obs.StatusCode, obs.StatusText)
		}
		return fmt.Sprintf("%d", obs.StatusCode)
	}
	if lr.Error != nil {
		return lr.Error.Message
	}
	return ""
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
	tlsObs, ok := lr.Observations.(*core.TLSObservations)
	if !ok || tlsObs == nil || tlsObs.TLSScan == nil {
		return nil
	}
	attemptsRaw := tlsObs.TLSScan["attempts"]
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
	httpObs, ok := lr.Observations.(*core.HTTPObservations)
	if !ok || httpObs == nil || len(httpObs.ResponseHeaders) == 0 {
		return nil
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(httpObs.ResponseHeaders))
	for k := range httpObs.ResponseHeaders {
		keys = append(keys, k)
	}
	sortStrings(keys)

	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "    %s: %s\n", k, httpObs.ResponseHeaders[k]); err != nil {
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
