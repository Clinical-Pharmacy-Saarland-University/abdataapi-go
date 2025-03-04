package compoundcontroller

import (
	"fmt"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type CompoundController struct {
	DB     *sqlx.DB
	Limits cfg.LimitsConfig
}

func NewCompoundController(resourceHandle *handle.ResourceHandle) *CompoundController {
	return &CompoundController{
		DB:     resourceHandle.SQLX,
		Limits: resourceHandle.Limits,
	}
}

type CompoundResponse struct {
	Name      string   `json:"name"`
	Standard  []string `json:"standards"`
	Preferred bool     `json:"preferred"`
}

// @name	CompoundSelectionQuery
type CompoundSelectionQuery struct {
	Names []string `json:"names" binding:"required" example:"Metoprolol,Aspirin"`
}

// @Summary		Get compounds by name
// @Description	Retrieves compounds by name and includes all related compounds sharing the same identifier.
// @Tags			compounds
// @Accept			json
// @Produce		json
// @Param			names	query	string	true	"Comma-separated compound names (e.g., Metoprolol,Aspirin)"
// @Success		200		{array}	object	"Successful response"
// @Failure		400		"Invalid request format or too many names provided"
// @Failure		500		"Internal server error"
// @Router			/compounds/names [get]
func (cc *CompoundController) GetSelectCompounds(c *gin.Context) {
	// Extract query parameters
	namesParam := c.Query("names")
	if namesParam == "" {
		handle.BadRequestError(c, "Missing required parameter: names")
		return
	}

	// Convert comma-separated values into a slice
	names := strings.Split(namesParam, ",")
	if len(names) > cc.Limits.BatchQueries {
		handle.BadRequestError(c, fmt.Sprintf("Too many names provided. Maximum is %d", cc.Limits.BatchQueries))
		return
	}

	db := cc.DB
	formattedResults := []map[string]interface{}{}

	// Query each name separately
	for _, name := range names {
		// Query to fetch Key_STO values for the given name
		keyStoQuery, keyStoArgs, _ := squirrel.Select("DISTINCT Name", "Key_STO").
			From("SNA_DB").
			Where(squirrel.Expr("Name LIKE ?", "%"+name+"%")).
			ToSql()

		var keyStoResults []struct {
			Name   string `db:"Name"`
			KeySTO uint64 `db:"Key_STO"`
		}

		err := db.Select(&keyStoResults, keyStoQuery, keyStoArgs...)
		if err != nil {
			handle.Error(c, fmt.Errorf("database query for Key_STO failed: %w", err))
			return
		}

		// If no matches found, return an empty array for this input
		if len(keyStoResults) == 0 {
			formattedResults = append(formattedResults, map[string]interface{}{
				"input":   name,
				"matches": []interface{}{},
			})
			continue
		}

		// Fetch compounds using Key_STO values
		var allKeySTOs []uint64
		for _, row := range keyStoResults {
			allKeySTOs = append(allKeySTOs, row.KeySTO)
		}

		var dbResults []struct {
			Name      string  `db:"Name"`
			Standard  *string `db:"Herkunft"`
			Preferred int     `db:"Vorzugsbezeichnung"`
			KeySTO    uint64  `db:"Key_STO"`
		}

		compoundsQuery, compoundsArgs, _ := squirrel.Select("Name", "Herkunft", "Vorzugsbezeichnung", "Key_STO").
			From("SNA_DB").
			Where(squirrel.Eq{"Key_STO": allKeySTOs}).
			ToSql()

		err = db.Select(&dbResults, compoundsQuery, compoundsArgs...)
		if err != nil {
			handle.Error(c, fmt.Errorf("database query for compounds failed: %w", err))
			return
		}

		// Organize results by Key_STO
		matches := [][]CompoundResponse{}
		seenKeySTOs := make(map[uint64]bool)

		for _, keyRow := range keyStoResults {
			if seenKeySTOs[keyRow.KeySTO] {
				continue // Avoid duplicates
			}
			seenKeySTOs[keyRow.KeySTO] = true

			matchesForKeySTO := []CompoundResponse{}
			for _, row := range dbResults {
				if keyRow.KeySTO == row.KeySTO {
					std := []string{}
					if row.Standard != nil {
						std = strings.Split(*row.Standard, ";")
					}

					compoundEntry := CompoundResponse{
						Name:      row.Name,
						Standard:  std,
						Preferred: row.Preferred == 1,
					}

					matchesForKeySTO = append(matchesForKeySTO, compoundEntry)
				}
			}

			if len(matchesForKeySTO) > 0 {
				matches = append(matches, matchesForKeySTO)
			}
		}

		// Store results
		formattedResults = append(formattedResults, map[string]interface{}{
			"input":   name,
			"matches": matches, // Nested array of matches, NO duplicates
		})
	}

	handle.Success(c, formattedResults)
}
