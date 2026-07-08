package adrcontroller

import (
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

	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type ADRController struct {
	DB                  *sqlx.DB
	Limits              cfg.LimitsConfig
	FrequencyTranslator func(*int, string) *string
}

func NewADRController(resourceHandle *handle.ResourceHandle) *ADRController {
	return &ADRController{
		DB:                  resourceHandle.SQLX,
		Limits:              resourceHandle.Limits,
		FrequencyTranslator: format.NewAdrFrequencyTranslator(),
	}
}

// @Summary		List ADRs for PZNs
// @Description	Get ADRs for one or more PZNs. Each PZN can have multiple ADRs.
// @Description	The `lang` parameter can be used to specify the language of the ADR descriptions.
// @Description	Valid values are `english`, `german`, and `german-simple`.
// @Description	The default language is `english`.
// @Description	`german-simple` returns the simplified German ADR description.
// @Tags			Adverse Drug Reactions
// @Produce		json
// @Param			pzns	query	string	true	"Comma-separated list of PZNs"
// @Param			lang	query	string	false	"Language for ADR names (default: english)"	Enums(english,german,german-simple)
// @Success		200		{array}	PznADR	"List of PZNs with ADRs"
// @Failure		400		"Bad request (e.g. invalid PZNs)"
// @Failure		404		"PZN(s) not found"
// @Router			/adrs/pzns [get]
func (ac *ADRController) GetAdrsForPZNs(c *gin.Context) {
	var query = struct {
		PZNs     string `form:"pzns"`
		Language string `form:"lang" binding:"omitempty,oneof=english german german-simple"`
	}{
		Language: "english",
	}

	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")
	res, err := fetchPznAdrs(pzns, ac.DB, ac, query.Language)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, res)
}

// @Summary		List ADRs for a compound name
// @Description	Get ADRs for formulations/routes matching a compound name query.
// @Description	The `lang` parameter can be used to specify the language of the ADR descriptions.
// @Description	Valid values are `english`, `german`, and `german-simple`.
// @Description	The default language is `english`.
// @Description	`german-simple` returns the simplified German ADR description.
// @Description	The `compound` parameter accepts repeated parameters (preferred, preserves commas) or a single comma-joined value.
// @Tags			Adverse Drug Reactions
// @Produce		json
// @Param			compound	query	[]string			true	"Compound name search terms, as repeated parameters (preferred) or a single comma-joined value"	collectionFormat(multi)	example:"metformin,metoprolol"
// @Param			lang		query	string				false	"Language for ADR names (default: english)"		Enums(english,german,german-simple)
// @Param			application	query	string				false	"Application filter (default: peroral)"			Enums(extern,invasive,peroral,all)
// @Success		200			{array}	CompoundADRGroup	"Matching compound/formulation ADRs grouped by input"
// @Failure		400			"Bad request (e.g. missing compound query or too many names)"
// @Failure		500			"Internal server error"
// @Router			/adrs/compounds [get]
func (ac *ADRController) GetAdrsForCompound(c *gin.Context) {
	var query = struct {
		Compound    []string `form:"compound"`
		Language    string   `form:"lang" binding:"omitempty,oneof=english german german-simple"`
		Application string   `form:"application" binding:"omitempty,oneof=extern invasive peroral all"`
	}{
		Language:    "english",
		Application: "peroral",
	}

	if !handle.QueryBind(c, &query) {
		return
	}

	compounds := splitAndTrim(handle.NormalizeList(query.Compound))
	if len(compounds) == 0 {
		handle.BadRequestError(c, "Missing required parameter: compound")
		return
	}

	if len(compounds) > ac.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", ac.Limits.BatchQueries))
		return
	}

	res, err := fetchCompoundAdrs(compounds, ac.DB, ac, query.Language, query.Application)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, res)
}

type PznADR struct {
	PZN  string `json:"pzn"`
	ADRs []ADR  `json:"adrs"`
}

type ADR struct {
	KeyFAM       uint64  `db:"Key_FAM" json:"-"`
	KeyMIV       uint64  `db:"Key_MIV" json:"-"`
	FrequencyInt *int    `db:"Haeufigkeit" json:"frequency_code"`
	Frequency    *string `json:"frequency"`
	Descriptor   string  `db:"Name" json:"description"`
}

type CompoundADRItem struct {
	CompoundName string `db:"compound_name" json:"compound_name"`
	Application  string `json:"application"`
	KeyFAM       uint64 `json:"-"`
	ADRs         []ADR  `json:"adrs"`
}

