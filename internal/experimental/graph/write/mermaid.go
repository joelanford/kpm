package write

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/opencontainers/go-digest"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

func MermaidURL(w io.Writer, g *graphv1.Graph) error {
	var codeBuf bytes.Buffer
	if err := Mermaid(&codeBuf, g); err != nil {
		return err
	}
	data := map[string]any{
		"code": codeBuf.String(),
		"mermaid": map[string]any{
			"theme": "default",
		},
		"autoSync":      true,
		"updateDiagram": true,
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var zBuf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&zBuf, 9)
	if err != nil {
		return err
	}
	if _, err := zw.Write(dataJSON); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(zBuf.Bytes())
	_, err = fmt.Fprintf(w, "https://mermaid.live/edit#pako:%s\n", encoded)
	return err
}

func Mermaid(w io.Writer, g *graphv1.Graph) error {
	fromCount := map[digest.Digest]int{}
	for _, edge := range g.Edges {
		fromCount[edge.From]++
	}

	mermaidNodes := make([]mermaidNode, 0, len(g.Nodes)+len(g.ReferenceOnlyNodes))
	for nodeDigest, node := range g.Nodes {
		class := "node"
		if fromCount[nodeDigest] == 0 {
			class = "source"
		}
		mermaidNodes = append(mermaidNodes, mermaidNode{
			ID:    nodeDigest.String(),
			NVR:   node.NVR,
			Class: class,
		})
	}
	for nodeDigest, node := range g.ReferenceOnlyNodes {
		if fromCount[nodeDigest] == 0 {
			return fmt.Errorf("invariant violation: reference-only nodes can never be graph sources")
		}
		mermaidNodes = append(mermaidNodes, mermaidNode{
			ID:    nodeDigest.String(),
			NVR:   node.NVR,
			Class: "referenceOnlyNode",
		})
	}
	slices.SortFunc(mermaidNodes, compareMermaidNodes)

	var sb bytes.Buffer

	_, _ = fmt.Fprintln(&sb, "graph LR")
	for _, node := range mermaidNodes {
		_, _ = fmt.Fprintf(&sb, "  %s[%s.v%s-%d]:::%s\n", node.ID, node.Name, node.Version, node.Release, node.Class)
	}
	for _, edge := range g.Edges {
		_, _ = fmt.Fprintf(&sb, "  %s --> %s\n", edge.From, edge.To)
	}
	_, _ = fmt.Fprintln(&sb, "  classDef source            fill:#e6fff0,stroke:#33cc66,stroke-width:2,color:#009933;")
	_, _ = fmt.Fprintln(&sb, "  classDef node              fill:#e6f0ff,stroke:#3366cc,stroke-width:2,color:#003399;")
	_, _ = fmt.Fprintln(&sb, "  classDef referenceOnlyNode fill:#f7f7f7,stroke:#cccccc,stroke-width:1,stroke-dasharray: 4 3,color:#999999;")

	_, err := w.Write(sb.Bytes())
	return err
}

type mermaidNode struct {
	ID string
	graphv1.NVR
	Class string
}

func compareMermaidNodes(a, b mermaidNode) int {
	if d := cmp.Compare(a.Name, b.Name); d != 0 {
		return d
	}
	if d := a.Version.Compare(&b.Version); d != 0 {
		return d
	}
	return cmp.Compare(a.Release, b.Release)
}
