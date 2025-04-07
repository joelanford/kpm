package write

import (
	"encoding/json"
	"io"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

func JSON(w io.Writer, g *graphv1.Graph) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(g)
}
