package helper_test

import (
	"errors"
	"sort"
	"testing"

	"observeddb-go-api/internal/utils/helper"
)

func TestIsUnique(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slice []int
		want  bool
	}{
		{name: "empty slice", slice: []int{}, want: true},
		{name: "nil slice", slice: nil, want: true},
		{name: "single element", slice: []int{1}, want: true},
		{name: "all unique", slice: []int{1, 2, 3, 4}, want: true},
		{name: "one duplicate", slice: []int{1, 2, 2, 3}, want: false},
		{name: "all same", slice: []int{7, 7, 7}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := helper.IsUnique(tt.slice); got != tt.want {
				t.Errorf("IsUnique(%v) = %v, want %v", tt.slice, got, tt.want)
			}
		})
	}
}

func TestIsUniqueStrings(t *testing.T) {
	t.Parallel()

	if !helper.IsUnique([]string{"a", "b", "c"}) {
		t.Error("expected distinct strings to be unique")
	}
	if helper.IsUnique([]string{"a", "a"}) {
		t.Error("expected duplicate strings to not be unique")
	}
}

// equalAsSet compares two int slices ignoring order and treating them as sets
// (both must contain the same elements; assumes inputs already have unique
// elements as produced by Unique).
func equalAsSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]int(nil), a...)
	bs := append([]int(nil), b...)
	sort.Ints(as)
	sort.Ints(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func TestUnique(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slice []int
		want  []int
	}{
		{name: "empty slice", slice: []int{}, want: []int{}},
		{name: "nil slice", slice: nil, want: []int{}},
		{name: "already unique", slice: []int{1, 2, 3}, want: []int{1, 2, 3}},
		{name: "with duplicates", slice: []int{1, 2, 2, 3, 3, 3}, want: []int{1, 2, 3}},
		{name: "all same", slice: []int{5, 5, 5}, want: []int{5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := helper.Unique(tt.slice)
			if !equalAsSet(got, tt.want) {
				t.Errorf("Unique(%v) = %v, want (as set) %v", tt.slice, got, tt.want)
			}
		})
	}
}

func TestSetDifference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    []int
		b    []int
		want []int
	}{
		{name: "empty a", a: []int{}, b: []int{1, 2}, want: []int{}},
		{name: "empty b", a: []int{1, 2, 3}, b: []int{}, want: []int{1, 2, 3}},
		{name: "partial overlap", a: []int{1, 2, 3, 4}, b: []int{2, 4}, want: []int{1, 3}},
		{name: "full overlap", a: []int{1, 2}, b: []int{1, 2, 3}, want: []int{}},
		{name: "no overlap", a: []int{1, 2}, b: []int{3, 4}, want: []int{1, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := helper.SetDifference(tt.a, tt.b)
			if !equalAsSet(got, tt.want) {
				t.Errorf("SetDifference(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSetDifferencePreservesOrderAndDuplicates(t *testing.T) {
	t.Parallel()

	// SetDifference keeps elements of a (including duplicates) in a's order.
	got := helper.SetDifference([]int{3, 1, 3, 2}, []int{2})
	want := []int{3, 1, 3}
	if len(got) != len(want) {
		t.Fatalf("SetDifference length = %v, want %v (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SetDifference order mismatch at %d: got %v, want %v", i, got, want)
		}
	}
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStrSetDifference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{name: "empty a", a: nil, b: []string{"x"}, want: nil},
		{name: "empty b", a: []string{"x", "y"}, b: nil, want: []string{"x", "y"}},
		{
			name: "case insensitive removal",
			a:    []string{"Apple", "Banana", "Cherry"},
			b:    []string{"apple", "CHERRY"},
			want: []string{"Banana"},
		},
		{
			name: "preserves original casing of a",
			a:    []string{"FooBar"},
			b:    []string{"other"},
			want: []string{"FooBar"},
		},
		{
			name: "all removed",
			a:    []string{"a", "B"},
			b:    []string{"A", "b"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := helper.StrSetDifference(tt.a, tt.b)
			if !equalStrSlice(got, tt.want) {
				t.Errorf("StrSetDifference(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSwapMap(t *testing.T) {
	t.Parallel()

	in := map[string]int{"a": 1, "b": 2, "c": 3}
	got := helper.SwapMap(in)
	want := map[int]string{1: "a", 2: "b", 3: "c"}

	if len(got) != len(want) {
		t.Fatalf("SwapMap length = %d, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("SwapMap[%v] = %q, want %q", k, got[k], v)
		}
	}
}

func TestSwapMapEmpty(t *testing.T) {
	t.Parallel()

	got := helper.SwapMap(map[string]int{})
	if len(got) != 0 {
		t.Errorf("SwapMap(empty) = %v, want empty map", got)
	}
}

func TestUpdateField(t *testing.T) {
	t.Parallel()

	okValidator := func(int) error { return nil }
	sentinelErr := errors.New("invalid value")
	failValidator := func(int) error { return sentinelErr }

	t.Run("from nil is a no-op returning nil", func(t *testing.T) {
		t.Parallel()
		to := 10
		validatorCalled := false
		validator := func(int) error {
			validatorCalled = true
			return nil
		}
		err := helper.UpdateField(&to, nil, validator)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if to != 10 {
			t.Errorf("expected 'to' unchanged at 10, got %d", to)
		}
		if validatorCalled {
			t.Error("validator should not be called when from is nil")
		}
	})

	t.Run("validator error is propagated and to unchanged", func(t *testing.T) {
		t.Parallel()
		to := 10
		from := 42
		err := helper.UpdateField(&to, &from, failValidator)
		if !errors.Is(err, sentinelErr) {
			t.Errorf("expected sentinel error, got %v", err)
		}
		if to != 10 {
			t.Errorf("expected 'to' unchanged at 10, got %d", to)
		}
	})

	t.Run("success updates to", func(t *testing.T) {
		t.Parallel()
		to := 10
		from := 42
		err := helper.UpdateField(&to, &from, okValidator)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if to != 42 {
			t.Errorf("expected 'to' updated to 42, got %d", to)
		}
	})
}

func TestRemoveTrailingSlash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "no trailing slash", in: "path", want: "path"},
		{name: "single trailing slash", in: "path/", want: "path"},
		{name: "only removes one slash", in: "path//", want: "path/"},
		{name: "just a slash", in: "/", want: ""},
		{name: "leading slash kept", in: "/path", want: "/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := helper.RemoveTrailingSlash(tt.in); got != tt.want {
				t.Errorf("RemoveTrailingSlash(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAddLeadingSlash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "no leading slash", in: "path", want: "/path"},
		{name: "already has leading slash", in: "/path", want: "/path"},
		{name: "just a slash", in: "/", want: "/"},
		{name: "trailing slash preserved", in: "path/", want: "/path/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := helper.AddLeadingSlash(tt.in); got != tt.want {
				t.Errorf("AddLeadingSlash(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
