package compoundcontroller

import (
	"fmt"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"regexp"
	"strings"

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

	// Construct the MATCH query input
	matchQueryInput := "\"" + strings.Join(names, "\" \"") + "\""

	// Query to fetch matching compounds using FULLTEXT search from the input column
	compoundsQuery := `
		SELECT a.Name, a.Herkunft, a.Vorzugsbezeichnung, a.Key_STO, b.Name AS Input
		FROM SNA_DB a
		JOIN SNA_DB b ON a.Key_STO = b.Key_STO
		WHERE MATCH(b.Name) AGAINST (? IN BOOLEAN MODE)
	`

	var dbResults []struct {
		Name      string  `db:"Name"`
		Standard  *string `db:"Herkunft"`
		Preferred int     `db:"Vorzugsbezeichnung"`
		KeySTO    uint64  `db:"Key_STO"`
		Input     string  `db:"Input"`
	}

	err := db.Select(&dbResults, compoundsQuery, matchQueryInput)
	if err != nil {
		handle.Error(c, fmt.Errorf("database query for compounds failed: %w", err))
		return
	}

	// Organize results by input name using regex matching instead of exact input comparison
	matchesByInput := make(map[string]map[uint64][]CompoundResponse)
	for _, name := range names {
		matchesByInput[name] = make(map[uint64][]CompoundResponse)
	}

	for _, row := range dbResults {
		std := []string{}
		if row.Standard != nil {
			std = strings.Split(*row.Standard, ";")
		}

		compoundEntry := CompoundResponse{
			Name:      row.Name,
			Standard:  std,
			Preferred: row.Preferred == 1,
		}

		// Use regex to match input names against the returned Input column
		for _, name := range names {
			matched, _ := regexp.MatchString("(?i)"+regexp.QuoteMeta(name), row.Input)
			if matched {
				if _, exists := matchesByInput[name]; !exists {
					matchesByInput[name] = make(map[uint64][]CompoundResponse)
				}
				if _, exists := matchesByInput[name][row.KeySTO]; !exists {
					matchesByInput[name][row.KeySTO] = []CompoundResponse{}
				}
				matchesByInput[name][row.KeySTO] = append(matchesByInput[name][row.KeySTO], compoundEntry)
			}
		}
	}

	// Format final results with nested match arrays
	for _, name := range names {
		groupedMatches := [][]CompoundResponse{}
		for _, compounds := range matchesByInput[name] {
			groupedMatches = append(groupedMatches, compounds)
		}
		formattedResults = append(formattedResults, map[string]interface{}{
			"input":   name,
			"matches": groupedMatches,
		})
	}

	handle.Success(c, formattedResults)
}
