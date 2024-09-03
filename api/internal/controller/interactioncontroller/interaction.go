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
	PlausibilityTranslator func(*int, bool) *string
	RelevanceTranslator    func(*int, bool) *string
	FrequencyTranslator    func(*int, bool) *string
	CredibilityTranslator  func(*int, bool) *string
	DirectionTranslator    func(*int, bool) *string
	DescriptionStruct      any
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
		DescriptionStruct:      format.Description(),
	}
}

func (ic *InteractionController) GetInterDescription(c *gin.Context) {
	c.JSON(http.StatusOK, ic.DescriptionStruct)
}

func (ic *InteractionController) PostInterPZNs(c *gin.Context) {
	type Query struct {
		ID           string   `json:"id" binding:"required"`
		PZNs         []string `json:"pzns" binding:"required"`
		DetailedDesc bool     `json:"details" binding:"omitempty"`
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

			result, err := fetchPZNInteractions(query.PZNs, db, ic, query.DetailedDesc)
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
	var query struct {
		PZNs         string `form:"pzns" binding:"required"`
		DetailedDesc bool   `form:"details" binding:"omitempty"`
	}

	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")

	result, err := fetchPZNInteractions(pzns, ic.DB, ic, query.DetailedDesc)
	if err != nil {
		handle.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"interactions": result})
}

func (ic *InteractionController) PostInterCompounds(c *gin.Context) {
	type Query struct {
		ID           string   `json:"id" binding:"required"`
		Compounds    []string `json:"compounds" binding:"required"`
		FetchDoses   bool     `json:"doses" binding:"omitempty"`
		DetailedDesc bool     `json:"details" binding:"omitempty"`
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
		Interactions *[]CompoundInteraction `json:"interactions"`
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

			result, err := fetchCompoundInteractions(query.Compounds, db, ic, query.FetchDoses, query.DetailedDesc)
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

// compounds: comma separated list of compounds
// doses: boolean flag to fetch doses
func (ic *InteractionController) GetInterCompounds(c *gin.Context) {
	var query struct {
		Compounds    string `form:"compounds" binding:"required"`
		FetchDose    bool   `form:"doses" binding:"omitempty"`
		DetailedDesc bool   `form:"details" binding:"omitempty"`
	}

	if !handle.QueryBind(c, &query) {
		return
	}

	compounds := strings.Split(query.Compounds, ",")

	result, err := fetchCompoundInteractions(compounds, ic.DB, ic, query.FetchDose, query.DetailedDesc)
	if err != nil {
		handle.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"interactions": result})
}

type CompoundInteraction struct {
	Plausibility *string         `json:"plausibility"`
	Relevance    *string         `json:"relevance"`
	Frequency    *string         `json:"frequency"`
	Credibility  *string         `json:"credibility"`
	Direction    *string         `json:"direction"`
	CompoundL    string          `json:"compound_left"`
	CompoundR    string          `json:"compound_right"`
	DosesL       []*CompoundDose `json:"doses_left"`
	DosesR       []*CompoundDose `json:"doses_right"`
}

func fetchCompoundInteractions( //nolint:gocognit // splitting up this function would make it less readable
	compounds []string,
	db *sqlx.DB,
	ic *InteractionController,
	fetchDoses bool,
	detailedDesc bool,
) ([]CompoundInteraction, error) {
	if err := validateCompounds(compounds, ic.Limits.InteractionDrugs); err != nil {
		return nil, apierr.New(http.StatusBadRequest, err.Error())
	}

	stoCompoundMap, err := fetchStoCompoundPairs(db, compounds)
	if err != nil {
		return nil, apierr.New(http.StatusInternalServerError, err.Error())
	}

	if diff := helper.StrSetDifference(compounds, slices.Collect(maps.Values(stoCompoundMap))); len(diff) > 0 {
		return nil, apierr.New(http.StatusNotFound, fmt.Sprintf("Compounds not found: %s", strings.Join(diff, ", ")))
	}

	keySto := slices.Collect(maps.Keys(stoCompoundMap))
	queryBuilder := squirrel.Select(
		"INT_C.Key_INT",
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
	var dbInteractions []struct {
		KeyINT       uint64 `db:"Key_INT"`
		Plausibility *int   `db:"Plausibilitaet"`
		Relevance    *int   `db:"Relevanz"`
		Frequency    *int   `db:"Haeufigkeit"`
		Credibility  *int   `db:"Quellenbewertung"`
		Direction    *int   `db:"Richtung"`
		KeyStoL      uint64 `db:"Key_STO_L"`
		KeyStoR      uint64 `db:"Key_STO_R"`
	}

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
			CompoundL:    stoCompoundMap[interaction.KeyStoL],
			CompoundR:    stoCompoundMap[interaction.KeyStoR],
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
					} else {
						results[i].DosesR = append(results[i].DosesR, &dose)
					}
				}
			}
		}
	}

	return results, nil
}