type CompoundADRGroup struct {
	Input string            `json:"input"`
	Items []CompoundADRItem `json:"items"`
}

func fetchPznAdrs(pzns []string, db *sqlx.DB, ac *ADRController, lang string) ([]PznADR, error) {
	if err := validate.PZNs(pzns, 1, ac.Limits.InteractionDrugs); err != nil {
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
	adrs, err := fetchAdrs(db, fams, lang, ac)
	if err != nil {
		return nil, apierr.New(http.StatusInternalServerError, err.Error())
	}

	famMap := make(map[uint64][]ADR, len(pzns))
	for _, adr := range adrs {
		famMap[adr.KeyFAM] = append(famMap[adr.KeyFAM], adr)
	}

	pznAdrs := make([]PznADR, 0, len(pzns))
	for key, adr := range famMap {
		famPzns := famPznMap[key]
		for _, pzn := range famPzns {
			pznAdrs = append(pznAdrs, PznADR{PZN: pzn, ADRs: adr})
		}
	}

	return pznAdrs, nil
}

func fetchAdrs(db *sqlx.DB, fams []uint64, lang string, ac *ADRController) ([]ADR, error) {
	langKey := 2 // english
	if lang != "english" {
		langKey = 1
	}
	simpleLang := lang == "german-simple"

	queryBuilder := squirrel.Select(
		"NEB_C.Key_FAM",
		"NEB_C.Key_MIV",
		"NEB_C.Haeufigkeit",
		"MIN_C.Name").
		From("NEB_C").
		Join("MIN_C ON NEB_C.Key_MIV = MIN_C.Key_MIV").
		Where(squirrel.And{
			squirrel.Eq{"NEB_C.Key_FAM": fams},
			squirrel.Expr("MIN_C.Key_MIV = NEB_C.Key_MIV"),
			squirrel.Eq{"Sprache": langKey},
		}).OrderBy("NEB_C.Key_FAM")

	if simpleLang {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"Vorzugsbezeichnung_L": 1})
	}

	var adrs []ADR
	query, args, _ := queryBuilder.ToSql()
	err := db.Select(&adrs, query, args...) //nolint:musttag // we only fetch raw data and mix it with translated data
	if err != nil {
		return nil, fmt.Errorf("error fetching adrs for PZNs: %w", err)
	}

	type adrKey struct {
		KeyFAM uint64
		KeyMIV uint64
	}
	seen := make(map[adrKey]struct{}, len(adrs))
	filtered := make([]ADR, 0, len(adrs))
	for i := range adrs {
		key := adrKey{
			KeyFAM: adrs[i].KeyFAM,
			KeyMIV: adrs[i].KeyMIV,
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		adrs[i].Frequency = ac.FrequencyTranslator(adrs[i].FrequencyInt, lang)
		filtered = append(filtered, adrs[i])
	}

	slices.SortStableFunc(filtered, func(a, b ADR) int {
		if a.FrequencyInt == nil && b.FrequencyInt == nil {
			return 0
		}
		if a.FrequencyInt == nil {
			return 1
		}
		if b.FrequencyInt == nil {
			return -1
		}
		return *a.FrequencyInt - *b.FrequencyInt
	})

	return filtered, nil
}

func fetchCompoundAdrs(
	compounds []string, db *sqlx.DB, ac *ADRController, lang string, application string,
) ([]CompoundADRGroup, error) {
	rows, err := fetchCompoundRows(db, compounds)
	if err != nil {
		return nil, err
	}

	rowsByInput := groupCompoundRowsByInput(compounds, rows)
	candidateFAMs := make([]uint64, 0, len(rows))
	seenCandidateFAMs := make(map[uint64]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seenCandidateFAMs[row.KeyFAM]; ok {
			continue
		}
		seenCandidateFAMs[row.KeyFAM] = struct{}{}
		candidateFAMs = append(candidateFAMs, row.KeyFAM)
	}

	famPznMap, err := fetchRepresentativePZNsByFAM(db, candidateFAMs)
	if err != nil {
		return nil, err
	}

	groupedItems := make([]CompoundADRGroup, 0, len(compounds))
	allSelectedFAMs := make([]uint64, 0)
	seenSelectedFAMs := make(map[uint64]struct{})

	for _, input := range compounds {
		items, selectedFAMs := selectRepresentativeCompoundItems(rowsByInput[input], famPznMap)
		groupedItems = append(groupedItems, CompoundADRGroup{
			Input: input,
			Items: filterCompoundADRItems(items, application),
		})

		for _, fam := range selectedFAMs {
			if _, ok := seenSelectedFAMs[fam]; ok {
				continue
			}
			seenSelectedFAMs[fam] = struct{}{}
			allSelectedFAMs = append(allSelectedFAMs, fam)
		}
	}

	adrs, err := fetchAdrs(db, allSelectedFAMs, lang, ac)
	if err != nil {
		return nil, err
	}

	adrByFAM := make(map[uint64][]ADR, len(allSelectedFAMs))
	for _, adr := range adrs {
		adrByFAM[adr.KeyFAM] = append(adrByFAM[adr.KeyFAM], adr)
	}

	for i := range groupedItems {
		for j := range groupedItems[i].Items {
			groupedItems[i].Items[j].ADRs = adrByFAM[groupedItems[i].Items[j].KeyFAM]
		}
	}

	return groupedItems, nil
}

