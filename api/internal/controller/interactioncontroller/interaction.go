package interactioncontroller

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/common"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/utils/apierr"
	"observeddb-go-api/internal/utils/format"
	"observeddb-go-api/internal/utils/helper"
	"observeddb-go-api/internal/utils/validate"

	"slices"
	"strings"
	"sync"

	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type InteractionController struct {
	DB                     *sqlx.DB
	Limits                 cfg.LimitsConfig
	PlausibilityTranslator func(*int, bool) *string
	RelevanceTranslator    func(*int, bool) *string
	FrequencyTranslator    func(*int, bool) *string
	CredibilityTranslator  func(*int, bool) *string
	DirectionTranslator    func(*int, bool) *string
	DescriptionStruct      any
}

type InteractionText struct {
	DataBasis           *string `json:"data_basis,omitempty"`
	PharmacologicEffect *string `json:"pharmacologic_effect,omitempty"`
	Mechanism           *string `json:"mechanism,omitempty"`
	Literature          *string `json:"literature,omitempty"`
} // @name InteractionText

func NewInteractionController(resourceHandle *handle.ResourceHandle) *InteractionController {
	return &InteractionController{
		DB:                     resourceHandle.SQLX,
		Limits:                 resourceHandle.Limits,
		PlausibilityTranslator: format.NewIntPlausibilityTranslator(),
		RelevanceTranslator:    format.NewIntRelevanceTranslator(),
		FrequencyTranslator:    format.NewIntFrequencyTranslator(),
		CredibilityTranslator:  format.NewIntCredibilityTranslator(),
		DirectionTranslator:    format.NewIntDirectionTranslator(),
		DescriptionStruct:      format.Description(),
	}
}

func (ic *InteractionController) GetInterDescription(c *gin.Context) {
	c.JSON(http.StatusOK, ic.DescriptionStruct)
}

// @Summary		Query drug-drug interactions between PZNs in batch
//
// @Description	This is the batch version of the `GET /interactions/pzns` endpoint.
//
// @Description	The result will be an array of drug-drug interactions between the provided PZNs.
// @Description	Each interaction will contain the plausibility, relevance, frequency, credibility,
// @Description	and direction of the interaction.
//
// @Description	The direction of the interaction describes the relationship between the victims (left)
// @Description	and the perpetrators (right).
//
// @Description	The left size and right side of the interaction can be more than one PZN if the same interaction
// @Description	is observed between multiple PZNs.
//
// @Description	If the `details` query parameter is set to `true`, the interaction descriptions will be more detailed.
// @Description	Ids for the queries are required to be unique.
//
// @Description	Queries will be processed in parallel.
// @Description	**It is possible that some/all queries will fail. This will result in error code `207`.**
// @Description	The response will contain the results of all queries, even if some of them failed.
// @Description	**The user is responsible for checking the status of each query in the batch.**
//
// @Tags			Drug-Drug Interactions
//
// @Produce		json
// @Param			request	body		[]PZNInteractionPostQuery						true	"Batch query for drug-drug interactions"
// @Success		200		{object}	handle.jsendSuccess[[]PZNBatchResult]			"Results with no errors"
// @Success		207		{object}	handle.jsendSuccess[[]PZNBatchResult]			"Results with errors"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Too many IDs or duplicate IDs"
//
// @Security		Bearer
//
// @Router			/interactions/pzns [post]
func (ic *InteractionController) PostInterPZNs(c *gin.Context) {
	type Query struct {
		ID           string   `json:"id" binding:"required" example:"1"`                 // ID of the query
		PZNs         []string `json:"pzns" binding:"required" example:"1234567,7654321"` // Array of PZNs
		DetailedDesc bool     `json:"details" binding:"omitempty" example:"true"`        // Detailed interaction descriptions
		FetchText    bool     `json:"text" binding:"omitempty" example:"true"`           // Fetch interaction text
	} //	@name	PZNInteractionPostQuery
	queries := []Query{}

	if !handle.JSONBind(c, &queries) {
		return
	}

	ids := make([]string, len(queries))
	for id := range queries {
		ids[id] = queries[id].ID
	}

	n := len(ids)
	if n > ic.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many IDs provided. Maximum is %d", ic.Limits.BatchQueries))
		return
	}

	if !helper.IsUnique(ids) {
		handle.BadRequestError(c, "Duplicate IDs provided")
		return
	}

	type BatchResult struct {
		ID string `json:"id" example:"1"` // ID of the query
		apierr.ResStatus
		Interactions *[]PZNInteraction `json:"interactions"` // Drug-drug interactions
	} //	@name	PZNBatchResult

	db := ic.DB
	maxConcurrency := ic.Limits.BatchJobs
	semaphore := make(chan struct{}, maxConcurrency)
	results := make([]BatchResult, n)
	var wg sync.WaitGroup

	for i, q := range queries {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int, query *Query) {
			defer wg.Done()
			defer func() {
				<-semaphore
			}()

			result, err := fetchPznInteractions(query.PZNs, db, ic, query.DetailedDesc, query.FetchText)
			results[idx] = BatchResult{q.ID, apierr.ToResponse(c, err), &result}
		}(i, &q)
	}
	wg.Wait()

	nSuccess := 0
	for _, result := range results {
		if result.Ok() {
			nSuccess++
		}
	}

	handle.SuccessWithStatus(c, apierr.BatchStatusCode(n, nSuccess), results)
}

