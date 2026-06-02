package mappers

func mapValueSlice[A, B any](items []A, mapper func(*A) *B) []*B {
	out := make([]*B, 0, len(items))
	for i := range items {
		if mapped := mapper(&items[i]); mapped != nil {
			out = append(out, mapped)
		}
	}
	return out
}

func mapPointerSlice[A, B any](items []*A, mapper func(*A) *B) []B {
	out := make([]B, 0, len(items))
	for _, item := range items {
		if mapped := mapper(item); mapped != nil {
			out = append(out, *mapped)
		}
	}
	return out
}

func mapValueSliceErr[A, B any](items []A, mapper func(*A) (*B, error)) ([]*B, error) {
	out := make([]*B, 0, len(items))
	for i := range items {
		mapped, err := mapper(&items[i])
		if err != nil {
			return nil, err
		}
		if mapped != nil {
			out = append(out, mapped)
		}
	}
	return out, nil
}

func mapPointerSliceErr[A, B any](items []*A, mapper func(*A) (*B, error)) ([]B, error) {
	out := make([]B, 0, len(items))
	for _, item := range items {
		mapped, err := mapper(item)
		if err != nil {
			return nil, err
		}
		if mapped != nil {
			out = append(out, *mapped)
		}
	}
	return out, nil
}
