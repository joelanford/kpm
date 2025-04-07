package v1

import (
	"github.com/Masterminds/semver/v3"
	"github.com/opencontainers/go-digest"
)

const (
	MediaTypeBundle        = "application/vnd.cncf.olm.bundle.v1+json"
	MediaTypeEdge          = "application/vnd.cncf.olm.edge.v1+json"
	MediaTypeTagDefinition = "application/vnd.cncf.olm.tagdefinition.v1+json"
	MediaTypeTag           = "application/vnd.cncf.olm.tag.v1+json"
	MediaTypeGraph         = "application/vnd.cncf.olm.graph.v1+json"

	ScopeNode = "node"
	ScopeEdge = "edge"
)

type NVR struct {
	Name    string         `json:"name"`
	Version semver.Version `json:"version"`
	Release uint64         `json:"release"`
}

type Node struct {
	MediaType string `json:"mediaType"`
	NVR       `json:",inline"`
	Reference string `json:"reference"`
}

type Edge struct {
	MediaType string `json:"mediaType"`

	From digest.Digest `json:"from"`
	To   digest.Digest `json:"to"`
}

type Tag struct {
	MediaType string `json:"mediaType"`
	Scope     string `json:"scope"`

	Key   string `json:"key"`
	Value string `json:"value"`

	Reference digest.Digest `json:"reference"`
}

type Graph struct {
	MediaType          string                 `json:"mediaType"`
	Nodes              map[digest.Digest]Node `json:"nodes"`
	ReferenceOnlyNodes map[digest.Digest]Node `json:"referenceOnlyNodes"`
	Edges              map[digest.Digest]Edge `json:"edges"`
	Tags               map[digest.Digest]Tag  `json:"tags"`
}