// @Summary		Query drug-drug interactions between PZNs
// @Description	The result will be an array of drug-drug interactions between the provided PZNs.
// @Description	Each interaction will contain the plausibility, relevance, frequency, credibility,
// @Description	and direction of the interaction.
//
// @Description	The direction of the interaction describes the relationship between the victims (left)
// @Description	and the perpetrators (right).
//
// @Description	The left size and right side of the interaction can be more than one PZN if the same interaction
// @Description	is observed between multiple PZNs.
//
// @Description	If the `details` query parameter is set to `true`, the interaction descriptions will be more detailed.
//
// @Tags			Drug-Drug Interactions
//
// @Produce		json
// @Param			pzns	query		string											true	"Comma separated string of PZNs"			example:"1234567,7654321"
// @Param			details	query		boolean											false	"Fetch detailed interaction descriptions"	default:"false"
// @Param			text	query		boolean											false	"Fetch interaction text"					default:"false"
// @Success		200		{object}	handle.jsendSuccess[[]PZNInteraction]			"List of drug-drug interactions"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Invalid PZNs"
// @Failure		404		{object}	handle.jsendFailure[handle.errorResponse]		"PZN(s) not found"
//
// @Security		Bearer
//
// @Router			/interactions/pzns [get]
func (ic *InteractionController) GetInterPZNs(c *gin.Context) {
	type Query struct {
		PZNs         string `form:"pzns" binding:"required" example:"1234567,7654321"`
		DetailedDesc bool   `form:"details" binding:"omitempty" example:"true"`
		FetchText    bool   `form:"text" binding:"omitempty" example:"true"`
	} //	@name	PZNInteractionQuery

	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")

	result, err := fetchPznInteractions(pzns, ic.DB, ic, query.DetailedDesc, query.FetchText)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

// @Summary		Query drug-drug interactions between compounds in batch
// @Description	This is the batch version of the `GET /interactions/compounds` endpoint.
//
// @Description	The result will be an array of drug-drug interactions between the provided compounds.
// @Description	Each interaction will contain the plausibility, relevance, frequency, credibility,
// @Description	and direction of the interaction.
//
// @Description	The direction of the interaction describes the relationship between the victims (left)
// @Description	and the perpetrators (right).
//
// @Description	The left side and right side of the interaction can include multiple compounds if the same interaction
// @Description	is observed between multiple compounds. This may happen if the same compound is marketed under different names.
//
// @Description	If the `details` query parameter is set to `true`, the interaction descriptions will be more detailed.
// @Description	If the `doses` query parameter is set to `true`, the relevant doses/formulations of the compounds involved will be included.
//
// @Description	Ids for the queries must be unique.
// @Description	Queries will be processed in parallel.
// @Description	**Some/all queries may fail, resulting in error code `207`.**
// @Description	The response will contain results for all queries, even if some failed.
// @Description	**The user must check the status of each query in the batch.**
//
// @Tags			Drug-Drug Interactions
//
// @Produce		json
// @Param			request	body		[]CompoundInteractionPostQuery					true	"Batch query for drug-drug interactions"
// @Success		200		{object}	handle.jsendSuccess[[]CompoundBatchResult]		"Results with no errors"
// @Success		207		{object}	handle.jsendSuccess[[]CompoundBatchResult]		"Results with errors"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Too many IDs or duplicate IDs"
//
// @Security		Bearer
//
// @Router			/interactions/compounds [post]
func (ic *InteractionController) PostInterCompounds(c *gin.Context) {
	type Query struct {
		ID           string   `json:"id" binding:"required" example:"1"`                          // ID of the query
		Compounds    []string `json:"compounds" binding:"required" example:"Aspirin,Paracetamol"` // Array of compounds
		FetchDoses   bool     `json:"doses" binding:"omitempty" example:"true"`                   // Fetch dose/formulation information
		DetailedDesc bool     `json:"details" binding:"omitempty" example:"true"`                 // Detailed interaction descriptions
		FetchText    bool     `json:"text" binding:"omitempty" example:"true"`                    // Fetch interaction text
	} //	@name	CompoundInteractionPostQuery
	queries := []Query{}

	if !handle.JSONBind(c, &queries) {
		return
	}

	ids := make([]string, len(queries))
	for i := range queries {
		ids[i] = queries[i].ID
	}

	n := len(ids)
	if n > ic.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many IDs provided. Maximum is %d", ic.Limits.BatchQueries))
		return
	}

	if !helper.IsUnique(ids) {
		handle.BadRequestError(c, "Duplicate IDs provided")
		return
	}

	type BatchResult struct {
		ID string `json:"id" example:"1"` // ID of the query
		apierr.ResStatus
		Interactions *[]CompoundInteraction `json:"interactions"` // Drug-drug interactions
	} //	@name	CompoundBatchResult

	db := ic.DB
	maxConcurrency := ic.Limits.BatchJobs
	semaphore := make(chan struct{}, maxConcurrency)
	results := make([]BatchResult, n)
	var wg sync.WaitGroup

	for i, q := range queries {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int, query *Query) {
			defer wg.Done()
			defer func() { <-semaphore }()

			result, err := fetchCompoundInteractions(query.Compounds, db, ic, query.FetchDoses, query.DetailedDesc, query.FetchText)
			results[idx] = BatchResult{query.ID, apierr.ToResponse(c, err), &result}
		}(i, &q)
	}
	wg.Wait()

	nSuccess := 0
	for _, result := range results {
		if result.Ok() {
			nSuccess++
		}
	}

	handle.SuccessWithStatus(c, apierr.BatchStatusCode(n, nSuccess), results)
}

