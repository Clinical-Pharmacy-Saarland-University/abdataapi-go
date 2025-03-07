package compoundcontroller

import (
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"

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
	OtherPrescribingGuidanceAvailable bool   `json:"other_prescribing_guidance_available" db:"other_prescribing_guidance_available"`
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
// @Tags			compounds
// @Accept			json
// @Produce		json
// @Param			names	query	string	true	"Comma-separated compound names (e.g., Metoprolol,Aspirin)"
// @Success		200		{array}	object	"Successful response"
// @Failure		400		"Invalid request format or too many names provided"
// @Failure		500		"Internal server error"
// @Router			/compounds/names [get]
func (cc *CompoundController) GetSelectCompounds(c *gin.Context) {
	// Extract query parameters
	namesParam := c.Query("names")
	if namesParam == "" {
		handle.BadRequestError(c, "Missing required parameter: names")
		return
	}

	// Convert comma-separated values into a slice
	names := strings.Split(namesParam, ",")
	if len(names) > cc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", cc.Limits.BatchQueries))
		return
	}

	formattedResults := cc.FetchCompounds(c, names, cc.DB) // Corrected method call

	handle.Success(c, formattedResults)
}

// Corrected method signature and ensured regex-based matching remains
func (cc *CompoundController) FetchCompounds(c *gin.Context, names []string, db *sqlx.DB) []map[string]interface{} {
	formattedResults := []map[string]interface{}{}

	// Construct the MATCH query input
	matchQueryInput := "\"" + strings.Join(names, "\" \"") + "\""

	// Query to fetch matching compounds using FULLTEXT search
	compoundsQuery := `
		SELECT DISTINCT
			a.Name, 
			a.Herkunft, 
			a.Vorzugsbezeichnung, 
			a.Key_STO, 
			b.Name AS Input
		FROM SNA_DB a
		JOIN SNA_DB b ON a.Key_STO = b.Key_STO
		WHERE MATCH(b.Name) AGAINST (? IN BOOLEAN MODE)
	`

	var dbResults []struct {
		Name      string  `db:"Name"`
		Standard  *string `db:"Herkunft"`
		Preferred int     `db:"Vorzugsbezeichnung"`
		KeySTO    uint64  `db:"Key_STO"`
		Input     string  `db:"Input"`
	}

	err := db.Select(&dbResults, compoundsQuery, matchQueryInput)
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
// @Tags			pharmgkb
// @Accept			json
// @Produce		json
// @Param			names	query	string	true	"Comma-separated compound names (e.g., Metoprolol,Aspirin)"
// @Success		200		{array}	object	"Successful response"
// @Failure		400		"Invalid request format or too many names provided"
// @Failure		500		"Internal server error"
// @Router			/compounds/guidelines [get]
func (cc *CompoundController) GetCompoundGuidelines(c *gin.Context) {
	// Extract query parameters
	namesParam := c.Query("names")
	if namesParam == "" {
		handle.BadRequestError(c, "Missing required parameter: names")
		return
	}

	// Convert comma-separated values into a slice
	names := strings.Split(namesParam, ",")
	if len(names) > cc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", cc.Limits.BatchQueries))
		return
	}

	// Fetch compounds first
	formattedResults := cc.FetchCompounds(c, names, cc.DB)

	// Extract unique compound names from `formattedResults`
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

	// If there are no compounds found, return empty results
	if len(uniqueNames) == 0 {
		handle.Success(c, []map[string]interface{}{})
		return
	}

	// Query `guideline_table` for guidelines related to the extracted compound names
	queryBuilder := squirrel.Select(
		"drug",
		"drug_id",
		"drug_type",
		"guideline_type",
		"guideline_id",
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
		"related_gene_id",
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

	// Organize guidelines by drug name while ensuring uniqueness
	guidelinesByDrug := make(map[string]map[string]Guideline) // drug -> guidelineID -> Guideline struct
	for _, guideline := range dbResults {
		drugKey := strings.ToLower(guideline.Drug) // Normalize drug name to lowercase
		if _, exists := guidelinesByDrug[drugKey]; !exists {
			guidelinesByDrug[drugKey] = make(map[string]Guideline)
		}
		guidelinesByDrug[drugKey][guideline.GuidelineID] = guideline // Ensures uniqueness by ID
	}

	// Replace `matches` with unique `guidelines` in the `formattedResults`
	for i := range formattedResults {
		uniqueGuidelines := make([]Guideline, 0)
		if matches, ok := formattedResults[i]["matches"].([][]CompoundResponse); ok {
			for _, group := range matches {
				for _, compound := range group {
					drugKey := strings.ToLower(compound.Name)
					if guidelines, exists := guidelinesByDrug[drugKey]; exists {
						for _, guideline := range guidelines {
							uniqueGuidelines = append(uniqueGuidelines, guideline)
						}
					}
				}
			}
		}
		// Replace `matches` with unique `guidelines`
		formattedResults[i]["guidelines"] = uniqueGuidelines
		delete(formattedResults[i], "matches")
	}

	// Return the enriched results
	handle.Success(c, formattedResults)
}
