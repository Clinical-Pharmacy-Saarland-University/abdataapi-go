package pzncontroller

import (
	"fmt"
	"maps"
	"net/http"
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

func (pc *PZNController) GetProductList(c *gin.Context) {

	const pageSize = 1000

	var params struct {
		Page int `form:"page"`
	}

	if !handle.QueryBind(c, &params) {
		return
	}

	// Count the total number of distinct PZNs
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
		return
	}

	pages := (cResult.Count + pageSize - 1) / pageSize

	if params.Page <= 0 {
		handle.BadRequestError(c, "Page must be a positive integer")
		return
	}

	if params.Page > pages {
		handle.NotFoundError(c, fmt.Sprintf("Page %d not found, total pages: %d", params.Page, pages))
		return
	}

	queryPzns := fmt.Sprintf(`
		SELECT DISTINCT PZN
		FROM PAE_DB 
		LEFT JOIN FAM_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM 
		WHERE PZN IS NOT NULL 
		AND KEY_ATC IS NOT NULL 
		AND FAM_DB.Veterinaerpraeparat = 0
		ORDER BY PZN
		LIMIT %d OFFSET %d`, pageSize, (params.Page-1)*pageSize)

	var pznsRes []string
	err = pc.DB.Select(&pznsRes, queryPzns)
	if err != nil {
		handle.Error(c, err)
		return
	}
	if len(pznsRes) == 0 {
		handle.NotFoundError(c, fmt.Sprintf("No PZNs found for page %d", params.Page))
		return
	}

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
		return
	}

	type dbResult struct {
		ProductName string  `db:"Produktname" json:"product"`
		ATC         string  `db:"Key_ATC" json:"atc"`
		PZN         string  `db:"PZN" json:"pzn"`
		KeySTO      string  `db:"Key_STO" json:"key_sto"`
		KeySTO1     *string `db:"Key_STO_1" json:"key_sto_1"`
		KeySTO2     *string `db:"Key_STO_2" json:"key_sto_2"`
		Typ         *int    `db:"Typ" json:"typ"`
		Name        string  `db:"Name" json:"name"`
	}

	var dbResults []dbResult
	err = pc.DB.Select(&dbResults, query, args...)
	if err != nil {
		handle.Error(c, err)
		return
	}

	// We have now ordered by PZN but have to eliminate Key_STOs that are also in Key_STO_1
	type result struct {
		ProductName     string   `json:"product"`
		ATC             string   `json:"atc"`
		PZN             *string  `json:"pzn"`
		ActiveCompounds []string `json:"active_compounds"`
	}

	var pznResults []dbResult
	var results []result
	for _, res := range dbResults {
		if len(pznResults) == 0 || pznResults[0].PZN == res.PZN {
			pznResults = append(pznResults, res)
			continue
		}

		if pznResults[0].PZN != res.PZN {
			r := result{
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
			pznResults = []dbResult{res}
			continue
		}
	}

	data := struct {
		Pages      int `json:"pages"`
		Page       int `json:"page"`
		PZNPerPage int `json:"pzns_per_page"`
		// Data contains the list of products with their active compounds
		Data []result `json:"products"`
	}{
		Pages:      pages,
		Page:       params.Page,
		PZNPerPage: pageSize,
		Data:       results,
	}

	handle.Success(c, data)
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
		categoryInt := int(dbResult.Category) // Convert uint64 to int for translation
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
