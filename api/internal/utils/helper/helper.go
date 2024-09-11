package helper

import "strings"

func IsUnique[T comparable](slice []T) bool {
	unique := make(map[T]struct{}, len(slice))
	for _, v := range slice {
		unique[v] = struct{}{}
	}
	return len(unique) == len(slice)
}

func Unique[T comparable](slice []T) []T {
	unique := make(map[T]struct{}, len(slice))
	for _, v := range slice {
		unique[v] = struct{}{}
	}

	uniqueSlice := make([]T, 0, len(unique))
	for v := range unique {
		uniqueSlice = append(uniqueSlice, v)
	}

	return uniqueSlice
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

func StrSetDifference(a, b []string) []string {
	bMap := make(map[string]struct{})
	for _, item := range b {
		bMap[strings.ToLower(item)] = struct{}{}
	}

	var result []string
	for _, item := range a {
		if _, found := bMap[strings.ToLower(item)]; !found {
			result = append(result, item)
		}
	}

	return result
}

// SwapMap returns a new map with the keys and values of the input map swapped.
// The input map must have unique values.
func SwapMap[K comparable, V comparable](m map[K]V) map[V]K {
	switchedMap := make(map[V]K, len(m))
	for key, value := range m {
		switchedMap[value] = key
	}
	return switchedMap
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

func RemoveTrailingSlash(s string) string {
	if len(s) > 0 && s[len(s)-1] == '/' {
		return s[:len(s)-1]
	}
	return s
}

func AddLeadingSlash(s string) string {
	if len(s) > 0 && s[0] != '/' {
		return "/" + s
	}
	return s
}
