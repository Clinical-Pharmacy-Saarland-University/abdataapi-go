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
	FrequencyTranslator func(*int, bool) *string
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
// @Description `german-simple` returns the simplified German ADR description.
// @Tags			Adverse Drug Reactions
// @Produce		json
// @Param			pzns	query	string	true	"Comma-separated list of PZNs"
// @Param			lang	query	string	false	"Language for ADR names (default: english)"	Enums(english,german,german-simple)
// @Success		200		{array}	PznADR	"List of PZNs with ADRs"
// @Failure		400		"Bad request (e.g. invalid PZNs)"
// @Failure		404		"PZN(s) not found"
// @Router			/adr [get]
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

	c.JSON(http.StatusOK, res)
}

type PznADR struct {
	PZN  string `json:"pzn"`
	ADRs []ADR  `json:"adrs"`
}

type ADR struct {
	KeyFAM       uint64  `db:"Key_FAM" json:"-"`
	FrequencyInt *int    `db:"Haeufigkeit" json:"frequency_code"`
	Frequency    *string `json:"frequency"`
	Descriptor   string  `db:"Name" json:"description"`
}

func fetchPznAdrs(pzns []string, db *sqlx.DB, ac *ADRController, lang string) ([]PznADR, error) {
	if err := validate.PZNs(pzns, 1, ac.Limits.InteractionDrugs); err != nil {
		return nil, apierr.New(http.StatusBadRequest, err.Error())
	}

	famPznMap, err := common.FamToPznMap(db, pzns)
	if err != nil {
		return nil, apierr.New(http.StatusInternalServerError, err.Error())
	}

	if diff := helper.SetDifference(pzns, slices.Collect(maps.Values(famPznMap))); len(diff) > 0 {
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

	pznFamMap := helper.SwapMap(famPznMap)
	pznAdrs := make([]PznADR, len(pzns))
	for i, pzn := range pzns {
		pznAdrs[i] = PznADR{
			PZN:  pzn,
			ADRs: famMap[pznFamMap[pzn]],
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

	en := lang == "english"
	for i := range adrs {
		adrs[i].Frequency = ac.FrequencyTranslator(adrs[i].FrequencyInt, en)
	}

	return adrs, nil
}
