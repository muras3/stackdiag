package json

import (
	"encoding/json"
	"io"

	"github.com/muras3/stackdiag/internal/core"
)

// Render writes the Result as indented JSON to w.
// HTML escaping is disabled to keep URLs readable.
func Render(w io.Writer, r *core.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// RenderCount writes the CountResult as indented JSON to w.
// HTML escaping is disabled to keep URLs readable.
func RenderCount(w io.Writer, r *core.CountResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
