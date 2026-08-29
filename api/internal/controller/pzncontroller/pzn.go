package pzncontroller

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/utils/apierr"
	"observeddb-go-api/internal/utils/format"
	"observeddb-go-api/internal/utils/validate"
	"slices"
	"sort"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type PZNController struct {
	DB                 *sqlx.DB
	Limits             cfg.LimitsConfig
	CategoryTranslator func(*int, bool) *string
}

func NewPZNController(resourceHandle *handle.ResourceHandle) *PZNController {
	return &PZNController{
		DB:                 resourceHandle.SQLX,
		Limits:             resourceHandle.Limits,
		CategoryTranslator: format.NewProductTranslator(),
	}
}

// @Summary		Search products by name
// @Description	Fuzzy-search products by one or more product names and return matching PZNs with active compounds grouped by input. Standard notes are included only when notes=true.
// @Tags			Product
// @Produce		json
// @Param			name		query	string				true	"Comma-separated product name search terms; encode literal commas as %2C"	example:"Delix+2%2C5,Plavix,Ramilich"
// @Param			limit		query	int					false	"Maximum number of products to return per input (default: 20, max: 100)"
// @Param			notes		query	bool				false	"Include standard notes"								default(false)
// @Param			indications	query	bool				false	"Include indications and the complete ATC hierarchy"	default(false)
// @Param			lang		query	string				false	"Language for standard notes, reviewed indication names, and ATC labels: german or english (default: german)"
// @Success		200			{array}	ProductSearchGroup	"Matching products with active compounds grouped by input"
// @Failure		400			"Bad request (e.g. missing name)"
// @Failure		500			"Internal server error"
// @Router			/product/search [get]
//
// @Security		Bearer
func (pc *PZNController) GetProductSearch(c *gin.Context) {
	var query struct {
		Limit       int    `form:"limit"`
		Notes       bool   `form:"notes"`
		Indications bool   `form:"indications"`
		Lang        string `form:"lang"`
	}
	if !handle.QueryBind(c, &query) {
		return
	}

	names := productNameQueryValues(c.Request.URL.RawQuery, "name")
	if len(names) == 0 {
		names = productNameQueryValues(c.Request.URL.RawQuery, "q")
	}
	if len(names) == 0 {
		handle.BadRequestError(c, "Missing required parameter: name")
		return
	}
	if len(names) > pc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", pc.Limits.BatchQueries))
		return
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	lang, err := normalizeStandardNoteLang(query.Lang)
	if err != nil {
		handle.BadRequestError(c, err.Error())
		return
	}

	result, err := fetchProductsByNames(names, limit, query.Notes, query.Indications, lang, pc.DB)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

type ProductSearchGroup struct {
	Input   string                `json:"input"`
	Results []ProductSearchResult `json:"results"`
}

type ProductSearchResult struct {
	ProductName     string              `json:"product_name"`
	PZN             string              `json:"pzn"`
	ActiveCompounds []string            `json:"active_compounds"`
	StandardNotes   []StandardNoteGroup `json:"standard_notes,omitempty"`
	Indications     []ProductIndication `json:"indications,omitempty"`
}

func fetchProductsByNames(names []string, limit int, includeNotes bool, includeIndications bool, lang string, db *sqlx.DB) ([]ProductSearchGroup, error) {
	results := make([]ProductSearchGroup, 0, len(names))
	for _, name := range names {
		products, err := fetchProductsByName(name, limit, includeNotes, includeIndications, lang, db)
		if err != nil {
			return nil, err
		}
		results = append(results, ProductSearchGroup{
			Input:   name,
			Results: products,
		})
	}

	return results, nil
}

// searchCompoundRow is a single active-compound row for the product search path,
// keyed by its owning Key_FAM.
type searchCompoundRow struct {
	KeyFAM  uint64  `db:"Key_FAM"`
	KeySTO  string  `db:"Key_STO"`
	Name    string  `db:"Name"`
	KeySTO1 *string `db:"Key_STO_1"`
}

func fetchProductsByName(name string, limit int, includeNotes bool, includeIndications bool, lang string, db *sqlx.DB) ([]ProductSearchResult, error) {
	// G115: limit is clamped to [1,100] by GetProductSearch before this call, so it cannot overflow or go negative.
	limitVal := uint64(limit) //nolint:gosec // limit is clamped to [1,100] upstream.
	productQuery := squirrel.Select(
		"FAM_DB.Key_FAM",
		"FAM_DB.Produktname",
		"PAE_DB.PZN").
		Distinct().
		From("FAM_DB").
		Join("PAE_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM").
		Where("PAE_DB.PZN IS NOT NULL").
		Where("FAM_DB.Key_ATC IS NOT NULL").
		Where("FAM_DB.Veterinaerpraeparat = 0").
		Where("LOWER(FAM_DB.Produktname) LIKE ?", "%"+strings.ToLower(name)+"%").
		OrderBy("FAM_DB.Produktname", "PAE_DB.PZN").
		Limit(limitVal)

	query, args, _ := productQuery.ToSql()
	products := []struct {
		KeyFAM      uint64 `db:"Key_FAM"`
		ProductName string `db:"Produktname"`
		PZN         string `db:"PZN"`
	}{}
	if err := db.Select(&products, query, args...); err != nil {
		return nil, fmt.Errorf("error searching products by name: %w", err)
	}
	if len(products) == 0 {
		return []ProductSearchResult{}, nil
	}

	fams := make([]uint64, 0, len(products))
	for _, product := range products {
		fams = append(fams, product.KeyFAM)
	}

	compoundsByFAM, err := activeCompoundsByFAM(fams, db)
	if err != nil {
		return nil, err
	}
	standardNotesByFAM := map[uint64][]StandardNoteGroup{}
	if includeNotes {
		standardNotesByFAM, err = fetchStandardNotesByFAM(fams, lang, db)
		if err != nil {
			return nil, err
		}
	}
	indicationsByFAM := map[uint64][]ProductIndication{}
	if includeIndications {
		indicationsByFAM, err = fetchIndicationsByFAM(fams, lang, db)
		if err != nil {
			return nil, err
		}
	}

	results := make([]ProductSearchResult, 0, len(products))
	for _, product := range products {
		result := ProductSearchResult{
			ProductName:     product.ProductName,
			PZN:             product.PZN,
			ActiveCompounds: compoundsByFAM[product.KeyFAM],
		}
		if includeNotes {
			result.StandardNotes = standardNotesByFAM[product.KeyFAM]
		}
		if includeIndications {
			result.Indications = indicationsByFAM[product.KeyFAM]
		}
		results = append(results, result)
	}

	return results, nil
}

// activeCompoundsByFAM fetches active compounds for the given Key_FAMs and returns
// them grouped per Key_FAM, dropping derivates (a Key_STO that appears as another
// row's Key_STO_1 within the same Key_FAM). Every requested Key_FAM is present in
// the result map (with an empty slice when it has no surviving compound).
func activeCompoundsByFAM(fams []uint64, db *sqlx.DB) (map[uint64][]string, error) {
	compoundQuery := squirrel.Select(
		"FAI_DB.Key_FAM",
		"FAI_DB.Key_STO",
		"SNA_DB.Name",
		"VSS_DB.Key_STO_1").
		Distinct().
		From("FAI_DB").
		LeftJoin("VSS_DB ON VSS_DB.Key_STO_2 = FAI_DB.Key_STO").
		LeftJoin("SNA_DB ON FAI_DB.Key_STO = SNA_DB.Key_STO").
		Where(squirrel.Eq{"FAI_DB.Key_FAM": fams}).
		Where("FAI_DB.Stofftyp = 1").
		Where("SNA_DB.Vorzugsbezeichnung = 1").
		Where(squirrel.Expr(`
			SNA_DB.Key_STO NOT IN (
				SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8
			)`)).
		OrderBy("FAI_DB.Key_FAM", "FAI_DB.Key_STO")

	query, args, _ := compoundQuery.ToSql()
	var compoundRows []searchCompoundRow
	if err := db.Select(&compoundRows, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching active compounds for product search: %w", err)
	}

	compoundsByFAM := make(map[uint64][]string, len(fams))
	for _, fam := range fams {
		compoundsByFAM[fam] = []string{}
	}

	groupedRows := make(map[uint64][]searchCompoundRow, len(fams))
	for _, row := range compoundRows {
		groupedRows[row.KeyFAM] = append(groupedRows[row.KeyFAM], row)
	}

	for fam, rows := range groupedRows {
		activeMap := make(map[string]string, len(rows))
		for _, row := range rows {
			active := true
			for _, other := range rows {
				if other.KeySTO1 != nil && row.KeySTO == *other.KeySTO1 {
					active = false
					break
				}
			}
			if active {
				activeMap[row.KeySTO] = row.Name
			}
		}
		compoundsByFAM[fam] = slices.Collect(maps.Values(activeMap))
	}

	return compoundsByFAM, nil
}

func productNameQueryValues(rawQuery string, key string) []string {
	values := make([]string, 0)
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}

		rawKey, rawValue, _ := strings.Cut(pair, "=")
		decodedKey, err := url.QueryUnescape(rawKey)
		if err != nil || decodedKey != key {
			continue
		}

		for _, rawPart := range strings.Split(rawValue, ",") {
			decodedPart, partErr := url.QueryUnescape(rawPart)
			if partErr != nil {
				continue
			}

			decodedPart = strings.TrimSpace(decodedPart)
			if decodedPart != "" {
				values = append(values, decodedPart)
			}
		}
	}

	return values
}

