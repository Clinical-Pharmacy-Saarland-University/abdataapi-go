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

func (fc *FormulationController) GetFormulations(c *gin.Context) {
	type Formulation struct {
		Formulation string `db:"Key_DAR" json:"formulation"`
		Description string `db:"Name" json:"description"`
	}
	db := fc.DB

	var formulations []Formulation
	err := db.Select(&formulations, "SELECT Key_DAR, Name FROM DAR_DB ORDER BY Key_DAR")
	if err != nil {
		handle.ServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"formulations": formulations})
}
