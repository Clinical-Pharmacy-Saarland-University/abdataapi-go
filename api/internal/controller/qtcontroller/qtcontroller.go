package qtcontroller

import (
	"fmt"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/common"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/utils/apierr"
	"observeddb-go-api/internal/utils/format"
	"observeddb-go-api/internal/utils/validate"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type QTController struct {
	DB                 *sqlx.DB
	Limits             cfg.LimitsConfig
	CategoryTranslator func(*int, bool) *string
}

func NewQTController(resourceHandle *handle.ResourceHandle) *QTController {
	return &QTController{
		DB:                 resourceHandle.SQLX,
		Limits:             resourceHandle.Limits,
		CategoryTranslator: format.NewQTCategoryTranslator(),
	}
}

type QTResponse struct {
	PZN        string `json:"pzn"`
	QTCategory string `json:"qt_category"`
}

type QTCompoundResult struct {
	Input      string `json:"input"`
	QTCategory string `json:"qt_category"`
}

type QTRequest struct {
	ID   int      `json:"id"`
	PZNs []string `json:"pzns"`
}

// @Summary		List QT status for PZNs
// @Description	Get QT status for one or more PZNs. Each PZN can only have one QT status.
// @Tags			QT
// @Produce		json
// @Param			pzns	query	string		true	"Comma separated string of PZNs"	example:"1234567,7654321"
// @Success		200		{array}	QTResponse	"List of PZNs with QT status"
// @Failure		400		"Bad request (e.g. invalid PZNs)"
// @Failure		404		"PZN(s) not found"
// @Router			/qt/pzns [get]
func (qc *QTController) GetQTStatus(c *gin.Context) {
	type Query struct {
		PZNs string `form:"pzns" binding:"required" example:"1234567,7654321"`
	} //	@name	PZNPriscusQuery
	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")

	result, err := fetchQTStatus(pzns, qc.DB, qc.CategoryTranslator)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

// @Summary		List QT status for compounds
// @Description	Get QT status for one or more compound names. Matches are grouped like the compound search endpoint and include all related compounds sharing the same identifier.
// @Tags			QT
// @Produce		json
// @Param			compounds	query	string				true	"Comma-separated compound names (e.g., Metoprolol,Aspirin)"
// @Success		200			{array}	QTCompoundResult	"List of input compounds with QT status"
// @Failure		400			"Bad request (e.g. missing names or too many names)"
// @Failure		500			"Internal server error"
// @Router			/qt/compounds [get]
func (qc *QTController) GetQTStatusByCompound(c *gin.Context) {
	compoundsParam := c.Query("compounds")
	if compoundsParam == "" {
		handle.BadRequestError(c, "Missing required parameter: compounds")
		return
	}

	names := splitAndTrim(compoundsParam)
	if len(names) == 0 {
		handle.BadRequestError(c, "Missing required parameter: compounds")
		return
	}

	if len(names) > qc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", qc.Limits.BatchQueries))
		return
	}

	result, err := fetchQTStatusByCompound(names, qc.DB, qc.CategoryTranslator)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

func fetchQTStatus(pzns []string, db *sqlx.DB, translate func(*int, bool) *string) ([]QTResponse, error) {
	for _, pzn := range pzns {
		if err := validate.PZN(pzn); err != nil {
			return nil, apierr.New(http.StatusBadRequest, fmt.Sprintf("Invalid PZN: %s", pzn))
		}
	}

	queryBuilder := squirrel.Select("PAE_DB.PZN", "SZG_DB.Key_SGR").
		From("FAI_DB").
		LeftJoin("SZG_DB ON FAI_DB.Key_STO = SZG_DB.Key_STO").
		LeftJoin("PAE_DB ON PAE_DB.Key_FAM = FAI_DB.Key_FAM").
		Where(squirrel.Eq{"PAE_DB.PZN": pzns}).
		Where(squirrel.Eq{"Key_SGR": []int{10079780, 10079781, 10079782}})

	query, args, _ := queryBuilder.ToSql()
	var qtResults []struct {
		PZN    string `db:"PZN"`
		KeySGR *int   `db:"Key_SGR"`
	}

	if err := db.Select(&qtResults, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching QT status: %w", err)
	}

	qtMap := make(map[string]string)
	for _, result := range qtResults {
		if result.KeySGR != nil {
			qtMap[result.PZN] = *translate(result.KeySGR, false)
		}
	}

	responses := make([]QTResponse, len(pzns))
	for i, pzn := range pzns {
		category, exists := qtMap[pzn]
		if !exists {
			category = "unknown"
		}
		responses[i] = QTResponse{PZN: pzn, QTCategory: category}
	}

	return responses, nil
}

func fetchQTStatusByCompound(names []string, db *sqlx.DB, translate func(*int, bool) *string) ([]QTCompoundResult, error) {
	normalizedToInput := make(map[string]string, len(names))
	for _, name := range names {
		normalizedToInput[strings.ToLower(name)] = name
	}

	stoCompoundMap, err := common.StoToCompoundsMap(db, names)
	if err != nil {
		return nil, fmt.Errorf("error resolving QT compound STO keys: %w", err)
	}

	inputQTCategory := make(map[string]string, len(names))
	qtKeySTOs := make([]uint64, 0, len(stoCompoundMap))
	for qtKeySTO, compounds := range stoCompoundMap {
		qtKeySTOs = append(qtKeySTOs, qtKeySTO)
		for _, compound := range compounds {
			inputQTCategory[compound] = "unknown"
		}
	}

	qtCategories := make(map[uint64]string, len(qtKeySTOs))
	if len(qtKeySTOs) > 0 {
		qtQueryBuilder := squirrel.Select("Key_STO", "Key_SGR").
			From("SZG_DB").
			Where(squirrel.Eq{"Key_STO": qtKeySTOs}).
			Where(squirrel.Eq{"Key_SGR": []int{10079780, 10079781, 10079782}})

		qtQuery, qtArgs, _ := qtQueryBuilder.ToSql()
		var qtResults []struct {
			KeySTO uint64 `db:"Key_STO"`
			KeySGR int    `db:"Key_SGR"`
		}

		if err := db.Select(&qtResults, qtQuery, qtArgs...); err != nil {
			return nil, fmt.Errorf("error fetching QT categories for compounds: %w", err)
		}

		for _, qtResult := range qtResults {
			if category := translate(&qtResult.KeySGR, false); category != nil {
				qtCategories[qtResult.KeySTO] = *category
			}
		}
	}

	for qtKeySTO, compounds := range stoCompoundMap {
		category := qtCategories[qtKeySTO]
		if category == "" {
			category = "unknown"
		}
		for _, compound := range compounds {
			inputQTCategory[compound] = category
		}
	}

	results := make([]QTCompoundResult, 0, len(names))
	for _, name := range names {
		resultCategory := inputQTCategory[name]
		if resultCategory == "" {
			resultCategory = "unknown"
		}

		results = append(results, QTCompoundResult{
			Input:      name,
			QTCategory: resultCategory,
		})
	}

	return results, nil
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}
