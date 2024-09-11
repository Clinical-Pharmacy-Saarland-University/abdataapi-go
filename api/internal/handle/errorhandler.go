package handle

import (
	"errors"
	"net/http"
	"observeddb-go-api/internal/utils/apierr"

	"github.com/gin-gonic/gin"
)

func ServerError(c *gin.Context, err error) {
	Error(c, apierr.New(http.StatusInternalServerError, err.Error()))
}

func UnauthorizedError(c *gin.Context) {
	Error(c, apierr.New(http.StatusUnauthorized, "Invalid credentials"))
}

func ForbiddenError(c *gin.Context, msg string) {
	Error(c, apierr.New(http.StatusForbidden, msg))
}

func BadRequestError(c *gin.Context, msg string) {
	Error(c, apierr.New(http.StatusBadRequest, msg))
}

func NotFoundError(c *gin.Context, msg string) {
	Error(c, apierr.New(http.StatusNotFound, msg))
}

type ErrorResponse struct {
	Error string `json:"error" example:"Internal server error"` // Error message
} //	@name	ErrorResponse

func Error(c *gin.Context, err error) {
	var apiErr apierr.Error
	if !errors.As(err, &apiErr) {
		apiErr = apierr.New(http.StatusInternalServerError, err.Error())
	}

	apiErr.Log(c)
	c.JSON(apiErr.Status(), ErrorResponse{Error: apiErr.Message()})
}