// productListRow is a single product/compound row returned by the product-list
// query, before grouping by PZN.
type productListRow struct {
	ProductName string  `db:"Produktname" json:"product"`
	ATC         string  `db:"Key_ATC" json:"atc"`
	PZN         string  `db:"PZN" json:"pzn"`
	KeySTO      string  `db:"Key_STO" json:"key_sto"`
	KeySTO1     *string `db:"Key_STO_1" json:"key_sto_1"`
	KeySTO2     *string `db:"Key_STO_2" json:"key_sto_2"`
	Typ         *int    `db:"Typ" json:"typ"`
	Name        string  `db:"Name" json:"name"`
}

// productListResult is one product entry (with its active compounds) in the
// product-list response.
type productListResult struct {
	ProductName     string   `json:"product"`
	ATC             string   `json:"atc"`
	PZN             *string  `json:"pzn"`
	ActiveCompounds []string `json:"active_compounds"`
}

func (pc *PZNController) GetProductList(c *gin.Context) {
	const pageSize = 1000

	var params struct {
		Page int `form:"page"`
	}

	if !handle.QueryBind(c, &params) {
		return
	}

	pages, ok := pc.countProductListPages(c, pageSize)
	if !ok {
		return
	}

	if params.Page <= 0 {
		handle.BadRequestError(c, "Page must be a positive integer")
		return
	}

	if params.Page > pages {
		handle.NotFoundError(c, fmt.Sprintf("Page %d not found, total pages: %d", params.Page, pages))
		return
	}

	pznsRes, ok := pc.fetchProductListPZNs(c, pageSize, params.Page)
	if !ok {
		return
	}

	dbResults, ok := pc.fetchProductListRows(c, pznsRes)
	if !ok {
		return
	}

	results := groupProductListResults(dbResults)

	data := struct {
		Pages      int `json:"pages"`
		Page       int `json:"page"`
		PZNPerPage int `json:"pzns_per_page"`
		// Data contains the list of products with their active compounds
		Data []productListResult `json:"products"`
	}{
		Pages:      pages,
		Page:       params.Page,
		PZNPerPage: pageSize,
		Data:       results,
	}

	handle.Success(c, data)
}

