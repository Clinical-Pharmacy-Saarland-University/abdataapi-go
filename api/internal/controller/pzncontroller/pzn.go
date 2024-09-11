package pzncontroller

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

type PZNController struct {
	DB     *sqlx.DB
	Limits cfg.LimitsConfig
}

func NewPZNController(resourceHandle *handle.ResourceHandle) *PZNController {
	return &PZNController{
		DB:     resourceHandle.SQLX,
		Limits: resourceHandle.Limits,
	}
}

func (pc *PZNController) GetActiveCompounds(c *gin.Context) {
	pzn := c.Param("pzn")
	result, err := fetchActiveCompounds(pzn, pc.DB)
	if err != nil {
		handle.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"active_compounds": result})
}

type CompoundName struct {
	Name      string   `json:"name"`
	Preferred bool     `json:"preferred"`
	Standard  []string `json:"standards"`
}

type Compound struct {
	Names []CompoundName `json:"compound"`
}

func fetchActiveCompounds(pzn string, db *sqlx.DB) ([]Compound, error) {
	if err := validate.PZN(pzn); err != nil {
		return nil, apierr.New(http.StatusBadRequest, err.Error())
	}

	queryBuilder := squirrel.Select(
		"Name",
		"Herkunft",
		"Vorzugsbezeichnung",
		"FAI_DB.Key_STO").
		From("PAE_DB").
		Distinct().
		RightJoin("FAI_DB ON PAE_DB.Key_FAM = FAI_DB.Key_FAM").
		LeftJoin("VSS_DB ON VSS_DB.Key_STO_2 = FAI_DB.Key_STO").
		LeftJoin("SNA_DB ON FAI_DB.Key_STO = SNA_DB.Key_STO").
		Where(squirrel.Eq{"PZN": pzn}).
		Where(squirrel.Eq{"Stofftyp": 1}).
		Where(squirrel.Or{squirrel.Eq{"Typ": nil}, squirrel.NotEq{"Typ": 100}}).
		Where("FAI_DB.Key_STO NOT IN (SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8)").
		OrderBy("FAI_DB.Key_STO")
	query, args, _ := queryBuilder.ToSql()
	var dbResults []struct {
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

	compounds := []Compound{}
	lastSto := uint64(0)
	lastCompound := Compound{}
	for _, dbResult := range dbResults {
		if dbResult.KeySTO != lastSto && lastSto != 0 {
			compounds = append(compounds, lastCompound)
			lastCompound = Compound{}
		}

		std := []string{}
		if dbResult.Standard != nil {
			std = strings.Split(*dbResult.Standard, ";")
		}
		cn := CompoundName{
			Name:      dbResult.Name,
			Preferred: dbResult.Preferred,
			Standard:  std,
		}
		lastCompound.Names = append(lastCompound.Names, cn)
		lastSto = dbResult.KeySTO
	}
	compounds = append(compounds, lastCompound)

	return compounds, nil
}