// @Summary		Query drug-drug interactions between compounds
// @Description	The result will be an array of drug-drug interactions between the provided compounds.
// @Description	Each interaction will contain the plausibility, relevance, frequency, credibility,
// @Description	and direction of the interaction.
//
// @Description	The direction of the interaction describes the relationship between the victims (left)
// @Description	and the perpetrators (right).
//
// @Description	The left size and right side of the interaction can be more than one compounds if the same interaction
// @Description	is observed between multiple compounds. This can be the case if the same compound is marketed
// @Description	under different names or derivates are considered.
//
// @Description	If the `details` query parameter is set to `true`, the interaction descriptions will be more detailed.
//
// @Description	If the `doses` query parameter is set to `true`, the interaction will contain the relevant
// @Description	doses/formulations of the compounds that are involved in the interaction.
//
// @Tags			Drug-Drug Interactions
// @Produce		json
// @Param			pzns	query		string											true	"Comma separated string of compounds"		example:"Aspirin,Paracetamol"
// @Param			doses	query		boolean											false	"Fetch doses"								default:"false"
// @Param			details	query		boolean											false	"Fetch detailed interaction descriptions"	default:"false"
// @Param			text	query		boolean											false	"Fetch interaction text"					default:"false"
// @Success		200		{object}	handle.jsendSuccess[[]CompoundInteraction]		"List of drug-drug interactions"
// @Failure		422		{object}	handle.jsendFailure[handle.validationResponse]	"Bad query format"
// @Failure		500		{object}	handle.jSendError								"Internal server error"
// @Failure		401		{object}	handle.jsendFailure[handle.errorResponse]		"Unauthorized"
// @Failure		400		{object}	handle.jsendFailure[handle.errorResponse]		"Invalid compound names"
// @Failure		404		{object}	handle.jsendFailure[handle.errorResponse]		"Compound(s) not found"
//
// @Security		Bearer
//
// @Router			/interactions/compounds [get]
func (ic *InteractionController) GetInterCompounds(c *gin.Context) {
	type Query struct {
		Compounds    string `form:"compounds" binding:"required" example:"Aspirin,Paracetamol"`
		FetchDose    bool   `form:"doses" binding:"omitempty" example:"true"`
		DetailedDesc bool   `form:"details" binding:"omitempty" example:"true"`
		FetchText    bool   `form:"text" binding:"omitempty" example:"true"`
	} //	@name	CompoundInteractionQuery

	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}

	compounds := strings.Split(query.Compounds, ",")

	result, err := fetchCompoundInteractions(compounds, ic.DB, ic, query.FetchDose, query.DetailedDesc, query.FetchText)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

