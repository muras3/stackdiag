package json

import (
	"encoding/json"
	"io"

	"github.com/muras3/stackdiag/internal/core"
)

// Render writes the Result as compact JSON to w.
// Compact format minimizes token count for AI agent consumption.
// HTML escaping is disabled to keep URLs readable.
func Render(w io.Writer, r *core.Result) error {
	return encode(w, r, false)
}

// RenderPretty writes the Result as indented JSON to w.
func RenderPretty(w io.Writer, r *core.Result) error {
	return encode(w, r, true)
}

// RenderCount writes the CountResult as compact JSON to w.
// HTML escaping is disabled to keep URLs readable.
func RenderCount(w io.Writer, r *core.CountResult) error {
	return encode(w, r, false)
}

// RenderCountPretty writes the CountResult as indented JSON to w.
func RenderCountPretty(w io.Writer, r *core.CountResult) error {
	return encode(w, r, true)
}

// encode writes v as JSON to w. When pretty is true, output is indented.
func encode(w io.Writer, v any, pretty bool) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

// toolError is the JSON structure for argument/target parse errors.
type toolError struct {
	SchemaVersion string           `json:"schema_version"`
	Error         *core.ProbeError `json:"error"`
	ExitCode      int              `json:"exit_code"`
}

// RenderToolError writes a structured JSON error for argument/target parse failures.
func RenderToolError(w io.Writer, code, message string, exitCode int) error {
	return renderToolError(w, code, message, exitCode, false)
}

// RenderToolErrorPretty writes a structured JSON error with indentation.
func RenderToolErrorPretty(w io.Writer, code, message string, exitCode int) error {
	return renderToolError(w, code, message, exitCode, true)
}

func renderToolError(w io.Writer, code, message string, exitCode int, pretty bool) error {
	e := toolError{
		SchemaVersion: core.SchemaVersion,
		Error:         &core.ProbeError{Code: code, Message: message},
		ExitCode:      exitCode,
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(e)
}
