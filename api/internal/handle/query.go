package handle

import (
	"errors"
	"net/http"
	"observeddb-go-api/internal/utils/apierr"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// JSONBind binds the JSON body to the query struct and handles any errors.
func JSONBind(c *gin.Context, query interface{}) bool {
	if err := c.ShouldBindJSON(query); err != nil {
		handleBindErrors(c, err, query)
		return false
	}
	return true
}

// QueryBind binds the query parameters to the query struct and handles any errors.
func QueryBind(c *gin.Context, query interface{}) bool {
	if err := c.ShouldBindQuery(query); err != nil {
		handleBindErrors(c, err, query)
		return false
	}
	return true
}

func handleBindErrors(c *gin.Context, err error, query interface{}) {
	var verr validator.ValidationErrors
	if errors.As(err, &verr) {
		ferrs := apierr.ValidationErrors(verr, query)
		c.JSON(http.StatusBadRequest, gin.H{"errors": ferrs})
		return
	}
	BadRequestError(c, err.Error())
}
