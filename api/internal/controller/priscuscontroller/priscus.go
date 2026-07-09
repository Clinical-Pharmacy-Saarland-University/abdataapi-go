package priscuscontroller

import (
	"fmt"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/common"
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

type PriscusCompoundResult struct {
	Input     string `json:"input"`
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

// @Summary		List Priscus status for compounds
// @Description	Get Priscus status for one or more compound names.
// @Description	Provide the list as repeated params (preferred, preserves names containing commas) or as a single legacy comma-joined value.
// @Tags			Priscus
// @Produce		json
// @Param			compounds	query	[]string				true	"Compound names (e.g., Metoprolol,Aspirin)"	collectionFormat(multi)
// @Success		200			{array}	PriscusCompoundResult	"List of input compounds with Priscus status"
// @Failure		400			"Bad request (e.g. missing compounds or too many names)"
// @Failure		500			"Internal server error"
// @Router			/priscus/compounds [get]
//
// @Security		Bearer
func (pc *PriscusController) GetPriscusStatusByCompound(c *gin.Context) {
	compounds := splitAndTrim(handle.NormalizeList(c.QueryArray("compounds")))
	if len(compounds) == 0 {
		handle.BadRequestError(c, "Missing required parameter: compounds")
		return
	}

	if len(compounds) > pc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", pc.Limits.BatchQueries))
		return
	}

	result, err := fetchPriscusStatusByCompound(compounds, pc.DB)
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

func fetchPriscusStatusByCompound(compounds []string, db *sqlx.DB) ([]PriscusCompoundResult, error) {
	stoCompoundMap, err := common.StoToCompoundsMap(db, compounds)
	if err != nil {
		return nil, fmt.Errorf("error resolving priscus compound STO keys: %w", err)
	}

	keySTOs := make([]uint64, 0, len(stoCompoundMap))
	inputPriscus := make(map[string]bool, len(compounds))
	for keySTO, matchedCompounds := range stoCompoundMap {
		keySTOs = append(keySTOs, keySTO)
		for _, compound := range matchedCompounds {
			inputPriscus[compound] = false
		}
	}

	priscusSTOs := make(map[uint64]bool, len(keySTOs))
	if len(keySTOs) > 0 {
		queryBuilder := squirrel.Select("Key_STO").
			From("SZG_DB").
			Where(squirrel.Eq{"Key_STO": keySTOs}).
			Where(squirrel.Eq{"Key_SGR": "10084520"})

		query, args, _ := queryBuilder.ToSql()
		var results []struct {
			KeySTO uint64 `db:"Key_STO"`
		}

		if selErr := db.Select(&results, query, args...); selErr != nil {
			return nil, fmt.Errorf("error fetching priscus status for compounds: %w", selErr)
		}

		for _, result := range results {
			priscusSTOs[result.KeySTO] = true
		}
	}

	for keySTO, matchedCompounds := range stoCompoundMap {
		isPriscus := priscusSTOs[keySTO]
		for _, compound := range matchedCompounds {
			inputPriscus[compound] = isPriscus
		}
	}

	response := make([]PriscusCompoundResult, 0, len(compounds))
	for _, compound := range compounds {
		response = append(response, PriscusCompoundResult{
			Input:     compound,
			IsPriscus: inputPriscus[compound],
		})
	}

	return response, nil
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
