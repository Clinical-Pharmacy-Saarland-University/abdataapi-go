package middleware

import (
	"errors"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/utils/tokens"
	"strings"

	"github.com/gin-gonic/gin"
)

func Authentication(authCfg *cfg.AuthTokenConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			handle.UnauthorizedError(c, "Authorization header is required")
			c.Abort()
			return
		}

		token, err := extractToken(authHeader)
		if err != nil {
			handle.UnauthorizedError(c, "Invalid Bearer format")
			c.Abort()
			return
		}

		claims, err := tokens.CheckAccessToken(token, authCfg)
		if err != nil {
			handle.UnauthorizedError(c, "Invalid access token")
			c.Abort()
			return
		}

		c.Set("user_id", claims.ID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)
		c.Next()
	}
}

func extractToken(authHeader string) (string, error) {
	splitToken := strings.Split(authHeader, "Bearer")
	if len(splitToken) != 2 {
		return "", errors.New("invalid Bearer format")
	}
	return strings.TrimSpace(splitToken[1]), nil
}
