package http

import (
	"SWPUCAT/internal/infrastructure/auth"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserID   = "user_id"
	ContextUsername = "username"
	ContextRole     = "role"
)

func JWTAuthMiddleware(jwtSvc *auth.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, Response{Code: 401, Message: "missing authorization header"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, Response{Code: 401, Message: "invalid authorization format"})
			c.Abort()
			return
		}

		userID, username, role, err := jwtSvc.ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, Response{Code: 401, Message: "invalid or expired token"})
			c.Abort()
			return
		}

		c.Set(ContextUserID, userID)
		c.Set(ContextUsername, username)
		c.Set(ContextRole, role)
		c.Next()
	}
}

func RBACMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(ContextRole)
		if !exists {
			c.JSON(http.StatusForbidden, Response{Code: 403, Message: "role not found"})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(http.StatusForbidden, Response{Code: 403, Message: "invalid role"})
			c.Abort()
			return
		}

		for _, allowed := range allowedRoles {
			if roleStr == allowed {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, Response{Code: 403, Message: "insufficient permissions"})
		c.Abort()
	}
}

func GetUserID(c *gin.Context) int64 {
	id, _ := c.Get(ContextUserID)
	return id.(int64)
}

func GetUsername(c *gin.Context) string {
	username, _ := c.Get(ContextUsername)
	return username.(string)
}

func GetRole(c *gin.Context) string {
	role, _ := c.Get(ContextRole)
	return role.(string)
}

func IsCaptain(c *gin.Context) bool {
	return GetRole(c) == "captain"
}
