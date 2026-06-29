package main

import (
	"encoding/json"
	"io"
)

// render writes v to w as indented JSON. HTML escaping is disabled so
// genealogy names (often Cyrillic) and URLs read naturally.
func render(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
