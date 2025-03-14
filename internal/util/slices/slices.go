package slices

import (
	"cmp"
	"iter"
	"slices"
)

func Map[I, O any](input iter.Seq2[int, I], fn func(I) O) iter.Seq2[int, O] {
	return func(yield func(int, O) bool) {
		for idx, val := range input {
			if !yield(idx, fn(val)) {
				return
			}
		}
	}
}

func MapSlice[I, O any](input []I, fn func(I) O) []O {
	return Collect(Map(slices.All(input), fn))
}

func Collect[I any](input iter.Seq2[int, I]) []I {
	var result []I
	for _, i := range input {
		result = append(result, i)
	}
	return result
}

func Uniq[V comparable](input []V) []V {
	m := map[V]struct{}{}
	for _, val := range input {
		m[val] = struct{}{}
	}
	result := make([]V, 0, len(m))
	for val := range m {
		result = append(result, val)
	}
	return result
}

func Sort[V cmp.Ordered](input []V) []V {
	slices.Sort(input)
	return input
}
