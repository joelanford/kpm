package v1

import (
	v1 "github.com/joelanford/kpm/internal/pkg/bundle/registry/v1"
)

type Package struct {
	Schema         string        `json:"schema"`
	Name           string        `json:"name"`
	DefaultChannel string        `json:"defaultChannel,omitempty"`
	Icon           *Icon         `json:"icon,omitempty"`
	Description    string        `json:"description,omitempty"`
	Properties     []v1.Property `json:"properties,omitempty"`
}

type Icon struct {
	Data      []byte `json:"base64data"`
	MediaType string `json:"mediatype"`
}