// countProductListPages runs the distinct-PZN count query and returns the total
// number of pages. On DB error it writes the error response and returns ok=false.
func (pc *PZNController) countProductListPages(c *gin.Context, pageSize int) (int, bool) {
	// Count the total number of distinct PZNs.
	type CountResult struct {
		Count int `db:"count"`
	}

	var cResult CountResult
	err := pc.DB.Get(&cResult, `
		SELECT COUNT(DISTINCT PZN) as count
		FROM PAE_DB
		LEFT JOIN FAM_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM
		WHERE PZN IS NOT NULL
		AND KEY_ATC IS NOT NULL
		AND FAM_DB.Veterinaerpraeparat = 0
	`)
	if err != nil {
		handle.Error(c, err)
		return 0, false
	}

	return (cResult.Count + pageSize - 1) / pageSize, true
}

// fetchProductListPZNs runs the paginated distinct-PZN query for the given page.
// On DB error or an empty page it writes the response and returns ok=false.
func (pc *PZNController) fetchProductListPZNs(c *gin.Context, pageSize, page int) ([]string, bool) {
	queryPzns := fmt.Sprintf(`
		SELECT DISTINCT PZN
		FROM PAE_DB
		LEFT JOIN FAM_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM
		WHERE PZN IS NOT NULL
		AND KEY_ATC IS NOT NULL
		AND FAM_DB.Veterinaerpraeparat = 0
		ORDER BY PZN
		LIMIT %d OFFSET %d`, pageSize, (page-1)*pageSize)

	var pznsRes []string
	if err := pc.DB.Select(&pznsRes, queryPzns); err != nil {
		handle.Error(c, err)
		return nil, false
	}
	if len(pznsRes) == 0 {
		handle.NotFoundError(c, fmt.Sprintf("No PZNs found for page %d", page))
		return nil, false
	}

	return pznsRes, true
}

// fetchProductListRows runs the product/compound query for the page's PZNs. On DB
// or query-build error it writes the error response and returns ok=false.
func (pc *PZNController) fetchProductListRows(c *gin.Context, pznsRes []string) ([]productListRow, bool) {
	queryBuilder := squirrel.Select(
		"DISTINCT FAM_DB.Produktname",
		"FAM_DB.Key_ATC",
		"PAE_DB.PZN",
		"SNA_DB.Key_STO",
		"VSS_DB.Key_STO_1",
		"VSS_DB.Key_STO_2",
		"VSS_DB.Typ",
		"SNA_DB.Name",
	).
		From("FAM_DB").
		LeftJoin("PAE_DB ON FAM_DB.Key_FAM = PAE_DB.Key_FAM").
		LeftJoin("FAI_DB ON FAI_DB.Key_FAM = FAM_DB.Key_FAM").
		LeftJoin("VSS_DB ON VSS_DB.Key_STO_2 = FAI_DB.Key_STO").
		LeftJoin("SNA_DB ON FAI_DB.Key_STO = SNA_DB.Key_STO").
		Where("FAM_DB.Key_ATC IS NOT NULL").
		Where("FAM_DB.Veterinaerpraeparat = 0").
		Where("FAI_DB.Stofftyp = 1").
		Where("PAE_DB.PZN IS NOT NULL").
		Where(squirrel.Or{
			squirrel.Expr("VSS_DB.Typ IS NULL"),
			squirrel.Expr("VSS_DB.Typ <> 100"),
		}).
		Where("SNA_DB.Vorzugsbezeichnung = 1").
		Where(squirrel.Expr(`
			SNA_DB.Key_STO NOT IN (
				SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8
			)`)).
		Where(squirrel.Eq{"PAE_DB.PZN": pznsRes})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		handle.Error(c, err)
		return nil, false
	}

	var dbResults []productListRow
	if selErr := pc.DB.Select(&dbResults, query, args...); selErr != nil {
		handle.Error(c, selErr)
		return nil, false
	}

	return dbResults, true
}

// groupProductListResults collapses the PZN-ordered rows into one entry per PZN,
// eliminating Key_STOs that also appear as another row's Key_STO_1 (derivates).
//
// NOTE: this preserves the existing behavior where the final PZN group is never
// flushed (only emitted when the next row has a different PZN), so a trailing
// group is dropped. The golden tests pin this exact behavior.
func groupProductListResults(dbResults []productListRow) []productListResult {
	var pznResults []productListRow
	var results []productListResult
	for _, res := range dbResults {
		if len(pznResults) == 0 || pznResults[0].PZN == res.PZN {
			pznResults = append(pznResults, res)
			continue
		}

		if pznResults[0].PZN != res.PZN {
			r := productListResult{
				ProductName:     pznResults[0].ProductName,
				ATC:             pznResults[0].ATC,
				PZN:             &pznResults[0].PZN,
				ActiveCompounds: []string{},
			}
			activeMap := make(map[string]string)
			for _, pznResult := range pznResults {
				ok := true
				for _, tmp := range pznResults {
					if tmp.KeySTO1 != nil && pznResult.KeySTO == *tmp.KeySTO1 {
						ok = false
						break
					}
				}
				if ok {
					activeMap[pznResult.KeySTO] = pznResult.Name
				}
			}
			r.ActiveCompounds = slices.Collect(maps.Values(activeMap))

			results = append(results, r)
			pznResults = []productListRow{res}
			continue
		}
	}

	return results
}

