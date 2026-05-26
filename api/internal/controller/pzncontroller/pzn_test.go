package pzncontroller

import (
	"reflect"
	"testing"
)

func TestProductNameQueryValuesPreservesEscapedCommas(t *testing.T) {
	got := productNameQueryValues("name=Delix+2%2C5,Plavix,Ramilich&limit=3", "name")
	want := []string{"Delix 2,5", "Plavix", "Ramilich"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("productNameQueryValues() = %#v, want %#v", got, want)
	}
}

func TestProductNameQueryValuesSupportsRepeatedParameters(t *testing.T) {
	got := productNameQueryValues("name=Delix+2%2C5&name=Plavix&limit=3", "name")
	want := []string{"Delix 2,5", "Plavix"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("productNameQueryValues() = %#v, want %#v", got, want)
	}
}
