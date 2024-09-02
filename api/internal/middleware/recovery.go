package middleware

import (
	"errors"
	"observeddb-go-api/internal/handle"

	"github.com/gin-gonic/gin"
)

func RecoveryHandler(c *gin.Context, err any) {
	realErr, ok := err.(error)
	if !ok {
		realErr = errors.New("unknown error")
	}

	handle.ServerError(c, realErr)
	c.Abort()
}
