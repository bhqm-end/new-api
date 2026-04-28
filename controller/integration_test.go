package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupIntegrationControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	originalQuotaForNewUser := common.QuotaForNewUser
	common.QuotaForNewUser = 0
	t.Cleanup(func() {
		common.QuotaForNewUser = originalQuotaForNewUser
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.Redemption{}, &model.Log{}); err != nil {
		t.Fatalf("failed to migrate integration test db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newIntegrationContext(t *testing.T, method string, target string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := common.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, reader)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	return ctx, recorder
}

func decodeIntegrationAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) tokenAPIResponse {
	t.Helper()

	var response tokenAPIResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode api response: %v", err)
	}
	return response
}

func seedIntegrationUser(t *testing.T, db *gorm.DB, externalId string, group string) model.User {
	t.Helper()

	user := model.User{
		ExternalId:  common.GetPointer(externalId),
		Username:    "u_" + externalId,
		DisplayName: "User " + externalId,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       group,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return user
}

func TestCreateIntegrationUserIsIdempotent(t *testing.T) {
	setupIntegrationControllerTestDB(t)

	body := map[string]any{
		"external_user_id": "site-user-1",
		"display_name":     "Site User",
		"group":            "default",
	}

	firstCtx, firstRecorder := newIntegrationContext(t, http.MethodPost, "/api/integration/users", body)
	CreateIntegrationUser(firstCtx)
	firstResponse := decodeIntegrationAPIResponse(t, firstRecorder)
	if !firstResponse.Success {
		t.Fatalf("expected first create to succeed, got message: %s", firstResponse.Message)
	}
	var firstUser integrationUserResponse
	if err := common.Unmarshal(firstResponse.Data, &firstUser); err != nil {
		t.Fatalf("failed to decode first user response: %v", err)
	}
	if !firstUser.Created {
		t.Fatalf("expected first call to create user")
	}
	if firstUser.ExternalUserId != "site-user-1" {
		t.Fatalf("expected external id to be returned, got %q", firstUser.ExternalUserId)
	}

	secondCtx, secondRecorder := newIntegrationContext(t, http.MethodPost, "/api/integration/users", body)
	CreateIntegrationUser(secondCtx)
	secondResponse := decodeIntegrationAPIResponse(t, secondRecorder)
	if !secondResponse.Success {
		t.Fatalf("expected second create to succeed, got message: %s", secondResponse.Message)
	}
	var secondUser integrationUserResponse
	if err := common.Unmarshal(secondResponse.Data, &secondUser); err != nil {
		t.Fatalf("failed to decode second user response: %v", err)
	}
	if secondUser.Created {
		t.Fatalf("expected second call to return existing user")
	}
	if firstUser.Id != secondUser.Id {
		t.Fatalf("expected idempotent create to return user %d, got %d", firstUser.Id, secondUser.Id)
	}
}

func TestCreateAndQueryIntegrationUserToken(t *testing.T) {
	db := setupIntegrationControllerTestDB(t)
	user := seedIntegrationUser(t, db, "site-user-2", "default")

	body := map[string]any{
		"name":  "image-token",
		"group": "default",
	}
	createCtx, createRecorder := newIntegrationContext(t, http.MethodPost, "/api/integration/users/"+strconv.Itoa(user.Id)+"/tokens", body)
	createCtx.Params = gin.Params{{Key: "user_id", Value: strconv.Itoa(user.Id)}}
	CreateIntegrationUserToken(createCtx)
	createResponse := decodeIntegrationAPIResponse(t, createRecorder)
	if !createResponse.Success {
		t.Fatalf("expected token create to succeed, got message: %s", createResponse.Message)
	}

	var createdToken integrationTokenResponse
	if err := common.Unmarshal(createResponse.Data, &createdToken); err != nil {
		t.Fatalf("failed to decode created token response: %v", err)
	}
	if createdToken.UserId != user.Id {
		t.Fatalf("expected token user id %d, got %d", user.Id, createdToken.UserId)
	}
	if !createdToken.UnlimitedQuota {
		t.Fatalf("expected integration token to be unlimited")
	}
	if createdToken.RemainQuota != 0 {
		t.Fatalf("expected integration token remain quota 0, got %d", createdToken.RemainQuota)
	}
	if createdToken.Key == "" || createdToken.Key == createdToken.MaskedKey {
		t.Fatalf("expected full token key and masked key to differ")
	}

	queryCtx, queryRecorder := newIntegrationContext(t, http.MethodGet, "/api/integration/users/"+strconv.Itoa(user.Id)+"/tokens?group=default", nil)
	queryCtx.Params = gin.Params{{Key: "user_id", Value: strconv.Itoa(user.Id)}}
	GetIntegrationUserTokens(queryCtx)
	queryResponse := decodeIntegrationAPIResponse(t, queryRecorder)
	if !queryResponse.Success {
		t.Fatalf("expected token query to succeed, got message: %s", queryResponse.Message)
	}

	var tokens []integrationTokenResponse
	if err := common.Unmarshal(queryResponse.Data, &tokens); err != nil {
		t.Fatalf("failed to decode token query response: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected one queried token, got %d", len(tokens))
	}
	if tokens[0].Key != createdToken.Key {
		t.Fatalf("expected queried token full key %q, got %q", createdToken.Key, tokens[0].Key)
	}
}

func TestRedeemIntegrationUserCard(t *testing.T) {
	db := setupIntegrationControllerTestDB(t)
	user := seedIntegrationUser(t, db, "site-user-3", "default")

	redemption := model.Redemption{
		UserId:      1,
		Key:         "integration-card",
		Status:      common.RedemptionCodeStatusEnabled,
		Name:        "card",
		Quota:       456,
		CreatedTime: common.GetTimestamp(),
	}
	if err := db.Create(&redemption).Error; err != nil {
		t.Fatalf("failed to create redemption: %v", err)
	}

	body := map[string]any{"key": redemption.Key}
	ctx, recorder := newIntegrationContext(t, http.MethodPost, "/api/integration/users/"+strconv.Itoa(user.Id)+"/redeem", body)
	ctx.Params = gin.Params{{Key: "user_id", Value: strconv.Itoa(user.Id)}}
	RedeemIntegrationUserCard(ctx)
	response := decodeIntegrationAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected redeem to succeed, got message: %s", response.Message)
	}

	var data struct {
		Quota     int `json:"quota"`
		UserQuota int `json:"user_quota"`
	}
	if err := common.Unmarshal(response.Data, &data); err != nil {
		t.Fatalf("failed to decode redeem response: %v", err)
	}
	if data.Quota != redemption.Quota {
		t.Fatalf("expected redeemed quota %d, got %d", redemption.Quota, data.Quota)
	}
	if data.UserQuota != redemption.Quota {
		t.Fatalf("expected user quota %d, got %d", redemption.Quota, data.UserQuota)
	}

	var fetched model.Redemption
	if err := db.First(&fetched, "id = ?", redemption.Id).Error; err != nil {
		t.Fatalf("failed to fetch redemption: %v", err)
	}
	if fetched.Status != common.RedemptionCodeStatusUsed {
		t.Fatalf("expected redemption to be used, got status %d", fetched.Status)
	}
	if fetched.UsedUserId != user.Id {
		t.Fatalf("expected used user id %d, got %d", user.Id, fetched.UsedUserId)
	}
}
