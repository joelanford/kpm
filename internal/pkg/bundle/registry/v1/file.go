package v1

import (
	"sigs.k8s.io/yaml"
)

type File[T any] struct {
	name  string
	data  []byte
	value T
}

func NewYAMLDataFile[T any](name string, data []byte) (*File[T], error) {
	var value T
	if err := yaml.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return &File[T]{name: name, data: data, value: value}, nil
}

func NewYAMLValueFile[T any](name string, value T) (*File[T], error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &File[T]{name: name, data: data, value: value}, nil
}

func NewPrecomputedFile[T any](name string, data []byte, value T) File[T] {
	return File[T]{name: name, data: data, value: value}
}

func (m File[T]) Name() string {
	return m.name
}

func (m File[T]) Data() []byte {
	return m.data
}

func (m File[T]) Value() T {
	return m.value
}
