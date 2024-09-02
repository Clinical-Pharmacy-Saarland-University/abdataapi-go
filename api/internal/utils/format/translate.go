package format

func NewPlausibilityTranslator() func(dbVal *int) *string {
	translator := map[int]string{
		10: "unknown mechanism",
		20: "plausible mechanism",
		30: "mechanism confirmed",
	}

	return baseTranslatorFactory(translator)
}

func NewRelevanceTranslator() func(value *int) *string {
	translator := map[int]string{
		0:  "no statement possible",
		10: "no interaction expected",
		20: "product-specific warning",
		30: "minor",
		40: "moderate",
		50: "severe",
		60: "contraindicated",
	}

	return baseTranslatorFactory(translator)
}

func NewFrequencyTranslator() func(value *int) *string {
	translator := map[int]string{
		1: "very common",
		2: "common",
		3: "occasionally",
		4: "rare",
		5: "very rare",
		6: "not known",
	}

	return baseTranslatorFactory(translator)
}

func NewCredibilityTranslator() func(value *int) *string {
	translator := map[int]string{
		10: "not known",
		20: "insufficient",
		30: "weak",
		40: "sufficient",
		50: "high",
	}

	return baseTranslatorFactory(translator)
}

func NewDirectionTranslator() func(value *int) *string {
	translator := map[int]string{
		0: "undirected interaction",
		1: "unidirectional interaction",
		2: "bidirectional interaction",
	}

	return baseTranslatorFactory(translator)
}

func baseTranslatorFactory(translator map[int]string) func(value *int) *string {
	return func(value *int) *string {
		if value == nil {
			return nil
		}

		s, ok := translator[*value]
		if !ok {
			return nil
		}
		return &s
	}
}
