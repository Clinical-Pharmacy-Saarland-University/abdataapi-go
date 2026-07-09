package compoundcontroller

import (
	"fmt"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"

	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"regexp"
	"strings"
)

type CompoundController struct {
	DB     *sqlx.DB
	Limits cfg.LimitsConfig
}

func NewCompoundController(resourceHandle *handle.ResourceHandle) *CompoundController {
	return &CompoundController{
		DB:     resourceHandle.SQLX,
		Limits: resourceHandle.Limits,
	}
}

type CompoundResponse struct {
	Name      string   `json:"name"`
	Standard  []string `json:"standards"`
	Preferred bool     `json:"preferred"`
}

type PGKBResponse struct {
	Name      string   `json:"name"`
	Guideline []string `json:"guidelines"`
}

type Guideline struct {
	Drug                              string `json:"drug" db:"drug"`
	DrugID                            string `json:"drug_id" db:"drug_id"`
	DrugType                          string `json:"drug_type" db:"drug_type"`
	GuidelineType                     string `json:"guideline_type" db:"guideline_type"`
	GuidelineID                       string `json:"guideline_id" db:"guideline_id"`
	GuidelineName                     string `json:"guideline_name" db:"guideline_name"`
	GuidelineIssuer                   string `json:"guideline_issuer" db:"guideline_issuer"`
	AlternateDrugAvailable            bool   `json:"alternate_drug_available" db:"alternate_drug_available"`
	CancerGenome                      bool   `json:"cancer_genome" db:"cancer_genome"`
	DosingInformationAvailable        bool   `json:"dosing_information_available" db:"dosing_information_available"`
	TestingInformationAvailable       bool   `json:"testing_information_available" db:"testing_info_available"`
	RecommendationAvailable           bool   `json:"recommendation_available" db:"recommendation_available"`
	OtherPrescribingGuidanceAvailable bool   `json:"other_prescribing_guidance_available" db:"other_prescribing_guidance_available"` //nolint:lll // struct tag defines the JSON/DB field mapping and cannot be wrapped.
	Pediatric                         bool   `json:"pediatric" db:"pediatric"`
	RelatedGeneType                   string `json:"related_gene_type" db:"related_gene_type"`
	RelatedGeneSymbol                 string `json:"related_gene_symbol" db:"related_gene_symbol"`
	RelatedGeneID                     string `json:"related_gene_id" db:"related_gene_id"`
	RelatedGeneName                   string `json:"related_gene_name" db:"related_gene_name"`
}

// @name	CompoundSelectionQuery
type CompoundSelectionQuery struct {
	Names []string `json:"names" binding:"required" example:"Metoprolol,Aspirin"`
}

// @Summary		Get compounds by name
// @Description	Retrieves compounds by name and includes all related compounds sharing the same identifier.
// @Description
// @Description	The `names` parameter accepts the list in two formats:
// @Description	1. **Repeated parameter (preferred):** `?names=Metoprolol&names=Mirtazapin-0,5-Wasser`.
// @Description	Each occurrence is treated as one name verbatim, so names that contain a comma are preserved.
// @Description	2. **Single comma-joined value (legacy):** `?names=Metoprolol,Aspirin`, split on commas.
// @Description	This form is still supported but cannot represent names that contain a comma.
// @Description
// @Description	A value is only split when the parameter is supplied **once**. Because a single-name lookup is
// @Description	valid here, a lone name containing a comma sent as one value is **silently split** and returns
// @Description	HTTP 200 with empty/incorrect matches (no error). Send such a name via the repeated form together
// @Description	with at least one other value so the comma is preserved.
// @Tags			compounds
// @Accept			json
// @Produce		json
// @Param			names	query	[]string	true	"Compound names, as repeated parameters (preferred) or a single comma-joined value"	collectionFormat(multi)	example:"Metoprolol,Aspirin"
// @Success		200		{array}	object		"Successful response"
// @Failure		400		"Invalid request format or too many names provided"
// @Failure		500		"Internal server error"
// @Router			/compounds/names [get]
func (cc *CompoundController) GetSelectCompounds(c *gin.Context) {
	// Accept the list as repeated `names` parameters (verbatim) or a single
	// comma-joined value (legacy), so names containing commas survive intact.
	names := handle.QueryList(c, "names")
	if handle.IsEmptyList(names) {
		handle.BadRequestError(c, "Missing required parameter: names")
		return
	}

	if len(names) > cc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", cc.Limits.BatchQueries))
		return
	}

	formattedResults := cc.FetchCompounds(c, names)

	handle.Success(c, formattedResults)
}