type CompoundInteraction struct {
	Plausibility *string          `json:"plausibility" example:"plausible mechanism"` // Plausibility of the interaction
	Relevance    *string          `json:"relevance" example:"minor"`                  // Relevance of the interaction
	Frequency    *string          `json:"frequency" example:"common"`                 // Frequency of the interaction
	Credibility  *string          `json:"credibility" example:"insufficient"`         // Credibility of the interaction
	Direction    *string          `json:"direction" example:"undirected interaction"` // Direction of the interaction
	CompoundsL   []string         `json:"compounds_left" example:"Aspirin"`           // Victim compound(s)
	CompoundsR   []string         `json:"compounds_right" example:"Paracetamol"`      // Perpetrator compound(s)
	DosesL       []*CompoundDose  `json:"doses_left"`                                 // Doses of the victim compounds
	DosesR       []*CompoundDose  `json:"doses_right"`                                // Doses of the perpetrator compounds
	Text         *InteractionText `json:"text,omitempty"`                             // Extracted interaction text
} //	@name	CompoundInteraction

type compoundDBInteraction struct {
	KeyINT       uint64  `db:"Key_INT"`
	TextRef      *uint64 `db:"Textverweis"`
	Plausibility *int    `db:"Plausibilitaet"`
	Relevance    *int    `db:"Relevanz"`
	Frequency    *int    `db:"Haeufigkeit"`
	Credibility  *int    `db:"Quellenbewertung"`
	Direction    *int    `db:"Richtung"`
	KeyStoL      uint64  `db:"Key_STO_L"`
	KeyStoR      uint64  `db:"Key_STO_R"`
}

func uniqueInteractions[T any](interactions []T) []T {
	seen := make(map[string]struct{})
	var unique []T

	for _, interaction := range interactions {
		serialized, err := json.Marshal(interaction)
		if err != nil {
			continue
		}
		key := string(serialized)

		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			unique = append(unique, interaction)
		}
	}
	return unique
}

