package format_test

import (
	"testing"

	"observeddb-go-api/internal/utils/format"
)

func intPtr(i int) *int { return &i }

// translatorCase describes a single expected translation for a known code.
type translatorCase struct {
	code         int
	wantShort    string
	wantDetailed string
}

func TestTranslators_KnownCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		translator func(*int, bool) *string
		cases      []translatorCase
	}{
		{
			name:       "IntPlausibility",
			translator: format.NewIntPlausibilityTranslator(),
			cases: []translatorCase{
				{10, "unknown mechanism", "Effects have only been observed, but there is no plausible mechanism."},
				{30, "mechanism confirmed", "There exists an explainable and proven mechanism."},
			},
		},
		{
			name:       "IntRelevance",
			translator: format.NewIntRelevanceTranslator(),
			cases: []translatorCase{
				{0, "no statement possible", "No assessment from the literature available."},
				{60, "contraindicated", "The interacting agents must not be combined."},
			},
		},
		{
			name:       "IntFrequency",
			translator: format.NewIntFrequencyTranslator(),
			cases: []translatorCase{
				{1, "very common", "Frequency of DDI >=10%"},
				{6, "not known", "Frequency of DDI is unknown."},
			},
		},
		{
			name:       "IntCredibility",
			translator: format.NewIntCredibilityTranslator(),
			cases: []translatorCase{
				{10, "not known", "Evidence for interaction is not known from the evaluated literature."},
				{50, "high", "Evidence for interaction is high from the evaluated literature."},
			},
		},
		{
			name:       "QTCategory",
			translator: format.NewQTCategoryTranslator(),
			cases: []translatorCase{
				//nolint:misspell // "Torsade de pointes" is a medical term, not a typo
				{10079780, "known risk", "Known risk for Torsade de pointes according to crediblemeds.org"},
				//nolint:misspell // "Torsade de pointes" is a medical term, not a typo
				{10079782, "conditional risk", "Conditional risk for Torsade de pointes according to crediblemeds.org"},
			},
		},
		{
			name:       "IntDirection",
			translator: format.NewIntDirectionTranslator(),
			cases: []translatorCase{
				{0, "undirected interaction", "Substances that mutually intensify each other's side effects."},
				{
					2, "bidirectional interaction",
					"Interacting substances both triggering the interaction and are both affected by their effect.",
				},
			},
		},
		{
			name:       "AdrFrequency",
			translator: format.NewAdrFrequencyTranslator(),
			cases: []translatorCase{
				{1, "Very common (>= 10%)", "Sehr häufig (>= 10%)"},
				{6, "Unknown", "Nicht bekannt"},
			},
		},
		{
			name:       "Product",
			translator: format.NewProductTranslator(),
			cases: []translatorCase{
				{1, "Drug", "Arzneimittel"},
				{8, "Other non-drug product", "Sonstiges Nicht-Arzneimittel"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, c := range tt.cases {
				code := c.code

				gotShort := tt.translator(&code, false)
				if gotShort == nil {
					t.Fatalf("code %d (detailed=false): got nil, want %q", code, c.wantShort)
				}
				if *gotShort != c.wantShort {
					t.Errorf("code %d (detailed=false): got %q, want %q", code, *gotShort, c.wantShort)
				}

				gotDetailed := tt.translator(&code, true)
				if gotDetailed == nil {
					t.Fatalf("code %d (detailed=true): got nil, want %q", code, c.wantDetailed)
				}
				if *gotDetailed != c.wantDetailed {
					t.Errorf("code %d (detailed=true): got %q, want %q", code, *gotDetailed, c.wantDetailed)
				}
			}
		})
	}
}

func TestTranslators_UnknownCode(t *testing.T) {
	t.Parallel()

	translators := map[string]func(*int, bool) *string{
		"IntPlausibility": format.NewIntPlausibilityTranslator(),
		"IntRelevance":    format.NewIntRelevanceTranslator(),
		"IntFrequency":    format.NewIntFrequencyTranslator(),
		"IntCredibility":  format.NewIntCredibilityTranslator(),
		"QTCategory":      format.NewQTCategoryTranslator(),
		"IntDirection":    format.NewIntDirectionTranslator(),
		"AdrFrequency":    format.NewAdrFrequencyTranslator(),
		"Product":         format.NewProductTranslator(),
	}

	for name, translator := range translators {
		translator := translator
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			unknown := intPtr(999999)
			if got := translator(unknown, false); got != nil {
				t.Errorf("unknown code (detailed=false): got %q, want nil", *got)
			}
			if got := translator(unknown, true); got != nil {
				t.Errorf("unknown code (detailed=true): got %q, want nil", *got)
			}
		})
	}
}

func TestTranslators_NilInput(t *testing.T) {
	t.Parallel()

	translators := map[string]func(*int, bool) *string{
		"IntPlausibility": format.NewIntPlausibilityTranslator(),
		"IntRelevance":    format.NewIntRelevanceTranslator(),
		"IntFrequency":    format.NewIntFrequencyTranslator(),
		"IntCredibility":  format.NewIntCredibilityTranslator(),
		"QTCategory":      format.NewQTCategoryTranslator(),
		"IntDirection":    format.NewIntDirectionTranslator(),
		"AdrFrequency":    format.NewAdrFrequencyTranslator(),
		"Product":         format.NewProductTranslator(),
	}

	for name, translator := range translators {
		translator := translator
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := translator(nil, false); got != nil {
				t.Errorf("nil input (detailed=false): got %q, want nil", *got)
			}
			if got := translator(nil, true); got != nil {
				t.Errorf("nil input (detailed=true): got %q, want nil", *got)
			}
		})
	}
}
