package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestIntegrationAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalKey := common.IntegrationApiKey
	t.Cleanup(func() {
		common.IntegrationApiKey = originalKey
	})

	common.IntegrationApiKey = "expected-secret"

	router := gin.New()
	router.GET("/integration-test", IntegrationAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	unauthorized := httptest.NewRecorder()
	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/integration-test", nil)
	router.ServeHTTP(unauthorized, unauthorizedReq)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing auth to return 401, got %d", unauthorized.Code)
	}

	forbidden := httptest.NewRecorder()
	forbiddenReq := httptest.NewRequest(http.MethodGet, "/integration-test", nil)
	forbiddenReq.Header.Set("Authorization", "Bearer wrong-secret")
	router.ServeHTTP(forbidden, forbiddenReq)
	if forbidden.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid auth to return 401, got %d", forbidden.Code)
	}

	authorized := httptest.NewRecorder()
	authorizedReq := httptest.NewRequest(http.MethodGet, "/integration-test", nil)
	authorizedReq.Header.Set("Authorization", "Bearer expected-secret")
	router.ServeHTTP(authorized, authorizedReq)
	if authorized.Code != http.StatusOK {
		t.Fatalf("expected valid auth to return 200, got %d", authorized.Code)
	}
}

func TestIntegrationAuthRejectsWhenNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalKey := common.IntegrationApiKey
	t.Cleanup(func() {
		common.IntegrationApiKey = originalKey
	})

	common.IntegrationApiKey = ""

	router := gin.New()
	router.GET("/integration-test", IntegrationAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/integration-test", nil)
	req.Header.Set("Authorization", "Bearer any-secret")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected unconfigured auth to return 403, got %d", recorder.Code)
	}
}