// @Summary		List active compounds for PZNs
// @Description	Get active compounds for one or more PZNs. Each PZN can have multiple active compounds.
// @Tags			Product
// @Produce		json
// @Param			pzns	query	string		true	"Comma separated string of PZNs"	example:"1234567,7654321"
// @Success		200		{array}	Compound	"List of PZNs with active compounds"
// @Failure		400		"Bad request (e.g. invalid PZNs)"
// @Failure		404		"PZN(s) not found"
// @Router			/product/activecompounds/pzns [get]
//
// @Security		Bearer
func (pc *PZNController) GetActiveCompounds(c *gin.Context) {
	type Query struct {
		PZNs string `form:"pzns" binding:"required" example:"1234567,7654321"`
	} //	@name	PZNActiveCoumpoundsQuery
	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}
	pzns := strings.Split(query.PZNs, ",")

	result, err := fetchActiveCompounds(pzns, pc.DB)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

type CompoundName struct {
	PZN       string   `json:"pzn"`
	Name      string   `json:"name"`
	Preferred bool     `json:"preferred"`
	Derivate  bool     `json:"derivate"`
	Standard  []string `json:"standards"`
}

type Compound struct {
	Names []CompoundName `json:"compound"`
}

func fetchActiveCompounds(pzns []string, db *sqlx.DB) ([]Compound, error) {
	for _, pzn := range pzns {
		if err := validate.PZN(pzn); err != nil {
			return nil, apierr.New(http.StatusBadRequest, fmt.Sprintf("Invalid PZN: %s", pzn))
		}
	}

	queryBuilder := squirrel.Select(
		"PAE_DB.PZN",
		"Name",
		"Typ",
		"Herkunft",
		"Vorzugsbezeichnung",
		"FAI_DB.Key_STO").
		From("PAE_DB").
		Distinct().
		RightJoin("FAI_DB ON PAE_DB.Key_FAM = FAI_DB.Key_FAM").
		LeftJoin("VSS_DB ON VSS_DB.Key_STO_2 = FAI_DB.Key_STO").
		LeftJoin("SNA_DB ON FAI_DB.Key_STO = SNA_DB.Key_STO").
		Where(squirrel.Eq{"PAE_DB.PZN": pzns}).
		Where(squirrel.Eq{"Stofftyp": 1}).
		// Where(squirrel.Or{squirrel.Eq{"Typ": nil}, squirrel.NotEq{"Typ": 100}}).
		Where("FAI_DB.Key_STO NOT IN (SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8)").
		OrderBy("PAE_DB.PZN, FAI_DB.Key_STO")

	query, args, _ := queryBuilder.ToSql()

	var dbResults []struct {
		PZN       string  `db:"PZN"`
		Name      string  `db:"Name"`
		Preferred bool    `db:"Vorzugsbezeichnung"`
		Derivate  *string `db:"Typ"`
		Standard  *string `db:"Herkunft"`
		KeySTO    uint64  `db:"Key_STO"`
	}

	err := db.Select(&dbResults, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching active compounds: %w", err)
	}

	if len(dbResults) == 0 {
		return nil, apierr.New(http.StatusNotFound, "PZN not found")
	}

	// Map to group compounds by PZN
	compoundsMap := make(map[string]*Compound)

	for _, dbResult := range dbResults {
		std := []string{}
		if dbResult.Standard != nil {
			std = strings.Split(*dbResult.Standard, ";")
		}

		der := false
		if dbResult.Derivate != nil {
			der = *dbResult.Derivate == "100"
		}

		cn := CompoundName{
			PZN:       dbResult.PZN,
			Name:      dbResult.Name,
			Preferred: dbResult.Preferred,
			Derivate:  der,
			Standard:  std,
		}

		// Retrieve or initialize a compound list for the given PZN
		if _, exists := compoundsMap[dbResult.PZN]; !exists {
			compoundsMap[dbResult.PZN] = &Compound{Names: []CompoundName{}}
		}

		// Append the compound name to the respective PZN entry
		compoundsMap[dbResult.PZN].Names = append(compoundsMap[dbResult.PZN].Names, cn)
	}

	// Convert map values to a slice
	var compounds []Compound
	for _, compound := range compoundsMap {
		compounds = append(compounds, *compound)
	}

	return compounds, nil
}

// PRODUCT INFO

