package sqlb

// Args widens a typed slice to the []any that variadic builder methods (Cond.In) take.
func Args[T any](v []T) []any {
	out := make([]any, len(v))
	for i, x := range v {
		out[i] = x
	}
	return out
}