func fetchCompoundInteractions( //nolint:gocognit // splitting up this function would make it less readable
	compounds []string,
	db *sqlx.DB,
	ic *InteractionController,
	fetchDoses bool,
	detailedDesc bool,
	fetchText bool,
) ([]CompoundInteraction, error) {
	if err := validate.Compounds(compounds, ic.Limits.InteractionDrugs); err != nil {
		return nil, apierr.New(http.StatusBadRequest, err.Error())
	}

	stoCompoundMap, err := common.StoToCompoundsMap(db, compounds)
	if err != nil {
		return nil, apierr.New(http.StatusInternalServerError, err.Error())
	}

	// check if all compounds are in the database
	var dbCompounds []string
	for bucket := range maps.Values(stoCompoundMap) {
		dbCompounds = append(dbCompounds, bucket...)
	}
	if diff := helper.SetDifference(compounds, dbCompounds); len(diff) > 0 {
		return nil, apierr.New(http.StatusNotFound, fmt.Sprintf("Compounds not found: %s", strings.Join(diff, ", ")))
	}

	keySto := slices.Collect(maps.Keys(stoCompoundMap))
	queryBuilder := squirrel.Select(
		"INT_C.Key_INT",
		"INT_C.Textverweis",
		"INT_C.Plausibilitaet",
		"INT_C.Relevanz",
		"INT_C.Haeufigkeit",
		"INT_C.Quellenbewertung",
		"INT_C.Richtung",
		"SZI_C1.Key_STO AS Key_STO_R",
		"SZI_C2.Key_STO AS Key_STO_L").
		From("SZI_C AS SZI_C1").
		Join("SZI_C AS SZI_C2 ON SZI_C1.Key_INT = SZI_C2.Key_INT").
		Where("INT_C.AMTS_individuell <> 0").
		Where(squirrel.Eq{"SZI_C1.Key_STO": keySto}).
		Where(squirrel.Eq{"SZI_C2.Key_STO": keySto}).
		Where("SZI_C1.Key_STO <> SZI_C2.Key_STO").
		Where("SZI_C1.Lokalisation = 'R'").
		Where("SZI_C2.Lokalisation = 'L'").
		LeftJoin("INT_C ON SZI_C1.Key_INT = INT_C.Key_INT").
		OrderBy("INT_C.Key_INT")

	query, args, _ := queryBuilder.ToSql()
	var dbInteractions []compoundDBInteraction

	err = db.Select(&dbInteractions, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching interactions: %w", err)
	}

	var results = make([]CompoundInteraction, len(dbInteractions))
	for i, interaction := range dbInteractions {
		results[i] = CompoundInteraction{
			Plausibility: ic.PlausibilityTranslator(interaction.Plausibility, detailedDesc),
			Relevance:    ic.RelevanceTranslator(interaction.Relevance, detailedDesc),
			Frequency:    ic.FrequencyTranslator(interaction.Frequency, detailedDesc),
			Credibility:  ic.CredibilityTranslator(interaction.Credibility, detailedDesc),
			Direction:    ic.DirectionTranslator(interaction.Direction, detailedDesc),
			CompoundsL:   stoCompoundMap[interaction.KeyStoL],
			CompoundsR:   stoCompoundMap[interaction.KeyStoR],
		}
	}

	if fetchText {
		textByInt, err := fetchInteractionTexts(db, dbInteractions)
		if err != nil {
			return nil, fmt.Errorf("error fetching interaction text: %w", err)
		}
		for i, interaction := range dbInteractions {
			if text, ok := textByInt[interaction.KeyINT]; ok {
				results[i].Text = text
			}
		}
	}

	if fetchDoses { //nolint:nestif // refactoring this is a mess
		keyINT := []uint64{}
		for _, interaction := range dbInteractions {
			keyINT = append(keyINT, interaction.KeyINT)
		}

		compoundDoses, errf := fetchCompoundDoses(db, keyINT, keySto)
		if errf != nil {
			return nil, fmt.Errorf("error fetching compound doses: %w", errf)
		}

		// interactions as well as doses are ordered by Key_INT
		for i, interaction := range dbInteractions {
			for _, dose := range compoundDoses {
				if dose.KeyINT == interaction.KeyINT {
					if dose.KeySTO == interaction.KeyStoL {
						results[i].DosesL = append(results[i].DosesL, &dose)
					} else if dose.KeySTO == interaction.KeyStoR {
						results[i].DosesR = append(results[i].DosesR, &dose)
					}
				}
			}
		}
	}

	results = uniqueInteractions(results)
	return results, nil
}