// @Summary		List product info for PZNs
// @Description	Get product info for one or more PZNs. Indications are included only when indications=true.
// @Tags			Product
// @Produce		json
// @Param			pzns		query	string			true	"Comma separated string of PZNs"						example:"1234567,7654321"
// @Param			indications	query	bool			false	"Include indications and the complete ATC hierarchy"	default(false)
// @Param			lang		query	string			false	"Language for reviewed indication names and ATC labels: german or english (default: german)"
// @Success		200			{array}	ProductInfos	"List of PZNs with product info"
// @Failure		400			"Bad request (e.g. invalid PZNs)"
// @Failure		404			"PZN(s) not found"
// @Router			/product/info/pzns [get]
//
// @Security		Bearer
func (pc *PZNController) GetProductInfo(c *gin.Context) {
	type Query struct {
		PZNs        string `form:"pzns" binding:"required" example:"1234567,7654321"`
		Indications bool   `form:"indications"`
		Lang        string `form:"lang"`
	} //	@name	PZNActiveCompoundsQuery

	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")
	lang, err := normalizeStandardNoteLang(query.Lang)
	if err != nil {
		handle.BadRequestError(c, err.Error())
		return
	}

	result, err := fetchProductInfo(pzns, query.Indications, lang, pc.DB, pc.CategoryTranslator)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

// ProductInfo represents information about a product
type ProductInfo struct {
	PZN         string              `json:"pzn"`
	IsComb      bool                `json:"is_combination"`
	Category    string              `json:"category"`
	ProductName string              `json:"product_name"`
	Indications []ProductIndication `json:"indications,omitempty"`
}

// ProductInfos represents a list of product info for multiple PZNs
type ProductInfos struct {
	ProductInfoList []ProductInfo `json:"product_info"`
}

func fetchProductInfo(pzns []string, includeIndications bool, lang string, db *sqlx.DB, translate func(*int, bool) *string) ([]ProductInfo, error) {
	for _, pzn := range pzns {
		if err := validate.PZN(pzn); err != nil {
			return nil, apierr.New(http.StatusBadRequest, fmt.Sprintf("Invalid PZN: %s", pzn))
		}
	}

	queryBuilder := squirrel.Select(
		"PAE_DB.PZN",
		"FAM_DB.Key_FAM",
		"FAM_DB.Produktgruppe",
		"FAM_DB.Monopraeparat",
		"FAM_DB.Produktname").
		From("PAE_DB").
		Distinct().
		RightJoin("FAM_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM").
		Where(squirrel.Eq{"PAE_DB.PZN": pzns}).
		OrderBy("PAE_DB.PZN")

	query, args, _ := queryBuilder.ToSql()

	var dbResults []struct {
		PZN      string `db:"PZN"`
		KeyFAM   uint64 `db:"Key_FAM"`
		Category uint64 `db:"Produktgruppe"`
		Monop    uint64 `db:"Monopraeparat"`
		PName    string `db:"Produktname"`
	}

	err := db.Select(&dbResults, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching product info: %w", err)
	}

	if len(dbResults) == 0 {
		return nil, apierr.New(http.StatusNotFound, "PZN not found")
	}

	indicationsByFAM := map[uint64][]ProductIndication{}
	if includeIndications {
		fams := make([]uint64, 0, len(dbResults))
		for _, dbResult := range dbResults {
			fams = append(fams, dbResult.KeyFAM)
		}

		var indErr error
		indicationsByFAM, indErr = fetchIndicationsByFAM(fams, lang, db)
		if indErr != nil {
			return nil, indErr
		}
	}

	var productInfos []ProductInfo
	for _, dbResult := range dbResults {
		// Category is a small ABDA Produktgruppe enum code, so the uint64->int conversion cannot overflow.
		categoryInt := int(dbResult.Category) //nolint:gosec // Produktgruppe is a small enum code.
		categoryName := translate(&categoryInt, false)

		productInfo := ProductInfo{
			PZN:         dbResult.PZN,
			IsComb:      dbResult.Monop == 0,
			Category:    *categoryName, // Use translated category name
			ProductName: dbResult.PName,
		}
		if includeIndications {
			productInfo.Indications = indicationsByFAM[dbResult.KeyFAM]
		}
		productInfos = append(productInfos, productInfo)
	}

	return productInfos, nil
}

type ProductIndication struct {
	Name     string           `json:"name"`
	Language string           `json:"language"`
	ATCCodes []ProductATCCode `json:"atc_codes"`
}

type ProductATCCode struct {
	Code    string `json:"code"`
	LabelEN string `json:"label_en,omitempty"`
	Level   int    `json:"level,omitempty"`
	Source  string `json:"source,omitempty"`
}

type productIndicationSeed struct {
	KeyFAM      uint64  `db:"Key_FAM"`
	KeyINDHaupt *string `db:"Key_IND_Haupt"`
	KeyINDNeben *string `db:"Key_IND_Neben"`
	KeyATC      *string `db:"Key_ATC"`
	KeyATCA     *string `db:"Key_ATCA"`
}

type indicationRelation struct {
	Source *string `db:"Key_IND_Quelle"`
	Target *string `db:"Key_IND_Ziel"`
}

type indicationNameRow struct {
	KeyIND   string  `db:"Key_IND"`
	Name     *string `db:"indication_name"`
	Language *string `db:"indication_language"`
}

type atcMappingRow struct {
	Code       string  `db:"atc_code"`
	Level      int     `db:"level"`
	LabelEN    *string `db:"label_en"`
	SourceYear *int    `db:"source_year"`
	SourceURL  *string `db:"source_url"`
}

func fetchIndicationsByFAM(fams []uint64, lang string, db *sqlx.DB) (map[uint64][]ProductIndication, error) {
	indicationsByFAM := make(map[uint64][]ProductIndication, len(fams))
	for _, fam := range fams {
		indicationsByFAM[fam] = []ProductIndication{}
	}
	if len(fams) == 0 {
		return indicationsByFAM, nil
	}

	seedQuery := squirrel.Select(
		"Key_FAM",
		"Key_IND_Haupt",
		"Key_IND_Neben",
		"Key_ATC",
		"Key_ATCA").
		Distinct().
		From("FAM_DB").
		Where(squirrel.Eq{"Key_FAM": fams}).
		OrderBy("Key_FAM")
	query, args, _ := seedQuery.ToSql()

	var seeds []productIndicationSeed
	if err := db.Select(&seeds, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching product indication keys: %w", err)
	}

	keysByFAM, atcByFAM, directKeys, allATC := collectIndicationSeeds(seeds)

	relations, err := fetchIndicationRelations(slices.Collect(maps.Keys(directKeys)), db)
	if err != nil {
		return nil, err
	}
	expandIndicationRelations(keysByFAM, relations)

	allKeys := make(map[string]struct{})
	for _, keys := range keysByFAM {
		for key := range keys {
			allKeys[key] = struct{}{}
		}
	}

	namesByKey, err := fetchIndicationNames(slices.Collect(maps.Keys(allKeys)), lang, db)
	if err != nil {
		return nil, err
	}

	atcByCode := map[string]ProductATCCode{}
	if lang == "english" {
		var err error
		atcByCode, err = fetchATCMapping(slices.Collect(maps.Keys(allATC)), db)
		if err != nil {
			return nil, err
		}
	}

	for fam, keys := range keysByFAM {
		atcCodes := buildProductATCCodes(atcByFAM[fam], atcByCode)
		sortedKeys := slices.Collect(maps.Keys(keys))
		sort.Strings(sortedKeys)
		for _, key := range sortedKeys {
			name, exists := namesByKey[key]
			if !exists || name.Name == "" {
				continue
			}
			indicationsByFAM[fam] = append(indicationsByFAM[fam], ProductIndication{
				Name:     name.Name,
				Language: name.Language,
				ATCCodes: atcCodes,
			})
		}
	}

	return indicationsByFAM, nil
}

func collectIndicationSeeds(seeds []productIndicationSeed) (map[uint64]map[string]struct{}, map[uint64][]string, map[string]struct{}, map[string]struct{}) {
	keysByFAM := make(map[uint64]map[string]struct{}, len(seeds))
	atcByFAM := make(map[uint64][]string, len(seeds))
	directKeys := make(map[string]struct{})
	allATC := make(map[string]struct{})
	for _, seed := range seeds {
		if _, exists := keysByFAM[seed.KeyFAM]; !exists {
			keysByFAM[seed.KeyFAM] = map[string]struct{}{}
		}
		for _, key := range []*string{seed.KeyINDHaupt, seed.KeyINDNeben} {
			if key == nil || strings.TrimSpace(*key) == "" {
				continue
			}
			value := strings.TrimSpace(*key)
			keysByFAM[seed.KeyFAM][value] = struct{}{}
			directKeys[value] = struct{}{}
		}
		for _, code := range []*string{seed.KeyATC, seed.KeyATCA} {
			if code == nil || strings.TrimSpace(*code) == "" {
				continue
			}
			value := strings.ToUpper(strings.TrimSpace(*code))
			atcByFAM[seed.KeyFAM] = appendUniqueString(atcByFAM[seed.KeyFAM], value)
			for _, hierarchyCode := range atcHierarchyCodes(value) {
				allATC[hierarchyCode] = struct{}{}
			}
		}
	}

	return keysByFAM, atcByFAM, directKeys, allATC
}

func expandIndicationRelations(keysByFAM map[uint64]map[string]struct{}, relations []indicationRelation) {
	for _, relation := range relations {
		for fam, direct := range keysByFAM {
			for key := range direct {
				if relation.Source != nil && *relation.Source == key && relation.Target != nil {
					direct[strings.TrimSpace(*relation.Target)] = struct{}{}
				}
				if relation.Target != nil && *relation.Target == key && relation.Source != nil {
					direct[strings.TrimSpace(*relation.Source)] = struct{}{}
				}
			}
			keysByFAM[fam] = direct
		}
	}
}

func fetchIndicationRelations(keys []string, db *sqlx.DB) ([]indicationRelation, error) {
	if len(keys) == 0 {
		return []indicationRelation{}, nil
	}
	sort.Strings(keys)
	queryBuilder := squirrel.Select("Key_IND_Quelle", "Key_IND_Ziel").
		Distinct().
		From("INV_DB").
		Where(squirrel.Or{
			squirrel.Eq{"Key_IND_Quelle": keys},
			squirrel.Eq{"Key_IND_Ziel": keys},
		}).
		OrderBy("Key_IND_Quelle", "Key_IND_Ziel")
	query, args, _ := queryBuilder.ToSql()

	var rows []indicationRelation
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching indication hierarchy links: %w", err)
	}

	return rows, nil
}

