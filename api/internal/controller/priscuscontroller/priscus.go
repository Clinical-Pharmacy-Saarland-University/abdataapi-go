package priscuscontroller

import (
	"fmt"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/utils/apierr"
	"observeddb-go-api/internal/utils/validate"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type PriscusController struct {
	DB     *sqlx.DB
	Limits cfg.LimitsConfig
}

func NewPriscusController(resourceHandle *handle.ResourceHandle) *PriscusController {
	return &PriscusController{
		DB:     resourceHandle.SQLX,
		Limits: resourceHandle.Limits,
	}
}

type PriscusResponse struct {
	PZN       string `json:"PZN"`
	IsPriscus bool   `json:"priscus"`
}

// @Summary		List Priscus status for PZNs
// @Description	Get Priscus status for one or more PZNs. Each PZN can only have one priscus status.
// @Tags			Priscus
// @Produce		json
// @Param			pzns	query	string			true	"Comma separated string of PZNs"	example:"1234567,7654321"
// @Success		200		{array}	PriscusResponse	"List of PZNs with Priscus status"
// @Failure		400		"Bad request (e.g. invalid PZNs)"
// @Failure		404		"PZN(s) not found"
// @Router			/priscus/pzns [get]
//
// @Security		Bearer
func (pc *PriscusController) GetPriscusStatus(c *gin.Context) {
	type Query struct {
		PZNs string `form:"pzns" binding:"required" example:"1234567,7654321"`
	} //	@name	PZNPriscusQuery
	var query Query
	if !handle.QueryBind(c, &query) {
		return
	}

	pzns := strings.Split(query.PZNs, ",")

	result, err := fetchPriscusStatus(pzns, pc.DB)
	if err != nil {
		handle.Error(c, err)
		return
	}

	handle.Success(c, result)
}

func fetchPriscusStatus(pzns []string, db *sqlx.DB) ([]PriscusResponse, error) {
	for _, pzn := range pzns {
		if err := validate.PZN(pzn); err != nil {
			return nil, apierr.New(http.StatusBadRequest, fmt.Sprintf("Invalid PZN: %s", pzn))
		}
	}

	queryBuilder := squirrel.Select("PAE_DB.PZN", "FAI_DB.Key_STO").
		From("FAI_DB").
		LeftJoin("SZG_DB ON FAI_DB.Key_STO = SZG_DB.Key_STO").
		LeftJoin("PAE_DB ON PAE_DB.Key_FAM = FAI_DB.Key_FAM").
		Where(squirrel.Eq{"PAE_DB.PZN": pzns}).
		Where(squirrel.Eq{"Key_SGR": "10084520"})

	query, args, _ := queryBuilder.ToSql()
	var priscusResults []struct {
		PZN    string `db:"PZN"`
		KeySTO uint64 `db:"Key_STO"`
	}

	if err := db.Select(&priscusResults, query, args...); err != nil {
		return nil, fmt.Errorf("error fetching priscus status: %w", err)
	}

	priscusMap := make(map[string]bool)
	for _, result := range priscusResults {
		priscusMap[result.PZN] = true
	}

	responses := make([]PriscusResponse, len(pzns))
	for i, pzn := range pzns {
		responses[i] = PriscusResponse{PZN: pzn, IsPriscus: priscusMap[pzn]}
	}

	return responses, nil
}