// FetchCompounds resolves the given names to compounds using regex-based matching.
func (cc *CompoundController) FetchCompounds(c *gin.Context, names []string) []map[string]interface{} {
	formattedResults := []map[string]interface{}{}

	// Construct the SQL query for exact matches
	queryBuilder := squirrel.Select(
		"DISTINCT a.Name",
		"a.Herkunft",
		"a.Vorzugsbezeichnung",
		"a.Key_STO",
		"b.Name AS Input").
		From("SNA_DB a").
		Join("SNA_DB b ON a.Key_STO = b.Key_STO").
		Where(squirrel.Eq{"b.Name": names}) // Exact match (case-insensitive due to collation)

	query, args, _ := queryBuilder.ToSql()

	var dbResults []struct {
		Name      string  `db:"Name"`
		Standard  *string `db:"Herkunft"`
		Preferred int     `db:"Vorzugsbezeichnung"`
		KeySTO    uint64  `db:"Key_STO"`
		Input     string  `db:"Input"`
	}

	err := cc.DB.Select(&dbResults, query, args...)
	if err != nil {
		handle.Error(c, fmt.Errorf("database query for compounds failed: %w", err))
		return nil
	}

	// Organize results by input name
	matchesByInput := make(map[string]map[uint64]map[string]CompoundResponse)
	for _, name := range names {
		matchesByInput[name] = make(map[uint64]map[string]CompoundResponse)
	}

	for _, row := range dbResults {
		std := []string{}
		if row.Standard != nil {
			std = strings.Split(*row.Standard, ";")
		}

		compoundEntry := CompoundResponse{
			Name:      row.Name,
			Standard:  std,
			Preferred: row.Preferred == 1,
		}

		// Use regex to match input names against the returned Input column
		for _, name := range names {
			matched, _ := regexp.MatchString("(?i)"+regexp.QuoteMeta(name), row.Input)
			if matched {
				if _, exists := matchesByInput[name]; !exists {
					matchesByInput[name] = make(map[uint64]map[string]CompoundResponse)
				}
				if _, exists := matchesByInput[name][row.KeySTO]; !exists {
					matchesByInput[name][row.KeySTO] = make(map[string]CompoundResponse)
				}
				matchesByInput[name][row.KeySTO][row.Name] = compoundEntry
			}
		}
	}

	// Format final results with nested match arrays
	for _, name := range names {
		groupedMatches := [][]CompoundResponse{}
		for _, compoundsMap := range matchesByInput[name] {
			compounds := []CompoundResponse{}
			for _, compound := range compoundsMap {
				compounds = append(compounds, compound)
			}
			groupedMatches = append(groupedMatches, compounds)
		}
		formattedResults = append(formattedResults, map[string]interface{}{
			"input":   name,
			"matches": groupedMatches,
		})
	}

	return formattedResults
}

// @Summary		Get guidelines by drug name
// @Description	Retrieves guidelines by drug name and includes all related synonyms for the drug.
// @Description
// @Description	The `names` parameter accepts the list in two formats:
// @Description	1. **Repeated parameter (preferred):** `?names=Metoprolol&names=Mirtazapin-0,5-Wasser`.
// @Description	Each occurrence is treated as one name verbatim, so names that contain a comma are preserved.
// @Description	2. **Single comma-joined value (legacy):** `?names=Metoprolol,Aspirin`, split on commas.
// @Description	This form is still supported but cannot represent names that contain a comma.
// @Description
// @Description	A value is only split when the parameter is supplied **once**. Because a single-name lookup is
// @Description	valid here, a lone name containing a comma sent as one value is **silently split** and returns
// @Description	HTTP 200 with empty/incorrect results (no error). Send such a name via the repeated form together
// @Description	with at least one other value so the comma is preserved.
// @Tags			pharmgkb
// @Accept			json
// @Produce		json
// @Param			names	query	[]string	true	"Compound names, as repeated parameters (preferred) or a single comma-joined value"	collectionFormat(multi)	example:"Metoprolol,Aspirin"
// @Success		200		{array}	object		"Successful response"
// @Failure		400		"Invalid request format or too many names provided"
// @Failure		500		"Internal server error"
// @Router			/compounds/guidelines [get]
func (cc *CompoundController) GetCompoundGuidelines(c *gin.Context) {
	// Accept the list as repeated `names` parameters (verbatim) or a single
	// comma-joined value (legacy), so names containing commas survive intact.
	names := handle.QueryList(c, "names")
	if handle.IsEmptyList(names) {
		handle.BadRequestError(c, "Missing required parameter: names")
		return
	}

	if len(names) > cc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", cc.Limits.BatchQueries))
		return
	}

	// Fetch compounds first
	formattedResults := cc.FetchCompounds(c, names)

	// Extract unique compound names from `formattedResults`
	uniqueNames := extractUniqueCompoundNames(formattedResults)

	// If there are no compounds found, return empty results
	if len(uniqueNames) == 0 {
		handle.Success(c, []map[string]interface{}{})
		return
	}

	// Query `guideline_table` for guidelines related to the extracted compound names
	queryBuilder := squirrel.Select(
		"drug",
		"guideline_id",
		"related_gene_id",
		"drug_id",
		"drug_type",
		"guideline_type",
		"guideline_name",
		"guideline_issuer",
		"alternate_drug_available",
		"cancer_genome",
		"dosing_information_available",
		"testing_info_available",
		"recommendation_available",
		"other_prescribing_guidance_available",
		"pediatric",
		"related_gene_type",
		"related_gene_symbol",
		"related_gene_name",
	).
		From("guideline_table").
		Where(squirrel.Eq{"drug": uniqueNames})

	query, args, _ := queryBuilder.ToSql()

	var dbResults []Guideline
	err := cc.DB.Select(&dbResults, query, args...)
	if err != nil {
		handle.Error(c, fmt.Errorf("error fetching guidelines: %w", err))
		return
	}

	// Organize guidelines by drug name while ensuring **strict uniqueness** using a `set-like` structure
	guidelinesByDrug := groupGuidelinesByDrug(dbResults)

	// Replace `matches` with **unique** `guidelines` in `formattedResults`
	attachGuidelinesToResults(formattedResults, guidelinesByDrug)

	// Return the enriched results
	handle.Success(c, formattedResults)
}

