package interactioncontroller

import (
	"reflect"
	"testing"
)

func TestNormalizeInteractionAnnotationLang(t *testing.T) {
	got, err := normalizeInteractionAnnotationLang("en")
	if err != nil {
		t.Fatalf("normalizeInteractionAnnotationLang() returned error: %v", err)
	}
	if got != "english" {
		t.Fatalf("normalizeInteractionAnnotationLang() = %q, want english", got)
	}

	if _, err := normalizeInteractionAnnotationLang("french"); err == nil {
		t.Fatal("normalizeInteractionAnnotationLang() expected error for invalid language")
	}
}

func TestParseAnnotationStringList(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "json string", raw: `"prostaglandins"`, want: []string{"prostaglandins"}},
		{name: "json array", raw: `["CYP1A2","CYP3A4"]`, want: []string{"CYP1A2", "CYP3A4"}},
		{name: "empty array", raw: `[]`, want: []string{}},
	}

	for _, tt := range tests {
		got := parseAnnotationStringList(&tt.raw)
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s: parseAnnotationStringList() = %#v, want %#v", tt.name, got, tt.want)
		}
	}
}

func TestLocalizeInteractionAnnotationTokens(t *testing.T) {
	gotEnglish := localizeInteractionAnnotationTokens("enzyme_inhibition; additive_effect", "english", interactionMechanismLabels)
	wantEnglish := []string{"enzyme inhibition", "additive effect"}
	if !reflect.DeepEqual(gotEnglish, wantEnglish) {
		t.Fatalf("localizeInteractionAnnotationTokens() = %#v, want %#v", gotEnglish, wantEnglish)
	}

	gotGerman := localizeInteractionAnnotationTokens("enzyme_inhibition; additive_effect", "german", interactionMechanismLabels)
	wantGerman := []string{"Enzymhemmung", "additive Wirkung"}
	if !reflect.DeepEqual(gotGerman, wantGerman) {
		t.Fatalf("localizeInteractionAnnotationTokens() = %#v, want %#v", gotGerman, wantGerman)
	}
}

func TestLocalizeInteractionAnnotationValue(t *testing.T) {
	got := localizeInteractionAnnotationValue("high", "german", confidenceLevelLabels)
	if got != "hoch" {
		t.Fatalf("localizeInteractionAnnotationValue() = %q, want hoch", got)
	}
}
