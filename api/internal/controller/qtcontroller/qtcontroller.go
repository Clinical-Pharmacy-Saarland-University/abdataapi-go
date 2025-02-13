package qtcontroller

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

type QTRequest struct {
	ID   int      `json:"id"`
	PZNs []string `json:"pzns"`
}

//	@Summary		List QT status for PZNs
//	@Description	Get QT status for one or more PZNs. Each PZN can only have one QT status.
//	@Tags			QT
//	@Produce		json
//	@Param			pzns	query	string		true	"Comma separated string of PZNs"	example:"1234567,7654321"
//	@Success		200		{array}	QTResponse	"List of PZNs with QT status"
//	@Failure		400		"Bad request (e.g. invalid PZNs)"
//	@Failure		404		"PZN(s) not found"
//	@Router			/qt/pzns [get]
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

	c.JSON(http.StatusOK, result)
}

//	@Summary		List QT status for PZNs via POST request
//	@Description	Retrieve QT status for multiple sets of PZNs.
//	@Tags			QT
//	@Accept			json
//	@Produce		json
//	@Param			body	body	[]QTRequest	true	"Array of ID and PZN lists"
//	@Success		200		{array}	QTResponse	"List of PZNs with QT status"
//	@Failure		400		"Bad request (e.g. invalid PZNs)"
//	@Failure		404		"PZN(s) not found"
//	@Router			/qt/pzns [post]
func (qc *QTController) PostQTStatus(c *gin.Context) {
	var requests []QTRequest
	if err := c.BindJSON(&requests); err != nil {
		handle.Error(c, apierr.New(http.StatusBadRequest, "Invalid request body"))
		return
	}

	var allPZNs []string
	for _, req := range requests {
		allPZNs = append(allPZNs, req.PZNs...)
	}

	result, err := fetchQTStatus(allPZNs, qc.DB, qc.CategoryTranslator)
	if err != nil {
		handle.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
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