type localizedIndicationName struct {
	Name     string
	Language string
}

func fetchIndicationNames(keys []string, lang string, db *sqlx.DB) (map[string]localizedIndicationName, error) {
	namesByKey := make(map[string]localizedIndicationName, len(keys))
	if len(keys) == 0 {
		return namesByKey, nil
	}
	sort.Strings(keys)
	queryBuilder := squirrel.Select("IND_DB.Key_IND")
	if lang == "english" {
		queryBuilder = queryBuilder.Columns(
			"COALESCE(NULLIF(TRANSLATION_INR_C.reviewed_name_en, ''), TRANSLATION_INR_C.name_en, INR_DB.Name, IND_DB.Name) AS indication_name",
			"CASE WHEN TRANSLATION_INR_C.name_en IS NULL THEN 'de' ELSE 'en' END AS indication_language")
	} else {
		queryBuilder = queryBuilder.Columns(
			"COALESCE(INR_DB.Name, IND_DB.Name) AS indication_name",
			"'de' AS indication_language")
	}
	queryBuilder = queryBuilder.
		Distinct().
		From("IND_DB").
		LeftJoin("INR_DB ON INR_DB.Key_IND = IND_DB.Key_IND").
		Where(squirrel.Eq{"IND_DB.Key_IND": keys}).
		OrderBy("IND_DB.Key_IND", "INR_DB.Zaehler")
	if lang == "english" {
		queryBuilder = queryBuilder.LeftJoin(
			"TRANSLATION_INR_C ON TRANSLATION_INR_C.key_ind = INR_DB.Key_IND " +
				"AND TRANSLATION_INR_C.zaehler = INR_DB.Zaehler " +
				"AND (TRANSLATION_INR_C.validation_status = 'valid' " +
				"OR (TRANSLATION_INR_C.validation_status = 'corrected' " +
				"AND NULLIF(TRANSLATION_INR_C.reviewed_name_en, '') IS NOT NULL)) " +
				"AND TRANSLATION_INR_C.review_source IS NOT NULL " +
				"AND TRANSLATION_INR_C.review_date IS NOT NULL")
	}
	query, args, _ := queryBuilder.ToSql()

	var rows []indicationNameRow
	if err := db.Select(&rows, query, args...); err != nil {
		if lang == "english" && isMissingINRTranslationTable(err) {
			return fetchIndicationNames(keys, "german", db)
		}
		return nil, fmt.Errorf("error fetching indication names: %w", err)
	}

	for _, row := range rows {
		if row.Name == nil {
			continue
		}
		if _, exists := namesByKey[row.KeyIND]; exists {
			continue
		}
		language := "de"
		if row.Language != nil && *row.Language == "en" {
			language = "en"
		}
		namesByKey[row.KeyIND] = localizedIndicationName{Name: *row.Name, Language: language}
	}

	return namesByKey, nil
}

