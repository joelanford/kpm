package slicesutil

func MapSlice[I, O any](in []I, mapFunc func(I) O) []O {
	out := make([]O, 0, len(in))
	for _, i := range in {
		out = append(out, mapFunc(i))
	}
	return out
}