func fetchCompoundRows(db *sqlx.DB, compounds []string) ([]struct {
	CompoundName          string  `db:"compound_name"`
	KeySTO                uint64  `db:"key_sto"`
	KeyFAM                uint64  `db:"key_fam"`
	KeyDAR                *string `db:"key_dar"`
	FormulationName       *string `db:"formulation_name"`
	ApplicationRouteCode  *int    `db:"applikationsweg_code"`
	ApplicationRouteLabel string  `db:"applikationsweg_label"`
	ApplicationGroup      string  `db:"application_group"`
}, error) {
	prefixes := make([]string, 0, len(compounds))
	for _, compound := range compounds {
		prefixes = append(prefixes, strings.ToLower(compound)+"%")
	}

	queryBuilder := squirrel.Select(
		"sto.Name AS compound_name",
		"fai.Key_STO AS key_sto",
		"fai.Key_FAM AS key_fam",
		"fam.Key_DAR AS key_dar",
		"dar.Name AS formulation_name",
		"fap.Applikationsweg AS applikationsweg_code",
		`CASE fap.Applikationsweg
			WHEN 0 THEN 'k. A.'
			WHEN 1 THEN 'bronchopulmonal'
			WHEN 2 THEN 'extern'
			WHEN 3 THEN 'extrakorporal'
			WHEN 4 THEN 'intraoral'
			WHEN 5 THEN 'invasiv'
			WHEN 6 THEN 'nasal'
			WHEN 7 THEN 'okulär'
			WHEN 8 THEN 'peroral'
			WHEN 9 THEN 'rektal'
			WHEN 10 THEN 'urogenital'
			ELSE 'unknown'
		END AS applikationsweg_label`,
		`CASE fap.Applikationsweg
			WHEN 2 THEN 'extern'
			WHEN 5 THEN 'invasive'
			WHEN 8 THEN 'peroral'
			ELSE 'other'
		END AS application_group`,
	).Distinct().
		From("SNA_DB sto").
		Join("FAI_DB fai ON fai.Key_STO = sto.Key_STO").
		Join("FAM_DB fam ON fam.Key_FAM = fai.Key_FAM").
		LeftJoin("DAR_DB dar ON dar.Key_DAR = fam.Key_DAR").
		Join("FAP_DB fap ON fap.Key_FAM = fai.Key_FAM").
		Where(squirrel.Eq{"fai.Stofftyp": 1}).
		OrderBy("sto.Name", "dar.Name", "fai.Key_FAM", "fap.Applikationsweg")

	ors := make(squirrel.Or, 0, len(prefixes))
	for _, prefix := range prefixes {
		ors = append(ors, squirrel.Expr("sto.Name LIKE ?", prefix))
	}
	queryBuilder = queryBuilder.Where(ors)

	query, args, _ := queryBuilder.ToSql()
	rows := []struct {
		CompoundName          string  `db:"compound_name"`
		KeySTO                uint64  `db:"key_sto"`
		KeyFAM                uint64  `db:"key_fam"`
		KeyDAR                *string `db:"key_dar"`
		FormulationName       *string `db:"formulation_name"`
		ApplicationRouteCode  *int    `db:"applikationsweg_code"`
		ApplicationRouteLabel string  `db:"applikationsweg_label"`
		ApplicationGroup      string  `db:"application_group"`
	}{}

	if err := db.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching adrs for compound: %w", err)
	}

	return rows, nil
}

