package middleware

import (
	"errors"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/utils/tokens"
	"strings"

	"github.com/gin-gonic/gin"
)

func Authentication(authCfg *cfg.AuthTokenConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			return
		}

		token, err := extractToken(authHeader)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Bearer format"})
			return
		}

		claims, err := tokens.CheckAccessToken(token, authCfg)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid access token"})
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
