package mem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/google/renameio"
	"github.com/opencontainers/go-digest"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
	"github.com/joelanford/kpm/internal/experimental/graph"
	"github.com/joelanford/kpm/internal/experimental/graph/index"
	"github.com/joelanford/kpm/internal/experimental/graph/write"
)

type Index struct {
	graphDir   string
	graph      *graphv1.Graph
	nvrToNodes map[graphv1.NVR][]digest.Digest
}

func Open(path string) (*Index, error) {
	g := index.NewGraph()
	graphFile := filepath.Join(path, "graph.json")
	graphData, err := os.ReadFile(graphFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(graphFile), 0700); err != nil {
			return nil, err
		}
		f, err := os.Create(graphFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if err := write.JSON(f, g); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(graphData, g); err != nil {
			return nil, err
		}
	}
	idx := newIndexFromGraph(g)
	idx.graphDir = path
	return idx, nil
}

func newIndexFromGraph(graph *graphv1.Graph) *Index {
	idx := &Index{
		graph:      graph,
		nvrToNodes: make(map[graphv1.NVR][]digest.Digest),
	}
	for nodeDigest, node := range graph.Nodes {
		idx.nvrToNodes[node.NVR] = append(idx.nvrToNodes[node.NVR], nodeDigest)
	}
	return idx
}

func (idx *Index) Graph() *graphv1.Graph {
	return idx.graph
}

func (idx *Index) AddNode(n graphv1.Node, tags map[string][]string) (digest.Digest, error) {
	dgst := index.DigestOf(n)
	idx.graph.Nodes[dgst] = n
	idx.nvrToNodes[n.NVR] = append(idx.nvrToNodes[n.NVR], dgst)
	idx.graph.Tags[dgst] = index.MergeTags(idx.graph.Tags[dgst], tags)
	return dgst, nil
}

func (idx *Index) AddTag(dgst digest.Digest, key string, value string) error {
	idx.graph.Tags[dgst] = index.MergeTags(idx.graph.Tags[dgst], map[string][]string{key: {value}})
	return nil
}

func (idx *Index) AddEdge(e graphv1.Edge, tags map[string][]string) (digest.Digest, error) {
	dgst := index.DigestOf(e)

	if _, ok := idx.graph.Nodes[e.From]; !ok {
		return "", fmt.Errorf("edge.from %q not found in graph nodes", e.From)
	}
	if _, ok := idx.graph.Nodes[e.To]; !ok {
		return "", fmt.Errorf("edge.to %q not found in graph nodes", e.To)
	}
	idx.graph.Edges[dgst] = e
	idx.graph.Tags[dgst] = index.MergeTags(idx.graph.Tags[dgst], tags)
	return dgst, nil
}

func (idx *Index) Close() error {
	if idx.graphDir == "" {
		return nil
	}
	graphFile := filepath.Join(idx.graphDir, "graph.json")
	t, err := renameio.TempFile(idx.graphDir, graphFile)
	if err != nil {
		return err
	}
	defer t.Cleanup()

	if err := write.JSON(t, idx.Graph()); err != nil {
		return err
	}
	if err := t.CloseAtomicallyReplace(); err != nil {
		return err
	}
	return nil
}

func (idx *Index) Apply(ctx context.Context, f *graph.Selector) error {
	refNodes := map[digest.Digest]struct{}{}
	for dgst, edge := range idx.graph.Edges {
		match, err := f.MatchEdge(ctx, idx.graph, dgst)
		if err != nil {
			return err
		}
		if match {
			refNodes[edge.From] = struct{}{}
			refNodes[edge.To] = struct{}{}
		} else {
			delete(idx.graph.Edges, dgst)
			delete(idx.graph.Tags, dgst)
		}
	}

	for dgst, node := range idx.graph.Nodes {
		match, err := f.MatchNode(ctx, idx.graph, dgst)
		if err != nil {
			return err
		}
		if !match {
			delete(idx.graph.Nodes, dgst)
			delete(idx.graph.Tags, dgst)
			idx.nvrToNodes[node.NVR] = slices.DeleteFunc(idx.nvrToNodes[node.NVR], func(d digest.Digest) bool {
				return d == dgst
			})
			if _, ok := refNodes[dgst]; ok {
				idx.graph.ReferenceOnlyNodes[dgst] = node
			}
		}
	}
	return nil
}