// extractUniqueCompoundNames collects the lowercased compound names from the
// resolved matches, deduplicating them so each drug is queried only once.
func extractUniqueCompoundNames(formattedResults []map[string]interface{}) []string {
	unpackedNames := make(map[string]bool) // Use a map to avoid duplicates
	for _, result := range formattedResults {
		if matches, ok := result["matches"].([][]CompoundResponse); ok {
			for _, group := range matches {
				for _, compound := range group {
					unpackedNames[strings.ToLower(compound.Name)] = true
				}
			}
		}
	}

	// Convert map keys (unique compound names) to slice
	uniqueNames := make([]string, 0, len(unpackedNames))
	for name := range unpackedNames {
		uniqueNames = append(uniqueNames, name)
	}

	return uniqueNames
}

// groupGuidelinesByDrug organizes guidelines by drug name while ensuring
// **strict uniqueness** using a `set-like` structure keyed by a composite key.
func groupGuidelinesByDrug(dbResults []Guideline) map[string]map[string]Guideline {
	guidelinesByDrug := make(map[string]map[string]Guideline) // drug (from DB) -> uniqueKey -> Guideline struct

	for _, guideline := range dbResults {
		// Use the drug name from the database.
		dbDrugName := strings.ToLower(guideline.Drug)
		// Unique composite key.
		uniqueKey := fmt.Sprintf("%s|%s|%s", dbDrugName, guideline.GuidelineID, guideline.RelatedGeneID)

		if _, exists := guidelinesByDrug[dbDrugName]; !exists {
			guidelinesByDrug[dbDrugName] = make(map[string]Guideline)
		}

		// **Ensure uniqueness by key** (if a duplicate exists, it won't be added)
		guidelinesByDrug[dbDrugName][uniqueKey] = guideline
	}

	return guidelinesByDrug
}

// attachGuidelinesToResults replaces the `matches` entry of each result with a
// **deduplicated** `guidelines` slice built from the grouped guidelines.
func attachGuidelinesToResults(
	formattedResults []map[string]interface{},
	guidelinesByDrug map[string]map[string]Guideline,
) {
	for i := range formattedResults {
		matches, _ := formattedResults[i]["matches"].([][]CompoundResponse)
		// Replace `matches` with **deduplicated** `guidelines`
		formattedResults[i]["guidelines"] = collectUniqueGuidelines(matches, guidelinesByDrug)
		delete(formattedResults[i], "matches")
	}
}

// collectUniqueGuidelines gathers the guidelines for the compounds contained in
// matches, **deduplicated** across all groups using the composite key.
func collectUniqueGuidelines(
	matches [][]CompoundResponse,
	guidelinesByDrug map[string]map[string]Guideline,
) []Guideline {
	uniqueGuidelines := make([]Guideline, 0)
	seenKeys := make(map[string]struct{}) // Track already added guidelines

	for _, group := range matches {
		for _, compound := range group {
			dbDrugName := strings.ToLower(compound.Name) // Use DB drug name
			guidelines, exists := guidelinesByDrug[dbDrugName]
			if !exists {
				continue
			}
			for uniqueKey, guideline := range guidelines {
				// Ensure **no duplicates in final output**
				if _, seen := seenKeys[uniqueKey]; !seen {
					seenKeys[uniqueKey] = struct{}{} // Mark as seen
					uniqueGuidelines = append(uniqueGuidelines, guideline)
				}
			}
		}
	}

	return uniqueGuidelines
}
