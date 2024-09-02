package interactioncontroller

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"observeddb-go-api/cfg"
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
	PlausibilityTranslator func(dbVal *int) *string
	RelevanceTranslator    func(value *int) *string
	FrequencyTranslator    func(value *int) *string
	CredibilityTranslator  func(value *int) *string
	DirectionTranslator    func(value *int) *string
}

func NewInteractionController(resourceHandle *handle.ResourceHandle) *InteractionController {
	return &InteractionController{
		DB:                     resourceHandle.SQLX,
		Limits:                 resourceHandle.Limits,
		PlausibilityTranslator: format.NewPlausibilityTranslator(),
		RelevanceTranslator:    format.NewRelevanceTranslator(),
		FrequencyTranslator:    format.NewFrequencyTranslator(),
		CredibilityTranslator:  format.NewCredibilityTranslator(),
		DirectionTranslator:    format.NewDirectionTranslator(),
	}
}

func (ic *InteractionController) PostInterPZNs(c *gin.Context) {
	type Query struct {
		ID   string   `json:"id" binding:"required"`
		PZNs []string `json:"pzns" binding:"required"`
	}
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
		ID string `json:"id"`
		apierr.ResStatus
		Interactions *[]PZNInteraction `json:"interactions"`
	}

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

			result, err := fetchPZNInteractions(query.PZNs, db, ic)
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
	c.JSON(apierr.BatchStatusCode(n, nSuccess), results)
}

// pzns: comma separated list of PZNs
func (ic *InteractionController) GetInterPZNs(c *gin.Context) {
	pznQuery := c.Query("pzns")
	pzns := strings.Split(pznQuery, ",")

	result, err := fetchPZNInteractions(pzns, ic.DB, ic)
	if err != nil {
		handle.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"interactions": result})
}

type PZNInteraction struct {
	Plausibility *string `json:"plausibility"`
	Relevance    *string `json:"relevance"`
	Frequency    *string `json:"frequency"`
	Credibility  *string `json:"credibility"`
	Direction    *string `json:"direction"`
	PZNL         string  `json:"pzn_l"`
	PZNR         string  `json:"pzn_r"`
}

func fetchPZNInteractions(pzns []string, db *sqlx.DB, ic *InteractionController) ([]PZNInteraction, error) {
	if err := validatePZNS(pzns, ic.Limits.BatchQueries); err != nil {
		return nil, apierr.New(http.StatusBadRequest, err.Error())
	}

	famPZNMap, err := fetchFamPZNPairs(db, pzns)
	if err != nil {
		return nil, apierr.New(http.StatusInternalServerError, err.Error())
	}

	if diff := helper.SetDifference(pzns, slices.Collect(maps.Values(famPZNMap))); len(diff) > 0 {
		return nil, apierr.New(http.StatusNotFound, fmt.Sprintf("PZNs not found: %s", strings.Join(diff, ", ")))
	}

	fams := slices.Collect(maps.Keys(famPZNMap))
	queryBuilder := squirrel.Select(
		"INT_C.Plausibilitaet",
		"INT_C.Relevanz",
		"INT_C.Haeufigkeit",
		"INT_C.Quellenbewertung",
		"INT_C.Richtung",
		"FZI_C1.Key_FAM AS Key_FAM_R",
		"FZI_C2.Key_FAM AS Key_FAM_L").
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
	var dbInteractions []struct {
		Plausibility *int   `db:"Plausibilitaet"`
		Relevance    *int   `db:"Relevanz"`
		Frequency    *int   `db:"Haeufigkeit"`
		Credibility  *int   `db:"Quellenbewertung"`
		Direction    *int   `db:"Richtung"`
		KeyFAML      uint64 `db:"Key_FAM_L"`
		KeyFAMR      uint64 `db:"Key_FAM_R"`
	}

	err = db.Select(&dbInteractions, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching interactions: %w", err)
	}

	var results = make([]PZNInteraction, len(dbInteractions))
	for i, interaction := range dbInteractions {
		results[i] = PZNInteraction{
			Plausibility: ic.PlausibilityTranslator(interaction.Plausibility),
			Relevance:    ic.RelevanceTranslator(interaction.Relevance),
			Frequency:    ic.FrequencyTranslator(interaction.Frequency),
			Credibility:  ic.CredibilityTranslator(interaction.Credibility),
			Direction:    ic.DirectionTranslator(interaction.Direction),
			PZNL:         famPZNMap[interaction.KeyFAML],
			PZNR:         famPZNMap[interaction.KeyFAMR],
		}
	}

	return results, nil
}

func validatePZNS(pzns []string, maxdrugs int) error {
	if len(pzns) < 2 {
		return errors.New("at least two PZNs must be provided")
	}

	if len(pzns) > maxdrugs {
		return fmt.Errorf("too many PZNs provided. Maximum is %d", maxdrugs)
	}

	if err := validate.PZNBatch(pzns); err != nil {
		return fmt.Errorf("invalid PZNs provided: %s", err.Error())
	}

	if !helper.IsUnique(pzns) {
		return errors.New("duplicate PZNs provided")
	}

	return nil
}

func fetchFamPZNPairs(db *sqlx.DB, pzns []string) (map[uint64]string, error) {
	n := len(pzns)
	queryBuilder := squirrel.Select("PZN", "Key_FAM").From("PAE_DB").Where(squirrel.Eq{"PZN": pzns}).Limit(uint64(n))
	query, args, _ := queryBuilder.ToSql()

	var paePairs []struct {
		PZN    string `db:"PZN"`
		KeyFAM uint64 `db:"Key_FAM"`
	}

	err := db.Select(&paePairs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching FAM-PZN pairs: %w", err)
	}

	famPZNMap := make(map[uint64]string, len(paePairs))
	for _, paePair := range paePairs {
		famPZNMap[paePair.KeyFAM] = paePair.PZN
	}

	return famPZNMap, nil
}
