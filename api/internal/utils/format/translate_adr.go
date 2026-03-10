package format

func NewAdrFrequencyTranslator() func(*int, string) *string {
	adrFrequencyDescriptionEn := map[int]string{
		1: "Very common (>= 10%)",
		2: "Common (>= 1% to < 10%)",
		3: "Occasional (>= 0.1% to < 1%)",
		4: "Rare (>= 0.01% to < 0.1%)",
		5: "Very rare (< 0.01%)",
		6: "Unknown",
	}

	adrFrequencyDescriptionDe := map[int]string{
		1: "Sehr häufig (>= 10%)",
		2: "Häufig (>= 1% bis < 10%)",
		3: "Gelegentlich (>= 0.1% bis < 1%)",
		4: "Selten (>= 0.01% bis < 0.1%)",
		5: "Sehr selten (< 0.01%)",
		6: "Nicht bekannt",
	}

	return func(value *int, lang string) *string {
		if value == nil {
			return nil
		}

		translator := adrFrequencyDescriptionEn
		if lang == "german" || lang == "german-simple" {
			translator = adrFrequencyDescriptionDe
		}

		s, ok := translator[*value]
		if !ok {
			return nil
		}
		return &s
	}
}