type PZNInteraction struct {
	KeyINT       uint64           `json:"-"`
	Plausibility *string          `json:"plausibility" example:"plausible mechanism"` // Plausibility of the interaction
	Relevance    *string          `json:"relevance" example:"minor"`                  // Relevance of the interaction
	Frequency    *string          `json:"frequency" example:"common"`                 // Frequency of the interaction
	Credibility  *string          `json:"credibility" example:"insufficient"`         // Credibility of the interaction
	Direction    *string          `json:"direction" example:"undirected interaction"` // Direction of the interaction
	PZNL         []string         `json:"pzn_left" example:"1234567"`                 // Victim PZN
	PZNR         []string         `json:"pzn_right" example:"7654321"`                // Perpetrator PZN
	Text         *InteractionText `json:"text,omitempty"`                             // Extracted interaction text
} //	@name	PZNInteraction

func fetchPznInteractions(
	pzns []string,
	db *sqlx.DB,
	ic *InteractionController,
	detailedDesc bool,
	fetchText bool,
) ([]PZNInteraction, error) {
	if err := validate.PZNs(pzns, 2, ic.Limits.InteractionDrugs); err != nil {
		return nil, apierr.New(http.StatusBadRequest, err.Error())
	}

	famPznMap, err := common.FamToPZN(db, pzns)
	if err != nil {
		return nil, apierr.New(http.StatusInternalServerError, err.Error())
	}

	var foundPzns []string
	for pzn := range maps.Values(famPznMap) {
		foundPzns = append(foundPzns, pzn...)
	}

	if diff := helper.SetDifference(pzns, foundPzns); len(diff) > 0 {
		return nil, apierr.New(http.StatusNotFound, fmt.Sprintf("PZNs not found: %s", strings.Join(diff, ", ")))
	}

	fams := slices.Collect(maps.Keys(famPznMap))
	queryBuilder := squirrel.Select(
		"INT_C.Plausibilitaet",
		"INT_C.Relevanz",
		"INT_C.Haeufigkeit",
		"INT_C.Quellenbewertung",
		"INT_C.Richtung",
		"INT_C.Key_INT",
		"INT_C.Textverweis",
		"FZI_C1.Key_FAM AS Key_FAM_R",
		"FZI_C2.Key_FAM AS Key_FAM_L",
		"SZI_C1.Key_STO AS Key_STO_R",
		"SZI_C2.Key_STO AS Key_STO_L").
		From("FZI_C AS FZI_C1").
		LeftJoin("SZI_C AS SZI_C1 ON FZI_C1.Key_INT = SZI_C1.Key_INT AND FZI_C1.Key_STO = SZI_C1.Key_STO").
		Join("FZI_C AS FZI_C2 ON FZI_C1.Key_INT = FZI_C2.Key_INT").
		LeftJoin("SZI_C AS SZI_C2 ON FZI_C2.Key_INT = SZI_C2.Key_INT AND FZI_C2.Key_STO = SZI_C2.Key_STO").
		Where("INT_C.AMTS_individuell <> 0").
		Where(squirrel.Eq{"FZI_C1.Key_FAM": fams}).
		Where(squirrel.Eq{"FZI_C2.Key_FAM": fams}).
		Where("FZI_C1.Key_FAM <> FZI_C2.Key_FAM").
		Where("SZI_C1.Lokalisation = 'R'").
		Where("SZI_C2.Lokalisation = 'L'").
		LeftJoin("INT_C ON FZI_C1.Key_INT = INT_C.Key_INT")

	query, args, _ := queryBuilder.ToSql()
	var dbInteractions []dbInteraction
	err = db.Select(&dbInteractions, query, args...) //nolint:musttag // need untaged fields
	if err != nil {
		return nil, fmt.Errorf("error fetching interactions: %w", err)
	}

	results := mapCompoundInteracions(dbInteractions, famPznMap, ic, detailedDesc)

	if fetchText {
		textByInt, err := fetchInteractionTexts(db, dbInteractions)
		if err != nil {
			return nil, fmt.Errorf("error fetching interaction text: %w", err)
		}
		for i := range results {
			if text, ok := textByInt[results[i].KeyINT]; ok {
				results[i].Text = text
			}
		}
	}

	results = uniqueInteractions(results)
	return results, nil
}

