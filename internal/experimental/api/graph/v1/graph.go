package v1

import (
	"github.com/Masterminds/semver/v3"
	"github.com/opencontainers/go-digest"
)

const (
	MediaTypeGraph = "application/vnd.cncf.olm.graph.v1+json"
	MediaTypeNode  = "application/vnd.cncf.olm.node.v1+json"
	MediaTypeEdge  = "application/vnd.cncf.olm.edge.v1+json"
)

type Graph struct {
	MediaType          string                                `json:"mediaType"`
	Nodes              map[digest.Digest]Node                `json:"nodes"`
	ReferenceOnlyNodes map[digest.Digest]Node                `json:"referenceOnlyNodes"`
	Edges              map[digest.Digest]Edge                `json:"edges"`
	Tags               map[digest.Digest]map[string][]string `json:"tags"`
}

type Node struct {
	MediaType string `json:"mediaType"`
	NVR       `json:",inline"`
	Reference string `json:"reference"`
}

type NVR struct {
	Name    string         `json:"name"`
	Version semver.Version `json:"version"`
	Release uint64         `json:"release"`
}

type Edge struct {
	MediaType string `json:"mediaType"`

	From digest.Digest `json:"from"`
	To   digest.Digest `json:"to"`
}
