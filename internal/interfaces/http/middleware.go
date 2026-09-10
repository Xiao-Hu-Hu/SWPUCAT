package http

import (
	"SWPUCAT/internal/application/authorization"
	"SWPUCAT/internal/domain/user"
	"SWPUCAT/internal/infrastructure/auth"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserID   = "user_id"
	ContextUsername = "username"
	ContextRole     = "role"
	ContextActor    = "actor"
)

func JWTAuthMiddleware(jwtSvc *auth.JWTService, authzSvc *authorization.Service) gin.HandlerFunc {
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

		claims, err := jwtSvc.ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, Response{Code: 401, Message: "invalid or expired token"})
			c.Abort()
			return
		}
		actor, err := authzSvc.ResolveActor(c.Request.Context(), claims.UserID, claims.AuthVersion)
		if err != nil {
			c.JSON(http.StatusUnauthorized, Response{Code: 401, Message: "account is inactive or session is revoked"})
			c.Abort()
			return
		}

		c.Set(ContextUserID, actor.UserID)
		c.Set(ContextUsername, actor.DisplayName)
		c.Set(ContextRole, string(actor.Role))
		c.Set(ContextActor, actor)
		c.Next()
	}
}

func RBACMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, exists := c.Get(ContextActor)
		if !exists {
			c.JSON(http.StatusForbidden, Response{Code: 403, Message: "role not found"})
			c.Abort()
			return
		}

		resolvedActor, ok := actor.(authorization.Actor)
		if !ok {
			c.JSON(http.StatusForbidden, Response{Code: 403, Message: "invalid role"})
			c.Abort()
			return
		}

		for _, allowed := range allowedRoles {
			if resolvedActor.Role == user.Role(allowed) {
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
	value, _ := id.(int64)
	return value
}

func GetUsername(c *gin.Context) string {
	username, _ := c.Get(ContextUsername)
	value, _ := username.(string)
	return value
}

func GetRole(c *gin.Context) string {
	role, _ := c.Get(ContextRole)
	value, _ := role.(string)
	return value
}

func IsCaptain(c *gin.Context) bool {
	role := GetRole(c)
	return role == "captain" || role == "super_admin"
}