type dbInteraction struct {
	KeyINT        uint64  `db:"Key_INT"`
	TextRef       *uint64 `db:"Textverweis"`
	Plausibility  *int    `db:"Plausibilitaet"`
	Relevance     *int    `db:"Relevanz"`
	Frequency     *int    `db:"Haeufigkeit"`
	Credibility   *int    `db:"Quellenbewertung"`
	Direction     *int    `db:"Richtung"`
	KeyFAML       uint64  `db:"Key_FAM_L"`
	KeyFAMR       uint64  `db:"Key_FAM_R"`
	KeyStoL       uint64  `db:"Key_STO_L"`
	KeyStoR       uint64  `db:"Key_STO_R"`
	KeyFAMLBucket []uint64
	KeyFAMRBucket []uint64
}

func mapCompoundInteracions(
	interactionTable []dbInteraction,
	pznFamMap map[uint64][]string,
	ic *InteractionController,
	detailedDesc bool,
) []PZNInteraction {
	type StoPair struct {
		KeyStoL uint64
		KeyStoR uint64
	}

	stoMap := make(map[StoPair][]*dbInteraction)
	for _, interaction := range interactionTable {
		pair := StoPair{interaction.KeyStoL, interaction.KeyStoR}
		stoMap[pair] = append(stoMap[pair], &interaction)
	}

	var curated []*dbInteraction
	for _, interactions := range stoMap {
		base := interactions[0]
		for _, tuples := range interactions {
			base.KeyFAMLBucket = append(base.KeyFAMLBucket, tuples.KeyFAML)
			base.KeyFAMRBucket = append(base.KeyFAMRBucket, tuples.KeyFAMR)
		}
		base.KeyFAMLBucket = helper.Unique(base.KeyFAMLBucket)
		base.KeyFAMRBucket = helper.Unique(base.KeyFAMRBucket)
		curated = append(curated, base)
	}

	var results = make([]PZNInteraction, len(curated))
	for i, interaction := range curated {
		results[i] = PZNInteraction{
			KeyINT:       interaction.KeyINT,
			Plausibility: ic.PlausibilityTranslator(interaction.Plausibility, detailedDesc),
			Relevance:    ic.RelevanceTranslator(interaction.Relevance, detailedDesc),
			Frequency:    ic.FrequencyTranslator(interaction.Frequency, detailedDesc),
			Credibility:  ic.CredibilityTranslator(interaction.Credibility, detailedDesc),
			Direction:    ic.DirectionTranslator(interaction.Direction, detailedDesc),
		}
		for _, keyFam := range interaction.KeyFAMLBucket {
			results[i].PZNL = append(results[i].PZNL, pznFamMap[keyFam]...)
		}
		for _, keyFam := range interaction.KeyFAMRBucket {
			results[i].PZNR = append(results[i].PZNR, pznFamMap[keyFam]...)
		}
	}

	return results
}

type CompoundDose struct {
	KeySTO          uint64   `db:"Key_STO" json:"-"`
	KeyINT          uint64   `db:"Key_INT" json:"-"`
	Value           *float64 `db:"Zahl" json:"value" example:"500"`
	Unit            *string  `db:"Einheit" json:"unit" example:"mg"`
	Suffix          *string  `db:"Suffix" json:"suffix" example:"(retard)"`
	DosageForm      *string  `db:"Key_DAR" json:"dosage_form" example:"TAB"`
	ActiveSubstance bool     `db:"ES" json:"active_substance" example:"true"`
} //	@name	CompoundDose

