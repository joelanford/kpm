package v1

import (
	"iter"

	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

const (
	ArtifactTypeUpgradeEdge        = "application/vnd.operatorframework.olm.upgrade.edge.v1+json"
	AnnotationUpgradeEdgeDirection = "olm.operatorframework.io/upgrade-edge-direction"

	UpgradeEdgeFrom UpgradeEdgeDirection = "from"
	UpgradeEdgeTo   UpgradeEdgeDirection = "to"
)

type UpgradeEdgeDirection string

var (
	_ oci.Artifact = (*UpgradeEdge)(nil)
)

type UpgradeEdge struct {
	From Bundle `oci:"artifact:artifactType=application/vnd.operatorframework.olm.bundle.v1+json;selector=olm.operatorframework.io/upgrade-edge-direction in (from)"`
	To   Bundle `oci:"artifact:artifactType=application/vnd.operatorframework.olm.bundle.v1+json;selector=olm.operatorframework.io/upgrade-edge-direction in (to)"`
}

func (u UpgradeEdge) ArtifactType() string {
	return ArtifactTypeUpgradeEdge
}

func (u UpgradeEdge) Config() oci.Blob {
	return nil
}

func (u UpgradeEdge) SubArtifacts() iter.Seq2[int, oci.Artifact] {
	return func(yield func(int, oci.Artifact) bool) {
		if !yield(0, &upgradeEdgeBundle{u.From, UpgradeEdgeFrom}) {
			return
		}
		yield(1, &upgradeEdgeBundle{u.To, UpgradeEdgeTo})
	}
}

func (u UpgradeEdge) Blobs() iter.Seq2[int, oci.Blob] {
	return func(yield func(int, oci.Blob) bool) {}
}

type upgradeEdgeBundle struct {
	Bundle
	direction UpgradeEdgeDirection
}

func (ueb *upgradeEdgeBundle) DescriptorAnnotations() map[string]string {
	m := ueb.Annotations()
	m[AnnotationUpgradeEdgeDirection] = string(ueb.direction)
	return m
}
