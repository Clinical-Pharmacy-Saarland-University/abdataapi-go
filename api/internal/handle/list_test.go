package handle_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"observeddb-go-api/internal/handle"

	"github.com/gin-gonic/gin"
)

func TestNormalizeList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "single comma-joined value splits",
			in:   []string{"a,b,c"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "single value without comma stays single item",
			in:   []string{"a"},
			want: []string{"a"},
		},
		{
			name: "two values returned verbatim",
			in:   []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "three values returned verbatim",
			in:   []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
		{
			// Documented, intentional limitation: a lone value is always comma-split,
			// because it cannot be distinguished from a legacy comma-joined list.
			name: "single comma-bearing value IS split (documented limitation)",
			in:   []string{"Mirtazapin-0,5-Wasser"},
			want: []string{"Mirtazapin-0", "5-Wasser"},
		},
		{
			// With >=2 values the repeated form is used verbatim, so an intra-item
			// comma is preserved.
			name: "intra-item comma preserved when >=2 values",
			in:   []string{"Mirtazapin-0,5-Wasser", "Apixaban"},
			want: []string{"Mirtazapin-0,5-Wasser", "Apixaban"},
		},
		{
			name: "empty slice returned verbatim",
			in:   []string{},
			want: []string{},
		},
		{
			name: "single empty value splits to single empty item",
			in:   []string{""},
			want: []string{""},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := handle.NormalizeList(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NormalizeList(%#v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsEmptyList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want bool
	}{
		{name: "empty slice is empty", in: []string{}, want: true},
		{name: "single empty value is empty", in: []string{""}, want: true},
		{name: "single non-empty value is not empty", in: []string{"a"}, want: false},
		{name: "value plus empty is not empty", in: []string{"a", ""}, want: false},
		{name: "two empty values is not empty", in: []string{"", ""}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := handle.IsEmptyList(tt.in)
			if got != tt.want {
				t.Errorf("IsEmptyList(%#v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestQueryList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		rawQuery string
		want     []string
	}{
		{
			name:     "single comma-joined value splits",
			rawQuery: "names=a,b,c",
			want:     []string{"a", "b", "c"},
		},
		{
			name:     "repeated values returned verbatim",
			rawQuery: "names=a&names=b",
			want:     []string{"a", "b"},
		},
		{
			name:     "repeated values preserve intra-item comma",
			rawQuery: "names=Mirtazapin-0,5-Wasser&names=X",
			want:     []string{"Mirtazapin-0,5-Wasser", "X"},
		},
		{
			// Documented limitation: a lone comma-bearing value is still split.
			name:     "single comma-bearing value IS split (documented limitation)",
			rawQuery: "names=Mirtazapin-0,5-Wasser",
			want:     []string{"Mirtazapin-0", "5-Wasser"},
		},
		{
			name:     "empty query parameter yields single empty item",
			rawQuery: "names=",
			want:     []string{""},
		},
		{
			name:     "missing query parameter yields no items",
			rawQuery: "",
			want:     nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/x?"+tt.rawQuery, nil)

			got := handle.QueryList(c, "names")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("QueryList with %q = %#v, want %#v", tt.rawQuery, got, tt.want)
			}
		})
	}
}
