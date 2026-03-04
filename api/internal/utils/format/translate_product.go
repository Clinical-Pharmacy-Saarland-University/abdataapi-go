package format

func NewProductTranslator() func(*int, bool) *string {
	productTypeDescriptionEn := map[int]string{
		1: "Drug",
		2: "Medical device",
		3: "Dietary supplement",
		4: "Formulation base material",
		5: "Food supplement",
		6: "Body care product",
		7: "Disinfectant",
		8: "Other non-drug product",
	}

	productTypeDescriptionDe := map[int]string{
		1: "Arzneimittel",
		2: "Medizinprodukt",
		3: "Diätetikum",
		4: "Rezepturgrundstoff",
		5: "Nahrungsergänzungsmittel",
		6: "Körperpflegemittel",
		7: "Desinfektionsmittel",
		8: "Sonstiges Nicht-Arzneimittel",
	}

	return baseTranslatorFactory(productTypeDescriptionEn, productTypeDescriptionDe)
}