func fetchCompoundDoses(db *sqlx.DB, keyInt, keySto []uint64) ([]CompoundDose, error) {
	queryBuilder := squirrel.Select(
		"FAI_DB.Key_STO",
		"FZI_C.Key_INT",
		"Zahl",
		"Einheit",
		"Suffix",
		"Key_DAR",
		"Entsprichtstoff IS NOT NULL AS ES").
		From("FZI_C").
		Distinct().
		LeftJoin("FAI_DB ON FAI_DB.Key_FAM = FZI_C.Key_FAM").
		LeftJoin("FAM_DB ON FAI_DB.Key_FAM = FAM_DB.Key_FAM").
		Where(squirrel.And{
			squirrel.Eq{"FZI_C.Key_INT": keyInt},
			squirrel.Eq{"FAI_DB.Key_STO": keySto},
			squirrel.Eq{"FAI_DB.Stofftyp": 1},
		}).
		OrderBy("FZI_C.Key_INT")

	query, args, _ := queryBuilder.ToSql()
	var compoundDoses []CompoundDose
	err := db.Select(&compoundDoses, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching dose compound interactions: %w", err)
	}

	return compoundDoses, nil
}

func fetchInteractionTexts[T interface {
	getKeyINT() uint64
	getTextRef() *uint64
}](db *sqlx.DB, interactions []T) (map[uint64]*InteractionText, error) {
	textRefs := make([]uint64, 0, len(interactions))
	intToTextRef := make(map[uint64]uint64, len(interactions))
	seenTextRefs := make(map[uint64]struct{}, len(interactions))
	for _, interaction := range interactions {
		if interaction.getTextRef() == nil {
			continue
		}
		textRef := *interaction.getTextRef()
		intToTextRef[interaction.getKeyINT()] = textRef
		if _, exists := seenTextRefs[textRef]; exists {
			continue
		}
		seenTextRefs[textRef] = struct{}{}
		textRefs = append(textRefs, textRef)
	}

	textByRef := make(map[uint64]*InteractionText, len(textRefs))
	if len(textRefs) == 0 {
		return map[uint64]*InteractionText{}, nil
	}

	queryBuilder := squirrel.Select("Textverweis", "Textfeld", "Text").
		From("ITX_C").
		Where(squirrel.Eq{"Textverweis": textRefs}).
		Where(squirrel.Eq{"Textfeld": []int{340, 40, 140, 9}})

	query, args, _ := queryBuilder.ToSql()
	var rows []struct {
		TextRef   uint64 `db:"Textverweis"`
		TextField int    `db:"Textfeld"`
		Text      string `db:"Text"`
	}
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, err
	}

	for _, row := range rows {
		if _, exists := textByRef[row.TextRef]; !exists {
			textByRef[row.TextRef] = &InteractionText{}
		}
		switch row.TextField {
		case 340:
			textByRef[row.TextRef].DataBasis = &row.Text
		case 40:
			textByRef[row.TextRef].PharmacologicEffect = &row.Text
		case 140:
			textByRef[row.TextRef].Mechanism = &row.Text
		case 9:
			textByRef[row.TextRef].Literature = &row.Text
		}
	}

	textByInt := make(map[uint64]*InteractionText, len(intToTextRef))
	for keyINT, textRef := range intToTextRef {
		if text, ok := textByRef[textRef]; ok {
			textByInt[keyINT] = text
		}
	}

	return textByInt, nil
}

func (d dbInteraction) getKeyINT() uint64 {
	return d.KeyINT
}

func (d dbInteraction) getTextRef() *uint64 {
	return d.TextRef
}

func (d compoundDBInteraction) getKeyINT() uint64 {
	return d.KeyINT
}

func (d compoundDBInteraction) getTextRef() *uint64 {
	return d.TextRef
}
