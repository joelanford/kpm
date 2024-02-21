package maps

import (
	"cmp"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
)

func MergeStrict[K cmp.Ordered, V any](m1, m2 map[K]V) (map[K]V, error) {
	m := make(map[K]V, len(m1)+len(m2))
	for k, v := range m1 {
		m[k] = v
	}
	duplicateKeys := sets.New[K]()
	for k, v := range m2 {
		if _, ok := m[k]; ok {
			duplicateKeys.Insert(k)
			continue
		}
		m[k] = v
	}
	if len(duplicateKeys) > 0 {
		return nil, fmt.Errorf("duplicate keys: %v", sets.List[K](duplicateKeys))
	}
	return m, nil
}
