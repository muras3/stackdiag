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
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// RenderCount writes the CountResult as compact JSON to w.
// HTML escaping is disabled to keep URLs readable.
func RenderCount(w io.Writer, r *core.CountResult) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// toolError is the JSON structure for argument/target parse errors.
type toolError struct {
	SchemaVersion string           `json:"schema_version"`
	Error         *core.ProbeError `json:"error"`
	ExitCode      int              `json:"exit_code"`
}

// RenderToolError writes a structured JSON error for argument/target parse failures.
func RenderToolError(w io.Writer, code, message string, exitCode int) error {
	e := toolError{
		SchemaVersion: core.SchemaVersion,
		Error:         &core.ProbeError{Code: code, Message: message},
		ExitCode:      exitCode,
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(e)
}