func isMissingINRTranslationTable(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "translation_inr_c") &&
		(strings.Contains(message, "doesn't exist") || strings.Contains(message, "does not exist") ||
			strings.Contains(message, "no such table"))
}

func fetchATCMapping(codes []string, db *sqlx.DB) (map[string]ProductATCCode, error) {
	atcByCode := make(map[string]ProductATCCode, len(codes))
	if len(codes) == 0 {
		return atcByCode, nil
	}
	sort.Strings(codes)
	queryBuilder := squirrel.Select(
		"atc_code",
		"level",
		"label_en",
		"source_year",
		"source_url").
		From("who_atc_mapping").
		Where(squirrel.Eq{"atc_code": codes}).
		OrderBy("atc_code")
	query, args, _ := queryBuilder.ToSql()

	var rows []atcMappingRow
	if err := db.Select(&rows, query, args...); err != nil {
		if isMissingATCMappingTable(err) {
			return atcByCode, nil
		}
		return nil, fmt.Errorf("error fetching WHO ATC mapping: %w", err)
	}

	for _, row := range rows {
		code := ProductATCCode{
			Code:  row.Code,
			Level: row.Level,
		}
		if row.LabelEN != nil {
			code.LabelEN = *row.LabelEN
		}
		if row.SourceURL != nil {
			code.Source = *row.SourceURL
		}
		if code.Source == "" && row.SourceYear != nil {
			code.Source = fmt.Sprintf("WHO ATC/DDD Index %d", *row.SourceYear)
		}
		atcByCode[row.Code] = code
	}

	return atcByCode, nil
}

func buildProductATCCodes(codes []string, atcByCode map[string]ProductATCCode) []ProductATCCode {
	uniqueCodes := make(map[string]struct{}, len(codes)*5)
	for _, code := range codes {
		for _, hierarchyCode := range atcHierarchyCodes(code) {
			uniqueCodes[hierarchyCode] = struct{}{}
		}
	}

	hierarchyCodes := slices.Collect(maps.Keys(uniqueCodes))
	sort.Slice(hierarchyCodes, func(i, j int) bool {
		leftLevel := atcLevel(hierarchyCodes[i])
		rightLevel := atcLevel(hierarchyCodes[j])
		if leftLevel == rightLevel {
			return hierarchyCodes[i] < hierarchyCodes[j]
		}
		return leftLevel < rightLevel
	})

	result := make([]ProductATCCode, 0, len(hierarchyCodes))
	for _, code := range hierarchyCodes {
		if atc, exists := atcByCode[code]; exists {
			result = append(result, atc)
			continue
		}
		result = append(result, ProductATCCode{Code: code, Level: atcLevel(code)})
	}
	return result
}

func atcHierarchyCodes(code string) []string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return []string{}
	}

	lengths := []int{1, 3, 4, 5, 7}
	result := make([]string, 0, len(lengths))
	for _, length := range lengths {
		if length > len(code) {
			break
		}
		result = append(result, code[:length])
	}
	return result
}

func atcLevel(code string) int {
	switch len(strings.TrimSpace(code)) {
	case 1:
		return 1
	case 3:
		return 2
	case 4:
		return 3
	case 5:
		return 4
	case 7:
		return 5
	default:
		return 0
	}
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func isMissingATCMappingTable(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "doesn't exist") ||
		strings.Contains(message, "no such table") ||
		strings.Contains(message, "unknown table")
}

// STANDARD NOTES

// @Summary		List standard notes for PZNs
// @Description	Get Standardhinweise for one or more PZNs. Products without Standardhinweise are returned with an empty standard_notes array.
// @Tags			Product
// @Produce		json
// @Param			pzns	query	string					true	"Comma separated string of PZNs"	example:"1234567,7654321"
// @Param			lang	query	string					false	"Language for standard note categories and text: german or english (default: german)"
// @Success		200		{array}	ProductStandardNotes	"List of PZNs with standard notes"
// @Failure		400		"Bad request (e.g. invalid PZNs)"
// @Failure		404		"PZN(s) not found"
// @Failure		500		"Internal server error"
// @Router			/product/standardnotes/pzns [get]
//
// @Security		Bearer
func (pc *PZNController) GetStandardNotes(c *gin.Context) {
	type Query struct {
		PZNs string `form:"pzns" binding:"required" example:"1234567,7654321"`
		Lang string `form:"lang"`
	}

	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")
	lang, err := normalizeStandardNoteLang(query.Lang)
	if err != nil {
		handle.BadRequestError(c, err.Error())
		return
	}

	result, err := fetchStandardNotes(pzns, lang, pc.DB)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

type ProductStandardNotes struct {
	PZN           string              `json:"pzn"`
	ProductName   string              `json:"product_name"`
	StandardNotes []StandardNoteGroup `json:"standard_notes"`
}

type StandardNoteGroup struct {
	Category string         `json:"category"`
	Notes    []StandardNote `json:"notes"`
}

type StandardNote struct {
	PatientInfo bool   `json:"patient_info"`
	Text        string `json:"text"`
}

func fetchStandardNotesByFAM(fams []uint64, lang string, db *sqlx.DB) (map[uint64][]StandardNoteGroup, error) {
	notesByFAM := make(map[uint64][]StandardNoteGroup, len(fams))
	for _, fam := range fams {
		notesByFAM[fam] = []StandardNoteGroup{}
	}
	if len(fams) == 0 {
		return notesByFAM, nil
	}

	queryBuilder := squirrel.Select(
		"FAS_DB.Key_FAM",
		"STA_DB.Key_STA",
		"STA_DB.Patienteninfo",
		"STA_DB.Text AS standard_text").
		Distinct().
		From("FAS_DB").
		Join("STA_DB ON STA_DB.Key_STA = FAS_DB.Key_STA").
		Where(squirrel.Eq{"FAS_DB.Key_FAM": fams}).
		OrderBy("FAS_DB.Key_FAM", "STA_DB.Key_STA")

	query, args, _ := queryBuilder.ToSql()
	var rows []struct {
		KeyFAM      uint64  `db:"Key_FAM"`
		KeySTA      string  `db:"Key_STA"`
		PatientInfo *int    `db:"Patienteninfo"`
		Text        *string `db:"standard_text"`
	}

	if err := db.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching standard notes by product family: %w", err)
	}

	for _, row := range rows {
		if row.Text == nil {
			continue
		}

		patientInfo := false
		if row.PatientInfo != nil {
			patientInfo = *row.PatientInfo == 1
		}

		note := StandardNote{
			PatientInfo: patientInfo,
			Text:        translateStandardNoteText(row.KeySTA, *row.Text, lang),
		}
		notesByFAM[row.KeyFAM] = appendStandardNoteGroup(notesByFAM[row.KeyFAM], row.KeySTA, note, lang)
	}

	return notesByFAM, nil
}

