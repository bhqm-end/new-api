package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func IntegrationAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		configuredKey := strings.TrimSpace(common.IntegrationApiKey)
		if configuredKey == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "integration api key is not configured",
			})
			c.Abort()
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		const bearerPrefix = "Bearer "
		if len(authHeader) < len(bearerPrefix) || !strings.EqualFold(authHeader[:len(bearerPrefix)], bearerPrefix) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "invalid integration authorization header",
			})
			c.Abort()
			return
		}

		providedKey := strings.TrimSpace(authHeader[len(bearerPrefix):])
		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(configuredKey)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "invalid integration api key",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
