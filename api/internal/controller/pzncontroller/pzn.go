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
// @Description	Fuzzy-search products by one or more product names and return matching PZNs with active compounds grouped by input.
// @Tags			Product
// @Produce		json
// @Param			name	query	string					true	"Comma-separated product name search terms; encode literal commas as %2C"	example:"Delix+2%2C5,Plavix,Ramilich"
// @Param			limit	query	int						false	"Maximum number of products to return per input (default: 20, max: 100)"
// @Success		200		{array}	ProductSearchGroup	"Matching products with active compounds grouped by input"
// @Failure		400		"Bad request (e.g. missing name)"
// @Failure		500		"Internal server error"
// @Router			/product/search [get]
//
// @Security		Bearer
func (pc *PZNController) GetProductSearch(c *gin.Context) {
	var query struct {
		Limit int `form:"limit"`
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

	result, err := fetchProductsByNames(names, limit, pc.DB)
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
	ProductName     string   `json:"product_name"`
	PZN             string   `json:"pzn"`
	ActiveCompounds []string `json:"active_compounds"`
}

func fetchProductsByNames(names []string, limit int, db *sqlx.DB) ([]ProductSearchGroup, error) {
	results := make([]ProductSearchGroup, 0, len(names))
	for _, name := range names {
		products, err := fetchProductsByName(name, limit, db)
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

func fetchProductsByName(name string, limit int, db *sqlx.DB) ([]ProductSearchResult, error) {
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

	results := make([]ProductSearchResult, 0, len(products))
	for _, product := range products {
		results = append(results, ProductSearchResult{
			ProductName:     product.ProductName,
			PZN:             product.PZN,
			ActiveCompounds: compoundsByFAM[product.KeyFAM],
		})
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
// @Description	Get product info (is_combination, category) for one or more PZNs.
// @Tags			Product
// @Produce		json
// @Param			pzns	query	string			true	"Comma separated string of PZNs"	example:"1234567,7654321"
// @Success		200		{array}	ProductInfos	"List of PZNs with product info"
// @Failure		400		"Bad request (e.g. invalid PZNs)"
// @Failure		404		"PZN(s) not found"
// @Router			/product/info/pzns [get]
//
// @Security		Bearer
func (pc *PZNController) GetProductInfo(c *gin.Context) {
	type Query struct {
		PZNs string `form:"pzns" binding:"required" example:"1234567,7654321"`
	} //	@name	PZNActiveCompoundsQuery

	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")

	result, err := fetchProductInfo(pzns, pc.DB, pc.CategoryTranslator)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

// ProductInfo represents information about a product
type ProductInfo struct {
	PZN         string `json:"pzn"`
	IsComb      bool   `json:"is_combination"`
	Category    string `json:"category"`
	ProductName string `json:"product_name"`
}

// ProductInfos represents a list of product info for multiple PZNs
type ProductInfos struct {
	ProductInfoList []ProductInfo `json:"product_info"`
}

func fetchProductInfo(pzns []string, db *sqlx.DB, translate func(*int, bool) *string) ([]ProductInfo, error) {
	for _, pzn := range pzns {
		if err := validate.PZN(pzn); err != nil {
			return nil, apierr.New(http.StatusBadRequest, fmt.Sprintf("Invalid PZN: %s", pzn))
		}
	}

	queryBuilder := squirrel.Select(
		"PAE_DB.PZN",
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

	// ✅ Convert results into []ProductInfo
	var productInfos []ProductInfo
	for _, dbResult := range dbResults {
		// Category is a small ABDA Produktgruppe enum code, so the uint64->int conversion cannot overflow.
		categoryInt := int(dbResult.Category) //nolint:gosec // Produktgruppe is a small enum code.
		categoryName := translate(&categoryInt, false)

		productInfos = append(productInfos, ProductInfo{
			PZN:         dbResult.PZN,
			IsComb:      dbResult.Monop == 0,
			Category:    *categoryName, // Use translated category name
			ProductName: dbResult.PName,
		})
	}

	return productInfos, nil
}
