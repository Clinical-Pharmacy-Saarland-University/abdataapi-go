package helper

func IsUnique[T comparable](slice []T) bool {
	unique := make(map[T]struct{}, len(slice))
	for _, v := range slice {
		unique[v] = struct{}{}
	}
	return len(unique) == len(slice)
}

// SetDifference returns the elements in a that are not in b.
func SetDifference[T comparable](a, b []T) []T {
	bSet := make(map[T]struct{}, len(b))
	for _, v := range b {
		bSet[v] = struct{}{}
	}

	diff := make([]T, 0, len(a))
	for _, v := range a {
		if _, ok := bSet[v]; !ok {
			diff = append(diff, v)
		}
	}

	return diff
}

// UpdateField updates 'to' with the value of 'from' if 'from' is not nil
// and validates the new value with the provided validator function.
func UpdateField[T any](to *T, from *T, validator func(T) error) error {
	if from == nil {
		return nil
	}

	if err := validator(*from); err != nil {
		return err
	}

	*to = *from
	return nil
}