func selectRepresentativeCompoundItems(rows []struct {
	CompoundName          string  `db:"compound_name"`
	KeySTO                uint64  `db:"key_sto"`
	KeyFAM                uint64  `db:"key_fam"`
	KeyDAR                *string `db:"key_dar"`
	FormulationName       *string `db:"formulation_name"`
	ApplicationRouteCode  *int    `db:"applikationsweg_code"`
	ApplicationRouteLabel string  `db:"applikationsweg_label"`
	ApplicationGroup      string  `db:"application_group"`
}, famPznMap map[uint64]string) ([]CompoundADRItem, []uint64) {
	seenApplication := make(map[string]struct{}, len(rows))
	selectedFamSeen := make(map[uint64]struct{}, len(rows))
	items := make([]CompoundADRItem, 0, len(rows))
	selectedFAMs := make([]uint64, 0, len(rows))

	for _, row := range rows {
		if row.ApplicationGroup == "other" {
			continue
		}
		if _, ok := seenApplication[row.ApplicationGroup]; ok {
			continue
		}
		pzn, ok := famPznMap[row.KeyFAM]
		if !ok || pzn == "" {
			continue
		}

		seenApplication[row.ApplicationGroup] = struct{}{}
		items = append(items, CompoundADRItem{
			CompoundName: row.CompoundName,
			Application:  row.ApplicationGroup,
			KeyFAM:       row.KeyFAM,
			ADRs:         []ADR{},
		})

		if _, famSeen := selectedFamSeen[row.KeyFAM]; famSeen {
			continue
		}
		selectedFamSeen[row.KeyFAM] = struct{}{}
		selectedFAMs = append(selectedFAMs, row.KeyFAM)
	}

	return items, selectedFAMs
}

func fetchRepresentativePZNsByFAM(db *sqlx.DB, fams []uint64) (map[uint64]string, error) {
	if len(fams) == 0 {
		return map[uint64]string{}, nil
	}

	queryBuilder := squirrel.Select("Key_FAM", "PZN").
		From("PAE_DB").
		Where(squirrel.Eq{"Key_FAM": fams}).
		OrderBy("Key_FAM", "PZN")

	query, args, _ := queryBuilder.ToSql()
	rows := []struct {
		KeyFAM uint64 `db:"Key_FAM"`
		PZN    string `db:"PZN"`
	}{}

	if err := db.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching representative PZNs by FAM: %w", err)
	}

	result := make(map[uint64]string, len(fams))
	for _, row := range rows {
		if _, ok := result[row.KeyFAM]; ok {
			continue
		}
		result[row.KeyFAM] = row.PZN
	}

	return result, nil
}

func filterCompoundADRItems(items []CompoundADRItem, application string) []CompoundADRItem {
	if application == "all" {
		return items
	}

	filtered := make([]CompoundADRItem, 0, len(items))
	for _, item := range items {
		if item.Application == application {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

func groupCompoundRowsByInput(compounds []string, rows []struct {
	CompoundName          string  `db:"compound_name"`
	KeySTO                uint64  `db:"key_sto"`
	KeyFAM                uint64  `db:"key_fam"`
	KeyDAR                *string `db:"key_dar"`
	FormulationName       *string `db:"formulation_name"`
	ApplicationRouteCode  *int    `db:"applikationsweg_code"`
	ApplicationRouteLabel string  `db:"applikationsweg_label"`
	ApplicationGroup      string  `db:"application_group"`
}) map[string][]struct {
	CompoundName          string  `db:"compound_name"`
	KeySTO                uint64  `db:"key_sto"`
	KeyFAM                uint64  `db:"key_fam"`
	KeyDAR                *string `db:"key_dar"`
	FormulationName       *string `db:"formulation_name"`
	ApplicationRouteCode  *int    `db:"applikationsweg_code"`
	ApplicationRouteLabel string  `db:"applikationsweg_label"`
	ApplicationGroup      string  `db:"application_group"`
} {
	grouped := make(map[string][]struct {
		CompoundName          string  `db:"compound_name"`
		KeySTO                uint64  `db:"key_sto"`
		KeyFAM                uint64  `db:"key_fam"`
		KeyDAR                *string `db:"key_dar"`
		FormulationName       *string `db:"formulation_name"`
		ApplicationRouteCode  *int    `db:"applikationsweg_code"`
		ApplicationRouteLabel string  `db:"applikationsweg_label"`
		ApplicationGroup      string  `db:"application_group"`
	}, len(compounds))

	for _, compound := range compounds {
		grouped[compound] = nil
	}

	for _, row := range rows {
		rowName := strings.ToLower(row.CompoundName)
		for _, compound := range compounds {
			if strings.HasPrefix(rowName, strings.ToLower(compound)) {
				grouped[compound] = append(grouped[compound], row)
			}
		}
	}

	return grouped
}

func splitAndTrim(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}

	return result
}