type PZNInteraction struct {
	Plausibility *string `json:"plausibility"`
	Relevance    *string `json:"relevance"`
	Frequency    *string `json:"frequency"`
	Credibility  *string `json:"credibility"`
	Direction    *string `json:"direction"`
	PZNL         string  `json:"pzn_left"`
	PZNR         string  `json:"pzn_right"`
}

func fetchPZNInteractions(
	pzns []string,
	db *sqlx.DB,
	ic *InteractionController,
	detailedDesc bool,
) ([]PZNInteraction, error) {
	if err := validatePZNS(pzns, ic.Limits.InteractionDrugs); err != nil {
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
			Plausibility: ic.PlausibilityTranslator(interaction.Plausibility, detailedDesc),
			Relevance:    ic.RelevanceTranslator(interaction.Relevance, detailedDesc),
			Frequency:    ic.FrequencyTranslator(interaction.Frequency, detailedDesc),
			Credibility:  ic.CredibilityTranslator(interaction.Credibility, detailedDesc),
			Direction:    ic.DirectionTranslator(interaction.Direction, detailedDesc),
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

func validateCompounds(compounds []string, maxdrugs int) error {
	if len(compounds) < 2 {
		return errors.New("at least two compounds must be provided")
	}

	if len(compounds) > maxdrugs {
		return fmt.Errorf("too many compounds provided. Maximum is %d", maxdrugs)
	}

	if !helper.IsUnique(compounds) {
		return errors.New("duplicate compounds provided")
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

func fetchStoCompoundPairs(db *sqlx.DB, compounds []string) (map[uint64]string, error) {
	queryBuilder := squirrel.Select("Name", "Key_STO").From("SNA_DB").Where(squirrel.Eq{"Name": compounds})
	query, args, _ := queryBuilder.ToSql()

	var snaPairs []struct {
		Name   string `db:"Name"`
		KeySTO uint64 `db:"Key_STO"`
	}

	err := db.Select(&snaPairs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching Compound-STO pairs: %w", err)
	}

	stoCompoundMap := make(map[uint64]string)
	for _, pair := range snaPairs {
		stoCompoundMap[pair.KeySTO] = pair.Name
	}

	return stoCompoundMap, nil
}

type CompoundDose struct {
	KeySTO              uint64   `db:"Key_STO" json:"-"`
	KeyINT              uint64   `db:"Key_INT" json:"-"`
	Value               *float64 `db:"Zahl" json:"value"`
	Unit                *string  `db:"Einheit" json:"unit"`
	Suffix              *string  `db:"Suffix" json:"suffix"`
	DosageForm          *string  `db:"Key_DAR" json:"dosage_form"`
	EquivalentSubstance bool     `db:"ES" json:"equivalent_substance"`
}

func fetchCompoundDoses(db *sqlx.DB, keyInt, keySto []uint64) ([]CompoundDose, error) {
	queryBuilder := squirrel.Select(
		"FAI_DB.Key_STO",
		"FZI_C.Key_INT",
		"Zahl",
		"Einheit",
		"Suffix",
		"Key_DAR",
		"Entsprichtstoff IS NULL AS ES").
		From("FZI_C").
		Distinct().
		LeftJoin("FAI_DB ON FAI_DB.Key_FAM = FZI_C.Key_FAM").
		LeftJoin("FAM_DB ON FAI_DB.Key_FAM = FAM_DB.Key_FAM").
		Where(squirrel.And{
			squirrel.Eq{"FZI_C.Key_INT": keyInt},
			squirrel.Eq{"FZI_C.Key_STO": keySto},
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
