package ociutil

import (
	"fmt"

	"github.com/containers/image/v5/docker/reference"
)

func ParseNamedTagged(ref string) (reference.NamedTagged, error) {
	named, err := reference.ParseNamed(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid reference %q: %w", ref, err)
	}
	namedTagged, ok := named.(reference.NamedTagged)
	if !ok {
		return nil, fmt.Errorf("invalid reference: %q is not a tagged reference", ref)
	}
	return namedTagged, nil
}
