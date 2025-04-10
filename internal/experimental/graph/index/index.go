package index

import (
	"crypto/sha256"
	"encoding/json"
	"slices"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/containers/image/v5/docker/reference"
	"github.com/opencontainers/go-digest"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

type Index interface {
	AddNode(n graphv1.Node, tags map[string][]string) (digest.Digest, error)
	AddEdge(e graphv1.Edge, tags map[string][]string) (digest.Digest, error)
	AddTag(digest digest.Digest, key string, value string) error
	Close() error
}

func MergeTags(a, b map[string][]string) map[string][]string {
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

var (
	digestBufPool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 0, 32)
			return &b
		},
	}
)

func DigestOf(v any) digest.Digest {
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
