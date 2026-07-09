package pzncontroller

import (
	"encoding/json"
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

func TestStandardNoteCategoryLabel(t *testing.T) {
	tests := []struct {
		keySTA string
		lang   string
		want   string
	}{
		{keySTA: "S001", lang: "german", want: "Schwangerschaft"},
		{keySTA: "S001", lang: "english", want: "Pregnancy"},
		{keySTA: "W001", lang: "english", want: "General note or warning"},
	}

	for _, tt := range tests {
		code := standardNoteCategoryCode(tt.keySTA)
		got := standardNoteCategoryLabel(code, tt.lang)
		if got != tt.want {
			t.Fatalf("standardNoteCategoryLabel(%q, %q) = %q, want %q", code, tt.lang, got, tt.want)
		}
	}
}

func TestNormalizeStandardNoteLang(t *testing.T) {
	got, err := normalizeStandardNoteLang("en")
	if err != nil {
		t.Fatalf("normalizeStandardNoteLang() returned error: %v", err)
	}
	if got != "english" {
		t.Fatalf("normalizeStandardNoteLang() = %q, want english", got)
	}

	if _, err := normalizeStandardNoteLang("french"); err == nil {
		t.Fatal("normalizeStandardNoteLang() expected error for invalid language")
	}
}

func TestTranslateStandardNoteText(t *testing.T) {
	got := translateStandardNoteText("S01", "Bisher kein Verdacht.", "english")
	want := "So far no suspicion of embryotoxic/teratogenic risk in humans (1st trimester)."
	if got != want {
		t.Fatalf("translateStandardNoteText() = %q, want %q", got, want)
	}

	got = translateStandardNoteText("S01", "Bisher kein Verdacht.", "german")
	if got != "Bisher kein Verdacht." {
		t.Fatalf("translateStandardNoteText() = %q, want German fallback", got)
	}
}

func TestProductSearchResultOmitsStandardNotesWhenEmpty(t *testing.T) {
	data, err := json.Marshal(ProductSearchResult{
		ProductName:     "Example",
		PZN:             "12345678",
		ActiveCompounds: []string{"Example compound"},
	})
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	if string(data) != `{"product_name":"Example","pzn":"12345678","active_compounds":["Example compound"]}` {
		t.Fatalf("json.Marshal() = %s", data)
	}
}
