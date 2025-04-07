package write

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"slices"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

func Mermaid(w io.Writer, g *graphv1.Graph) error {
	mermaidNodes := make([]mermaidNode, 0, len(g.Nodes)+len(g.ReferenceOnlyNodes))
	for nodeDigest, node := range g.Nodes {
		mermaidNodes = append(mermaidNodes, mermaidNode{
			ID:    nodeDigest.String(),
			NVR:   node.NVR,
			Class: "node",
		})
	}
	for nodeDigest, node := range g.ReferenceOnlyNodes {
		mermaidNodes = append(mermaidNodes, mermaidNode{
			ID:    nodeDigest.String(),
			NVR:   node.NVR,
			Class: "referenceOnlyNode",
		})
	}
	slices.SortFunc(mermaidNodes, compareMermaidNodes)

	var sb bytes.Buffer

	_, _ = fmt.Fprintln(&sb, "graph BT")
	for _, node := range mermaidNodes {
		_, _ = fmt.Fprintf(&sb, "  %s[%s.v%s-%d]:::%s\n", node.ID, node.Name, node.Version, node.Release, node.Class)
	}
	for _, edge := range g.Edges {
		_, _ = fmt.Fprintf(&sb, "  %s --> %s\n", edge.From, edge.To)
	}
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
