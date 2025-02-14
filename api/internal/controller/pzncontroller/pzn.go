package pzncontroller

import (
	"fmt"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/utils/apierr"
	"observeddb-go-api/internal/utils/format"
	"observeddb-go-api/internal/utils/validate"
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

// @Summary		List active compounds for PZNs
// @Description	Get active compounds for one or more PZNs. Each PZN can only have multiple active compounds.
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

	c.JSON(http.StatusOK, gin.H{"active_compounds": result})
}

type CompoundName struct {
	PZN       string   `json:"pzn"`
	Name      string   `json:"name"`
	Preferred bool     `json:"preferred"`
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
		Where(squirrel.Or{squirrel.Eq{"Typ": nil}, squirrel.NotEq{"Typ": 100}}).
		Where("FAI_DB.Key_STO NOT IN (SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8)").
		OrderBy("PAE_DB.PZN, FAI_DB.Key_STO")

	query, args, _ := queryBuilder.ToSql()

	var dbResults []struct {
		PZN       string  `db:"PZN"`
		Name      string  `db:"Name"`
		Preferred bool    `db:"Vorzugsbezeichnung"`
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

		cn := CompoundName{
			PZN:       dbResult.PZN,
			Name:      dbResult.Name,
			Preferred: dbResult.Preferred,
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

	c.JSON(http.StatusOK, gin.H{"product_info": result})
}

// ProductInfo represents information about a product
type ProductInfo struct {
	PZN      string `json:"pzn"`
	IsComb   bool   `json:"is_combination"`
	Category string `json:"category"`
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
		"FAM_DB.Monopraeparat").
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
			PZN:      dbResult.PZN,
			IsComb:   dbResult.Monop == 0,
			Category: *categoryName, // Use translated category name
		})
	}

	return productInfos, nil
}