func appendStandardNoteGroup(groups []StandardNoteGroup, keySTA string, note StandardNote, lang string) []StandardNoteGroup {
	categoryCode := standardNoteCategoryCode(keySTA)
	for i := range groups {
		if groups[i].Category == standardNoteCategoryLabel(categoryCode, lang) {
			groups[i].Notes = append(groups[i].Notes, note)
			return groups
		}
	}

	return append(groups, StandardNoteGroup{
		Category: standardNoteCategoryLabel(categoryCode, lang),
		Notes:    []StandardNote{note},
	})
}

func standardNoteCategoryCode(keySTA string) string {
	if keySTA == "" {
		return ""
	}

	return strings.ToUpper(keySTA[:1])
}

func standardNoteCategoryLabel(categoryCode string, lang string) string {
	switch categoryCode {
	case "A":
		if lang == "english" {
			return "Application and dosage"
		}
		return "Anwendung und Dosierung"
	case "H":
		if lang == "english" {
			return "Excipients"
		}
		return "Hilfsstoffe"
	case "L":
		if lang == "english" {
			return "Lactation"
		}
		return "Laktation"
	case "S":
		if lang == "english" {
			return "Pregnancy"
		}
		return "Schwangerschaft"
	case "W":
		if lang == "english" {
			return "General note or warning"
		}
		return "Allgemeiner Hinweis oder Warnhinweis"
	default:
		if lang == "english" {
			return "Unknown"
		}
		return "Unbekannt"
	}
}

func normalizeStandardNoteLang(lang string) (string, error) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return "german", nil
	}
	if lang == "de" {
		return "german", nil
	}
	if lang == "en" {
		return "english", nil
	}
	if lang != "german" && lang != "english" {
		return "", fmt.Errorf("invalid lang: must be german or english")
	}

	return lang, nil
}

func fetchStandardNotes(pzns []string, lang string, db *sqlx.DB) ([]ProductStandardNotes, error) {
	normalizedPZNs := make([]string, 0, len(pzns))
	for _, pzn := range pzns {
		pzn = strings.TrimSpace(pzn)
		if err := validate.PZN(pzn); err != nil {
			return nil, apierr.New(http.StatusBadRequest, fmt.Sprintf("Invalid PZN: %s", pzn))
		}
		normalizedPZNs = append(normalizedPZNs, pzn)
	}
	pzns = normalizedPZNs

	productQuery := squirrel.Select(
		"PAE_DB.PZN",
		"FAM_DB.Key_FAM",
		"FAM_DB.Produktname").
		Distinct().
		From("PAE_DB").
		Join("FAM_DB ON FAM_DB.Key_FAM = PAE_DB.Key_FAM").
		Where(squirrel.Eq{"PAE_DB.PZN": pzns}).
		OrderBy("PAE_DB.PZN")

	query, args, _ := productQuery.ToSql()
	var products []struct {
		PZN         string `db:"PZN"`
		KeyFAM      uint64 `db:"Key_FAM"`
		ProductName string `db:"Produktname"`
	}

	if err := db.Select(&products, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching products for standard notes: %w", err)
	}
	if len(products) == 0 {
		return nil, apierr.New(http.StatusNotFound, "PZN not found")
	}

	fams := make([]uint64, 0, len(products))
	for _, product := range products {
		fams = append(fams, product.KeyFAM)
	}
	standardNotesByFAM, err := fetchStandardNotesByFAM(fams, lang, db)
	if err != nil {
		return nil, err
	}

	resultByPZN := make(map[string]ProductStandardNotes, len(products))
	for _, product := range products {
		resultByPZN[product.PZN] = ProductStandardNotes{
			PZN:           product.PZN,
			ProductName:   product.ProductName,
			StandardNotes: standardNotesByFAM[product.KeyFAM],
		}
	}

	results := make([]ProductStandardNotes, 0, len(resultByPZN))
	for _, pzn := range pzns {
		pzn = strings.TrimSpace(pzn)
		if result, exists := resultByPZN[pzn]; exists {
			results = append(results, result)
		}
	}

	return results, nil
}
