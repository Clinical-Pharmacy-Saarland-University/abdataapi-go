package formulationcontroller

import (
	"net/http"
	"observeddb-go-api/internal/handle"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type FormulationController struct {
	DB *sqlx.DB
}

func NewFormulationController(resourceHandle *handle.ResourceHandle) *FormulationController {
	return &FormulationController{
		DB: resourceHandle.SQLX,
	}
}

// @Summary		List all drug formulation codes and their descriptions
// @Description	Drug formulation codes and their descriptions that are used in the database.
// @Description	These codes are used, e.g., in the compound interaction endpoint.
// @Tags			Formulation
// @Produce		json
// @Success		200	{object}	formulationcontroller.FormResponse	"Response with formulations"
// @Failure		500	{object}	handle.ErrorResponse				"Internal server error"
// @Router			/formulations [get]
// @Security		Bearer
func (fc *FormulationController) GetFormulations(c *gin.Context) {
	type Formulation struct {
		Formulation string `db:"Key_DAR" json:"formulation" example:"TAB"` // Formulation code
		Description string `db:"Name" json:"description" example:"Tablet"` // Formulation description
	} //	@name	Formulation
	db := fc.DB

	var formulations []Formulation
	err := db.Select(&formulations, "SELECT Key_DAR, Name FROM DAR_DB ORDER BY Key_DAR")
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	type Response struct {
		Formulations []Formulation `json:"formulations"`
	} //	@name	FormResponse
	c.JSON(http.StatusOK, Response{Formulations: formulations})
}
