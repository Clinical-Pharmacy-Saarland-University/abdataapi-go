package middleware

import (
	"net/http"
	"observeddb-go-api/internal/utils/validate"

	"github.com/gin-gonic/gin"
)

func AdminAccess() gin.HandlerFunc {
	return roleMiddleware("admin")
}

func ApproverAccess() gin.HandlerFunc {
	return roleMiddleware("approver")
}

func roleMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := c.GetString("user_role")

		if err := validate.Access(requiredRole, userRole); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Unauthorized access"})
			return
		}

		c.Next()
	}
}
