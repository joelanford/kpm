package graph

import (
	"github.com/opencontainers/go-digest"

	graphv1 "github.com/joelanford/kpm/internal/experimental/api/graph/v1"
)

type Index struct {
	graph     *graphv1.Graph
	nvrToNode map[graphv1.NVR]digest.Digest

	nodeToTags map[digest.Digest][]digest.Digest
	edgeToTags map[digest.Digest][]digest.Digest

	//           key        value
	tagToNodes map[string]map[string][]digest.Digest
	tagToEdges map[string]map[string][]digest.Digest
}

func NewIndex() *Index {
	return &Index{
		graph: &graphv1.Graph{
			MediaType:          graphv1.MediaTypeGraph,
			Nodes:              map[digest.Digest]graphv1.Node{},
			Edges:              map[digest.Digest]graphv1.Edge{},
			Tags:               map[digest.Digest]graphv1.Tag{},
			ReferenceOnlyNodes: map[digest.Digest]graphv1.Node{},
		},
		nvrToNode:  map[graphv1.NVR]digest.Digest{},
		nodeToTags: map[digest.Digest][]digest.Digest{},
		edgeToTags: map[digest.Digest][]digest.Digest{},
		tagToNodes: map[string]map[string][]digest.Digest{},
		tagToEdges: map[string]map[string][]digest.Digest{},
	}
}

func (idx *Index) AddNode(n graphv1.Node, tags map[string][]string) digest.Digest {
	nodeDigest := digestOf(n)
	if existingDigest, ok := idx.nvrToNode[n.NVR]; ok && existingDigest != nodeDigest {
		panic("existing NVR found with different node digest")
	}
	idx.graph.Nodes[nodeDigest] = n
	idx.nvrToNode[n.NVR] = nodeDigest

	for key, values := range tags {
		for _, value := range values {
			idx.TagNode(nodeDigest, key, value)
		}
	}
	return nodeDigest
}

func (idx *Index) TagNode(nodeDigest digest.Digest, key, value string) digest.Digest {
	tag := graphv1.Tag{
		MediaType: graphv1.MediaTypeTag,
		Scope:     graphv1.ScopeNode,
		Key:       key,
		Value:     value,
		Reference: nodeDigest,
	}
	tagDigest := digestOf(tag)
	idx.graph.Tags[tagDigest] = tag
	idx.nodeToTags[nodeDigest] = append(idx.nodeToTags[nodeDigest], tagDigest)

	if idx.tagToNodes[key] == nil {
		idx.tagToNodes[key] = map[string][]digest.Digest{}
	}
	idx.tagToNodes[key][value] = append(idx.tagToNodes[key][value], nodeDigest)
	return tagDigest
}

func (idx *Index) AddEdge(e graphv1.Edge, tags map[string][]string) {
	edgeDigest := digestOf(e)

	if _, ok := idx.graph.Nodes[e.From]; !ok {
		panic("from node not found")
	}
	if _, ok := idx.graph.Edges[e.To]; !ok {
		panic("to node not found")
	}

	if _, ok := idx.graph.Edges[edgeDigest]; ok {
		panic("duplicate edge found")
	}
	for key, values := range tags {
		for _, value := range values {
			idx.TagEdge(edgeDigest, key, value)
		}
	}
	idx.graph.Edges[edgeDigest] = e
}

func (idx *Index) TagEdge(edgeDigest digest.Digest, key, value string) digest.Digest {
	tag := graphv1.Tag{
		MediaType: graphv1.MediaTypeTag,
		Scope:     graphv1.ScopeNode,
		Key:       key,
		Value:     value,
		Reference: edgeDigest,
	}
	tagDigest := digestOf(tag)
	idx.graph.Tags[tagDigest] = tag
	idx.edgeToTags[edgeDigest] = append(idx.edgeToTags[edgeDigest], tagDigest)

	if idx.tagToEdges[key] == nil {
		idx.tagToEdges[key] = map[string][]digest.Digest{}
	}
	idx.tagToEdges[key][value] = append(idx.tagToEdges[key][value], edgeDigest)
	return tagDigest
}
