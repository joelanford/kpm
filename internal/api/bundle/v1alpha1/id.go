package v1alpha1

import (
	"fmt"
	"path"

	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joelanford/kpm/internal/api/specs/v1"
)

const (
	KindID      = "ID"
	MediaTypeID = "application/vnd.io.operatorframework.kpm.bundle.v1alpha1.id+json"
)

type ID struct {
	metav1.TypeMeta `json:",inline"`

	Name    string         `json:"name"`
	Version semver.Version `json:"version"`
	Release Release        `json:"release"`
}

func NewID(name string, version semver.Version, release Release) ID {
	return ID{
		TypeMeta: metav1.TypeMeta{
			Kind:       KindID,
			APIVersion: v1.GroupVersion.String(),
		},
		Name:    name,
		Version: version,
		Release: release,
	}
}

func (i ID) Filename() string {
	return fmt.Sprintf("%s.bundle.kpm", i.String())
}

func (i ID) FullyQualifiedReference(namespace string) (reference.NamedTagged, error) {
	refStr := path.Join(namespace, i.UnqualifiedReference())
	fullRef, err := reference.ParseNamed(refStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse reference %q: %v", refStr, err)
	}
	return fullRef.(reference.NamedTagged), nil
}

func (i ID) UnqualifiedReference() string {
	return fmt.Sprintf("%s:%s-%s", i.Name, i.Version.String(), i.Release.String())
}

func (i ID) String() string {
	return fmt.Sprintf("%s.v%s-%s", i.Name, i.Version.String(), i.Release.String())
}
