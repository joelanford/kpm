package graph

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/containers/image/v5/docker/reference"
	"github.com/opencontainers/go-digest"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

type Index struct {
	graph      *graphv1.Graph
	nvrToNodes map[graphv1.NVR][]digest.Digest
}

func NewIndexFromGraph(graph *graphv1.Graph) *Index {
	idx := &Index{
		graph:      graph,
		nvrToNodes: make(map[graphv1.NVR][]digest.Digest),
	}
	for nodeDigest, node := range graph.Nodes {
		idx.nvrToNodes[node.NVR] = append(idx.nvrToNodes[node.NVR], nodeDigest)
	}
	return idx
}

func NewIndex() *Index {
	return &Index{
		graph:      NewGraph(),
		nvrToNodes: map[graphv1.NVR][]digest.Digest{},
	}
}

func NewGraph() *graphv1.Graph {
	return &graphv1.Graph{
		MediaType:          graphv1.MediaTypeGraph,
		Nodes:              map[digest.Digest]graphv1.Node{},
		Edges:              map[digest.Digest]graphv1.Edge{},
		Tags:               map[digest.Digest]map[string][]string{},
		ReferenceOnlyNodes: map[digest.Digest]graphv1.Node{},
	}
}

func NewNode(name string, version semver.Version, release uint64, ref reference.Canonical) *graphv1.Node {
	return &graphv1.Node{
		MediaType: graphv1.MediaTypeNode,
		NVR: graphv1.NVR{
			Name:    name,
			Version: version,
			Release: release,
		},
		Reference: ref.String(),
	}
}

func NewEdge(from, to digest.Digest) graphv1.Edge {
	return graphv1.Edge{
		MediaType: graphv1.MediaTypeEdge,
		From:      from,
		To:        to,
	}
}

func (idx *Index) Graph() *graphv1.Graph {
	return idx.graph
}

func (idx *Index) AddNode(n graphv1.Node, tags map[string][]string) digest.Digest {
	dgst := digestOf(n)
	idx.graph.Nodes[dgst] = n
	idx.nvrToNodes[n.NVR] = append(idx.nvrToNodes[n.NVR], dgst)
	idx.graph.Tags[dgst] = mergeTags(idx.graph.Tags[dgst], tags)
	return dgst
}

func (idx *Index) AddTag(dgst digest.Digest, key, value string) {
	idx.graph.Tags[dgst] = mergeTags(idx.graph.Tags[dgst], map[string][]string{key: {value}})
}

func (idx *Index) AddEdge(e graphv1.Edge, tags map[string][]string) (digest.Digest, error) {
	dgst := digestOf(e)

	if _, ok := idx.graph.Nodes[e.From]; !ok {
		return "", fmt.Errorf("edge.from %q not found in graph nodes", e.From)
	}
	if _, ok := idx.graph.Nodes[e.To]; !ok {
		return "", fmt.Errorf("edge.to %q not found in graph nodes", e.To)
	}
	idx.graph.Edges[dgst] = e
	idx.graph.Tags[dgst] = mergeTags(idx.graph.Tags[dgst], tags)
	return dgst, nil
}

func mergeTags(a, b map[string][]string) map[string][]string {
	out := make(map[string][]string)
	keys := make(map[string]struct{})
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	for k := range keys {
		vals := append(slices.Clone(a[k]), b[k]...)
		slices.Sort(vals)
		vals = slices.Compact(vals)
		out[k] = vals
	}
	return out
}

var (
	digestBufPool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 0, 32)
			return &b
		},
	}
)

func digestOf(v any) digest.Digest {
	hasher := sha256.New()
	enc := json.NewEncoder(hasher)
	enc.SetIndent("", "")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}

	b := digestBufPool.Get().(*[]byte)
	defer func() {
		*b = (*b)[:0]
		digestBufPool.Put(b)
	}()
	return digest.NewDigestFromBytes(digest.SHA256, hasher.Sum(*b))
}
