package format

func NewPlausibilityTranslator() func(*int, bool) *string {
	translator := map[int]string{
		10: "unknown mechanism",
		20: "plausible mechanism",
		30: "mechanism confirmed",
	}

	detailedTranslator := map[int]string{
		10: "Effects have only been observed, but there is no plausible mechanism.",
		20: "Observed interaction effects can be theoretically explained by compound properties.",
		30: "There exists an explainable and proven mechanism.",
	}

	return baseTranslatorFactory(translator, detailedTranslator)
}

func NewRelevanceTranslator() func(*int, bool) *string {
	translator := map[int]string{
		0:  "no statement possible",
		10: "no interaction expected",
		20: "product-specific warning",
		30: "minor",
		40: "moderate",
		50: "severe",
		60: "contraindicated",
	}

	detailedTranslator := map[int]string{
		0:  "No assessment from the literature available.",
		10: "Literature provides indications that no interaction occurs, or no interactions are expected based on PK/PD.",
		20: "Specific warnings from a pharmaceutical company, usually from the product information.",
		30: "Interaction does not necessarily have therapeutic consequences but should be monitored.",
		40: "Interaction can lead to therapeutically relevant consequences.",
		50: "Interaction can potentially be life-threatening or lead to serious, possibly irreversible consequences.",
		60: "The interacting agents must not be combined.",
	}

	return baseTranslatorFactory(translator, detailedTranslator)
}

func NewFrequencyTranslator() func(*int, bool) *string {
	translator := map[int]string{
		1: "very common",
		2: "common",
		3: "occasionally",
		4: "rare",
		5: "very rare",
		6: "not known",
	}

	detailedTranslator := map[int]string{
		1: "Frequency of DDI >=10%",
		2: "Frequency of DDI >=1% and <10%",
		3: "Frequency of DDI >=0.1% and <1%",
		4: "Frequency of DDI >=0.01% and <0.1%",
		5: "Frequency of DDI <0.001%",
		6: "Frequency of DDI is unknown.",
	}

	return baseTranslatorFactory(translator, detailedTranslator)
}

func NewCredibilityTranslator() func(*int, bool) *string {
	translator := map[int]string{
		10: "not known",
		20: "insufficient",
		30: "weak",
		40: "sufficient",
		50: "high",
	}

	detailedTranslator := map[int]string{
		10: "Evidence for interaction is not known from the evaluated literature.",
		20: "Evidence for interaction is insufficient from the evaluated literature.",
		30: "Evidence for interaction is weak from the evaluated literature.",
		40: "Evidence for interaction is sufficient from the evaluated literature.",
		50: "Evidence for interaction is high from the evaluated literature.",
	}

	return baseTranslatorFactory(translator, detailedTranslator)
}

func NewDirectionTranslator() func(*int, bool) *string {
	translator := map[int]string{
		0: "undirected interaction",
		1: "unidirectional interaction",
		2: "bidirectional interaction",
	}

	detailedTranslator := map[int]string{
		0: "Substances that mutually intensify each other's side effects.",
		1: "Perpetrator drug (right position) and a victim drug (left position).",
		2: "Interacting substances both triggering the interaction and are both affected by their effect.",
	}

	return baseTranslatorFactory(translator, detailedTranslator)
}

func baseTranslatorFactory(translator, detailed map[int]string) func(*int, bool) *string {

	tMap := map[bool](map[int]string){false: translator, true: detailed}

	return func(value *int, detailed bool) *string {
		if value == nil {
			return nil
		}

		s, ok := tMap[detailed][*value]
		if !ok {
			return nil
		}
		return &s
	}
}

func Description() any {
	type Desc struct {
		Code        []int    `json:"code"`
		Description []string `json:"description"`
		Detail      []string `json:"detail"`
	}

	getDescriptions := func(codes []int, translator func(*int, bool) *string) Desc {
		desc := Desc{Code: codes}
		for _, code := range codes {
			desc.Description = append(desc.Description, *translator(&code, false))
			desc.Detail = append(desc.Detail, *translator(&code, true))
		}
		return desc
	}

	return struct {
		Plausibility Desc `json:"plausibility"`
		Relevance    Desc `json:"relevance"`
		Frequency    Desc `json:"frequency"`
		Credibility  Desc `json:"credibility"`
		Direction    Desc `json:"direction"`
	}{
		Plausibility: getDescriptions([]int{10, 20, 30}, NewPlausibilityTranslator()),
		Relevance:    getDescriptions([]int{0, 10, 20, 30, 40, 50, 60}, NewRelevanceTranslator()),
		Frequency:    getDescriptions([]int{1, 2, 3, 4, 5, 6}, NewFrequencyTranslator()),
		Credibility:  getDescriptions([]int{10, 20, 30, 40, 50}, NewCredibilityTranslator()),
		Direction:    getDescriptions([]int{0, 1, 2}, NewDirectionTranslator()),
	}
}
