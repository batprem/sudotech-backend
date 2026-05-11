package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const ctxUserKey = "uid"

func (s *Service) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		uid, err := s.Parse(strings.TrimPrefix(header, prefix))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ctxUserKey, uid)
		c.Next()
	}
}

func UserID(c *gin.Context) int64 {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return 0
	}
	uid, _ := v.(int64)
	return uid
}
