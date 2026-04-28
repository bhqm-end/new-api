package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type integrationCreateUserRequest struct {
	ExternalUserId string `json:"external_user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email"`
	Group          string `json:"group"`
}

type integrationUserResponse struct {
	Id             int    `json:"id"`
	ExternalUserId string `json:"external_user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email,omitempty"`
	Group          string `json:"group"`
	Quota          int    `json:"quota"`
	Created        bool   `json:"created"`
	CreatedAt      int64  `json:"created_at,omitempty"`
	LastLoginAt    int64  `json:"last_login_at,omitempty"`
}

type integrationCreateTokenRequest struct {
	Name               string  `json:"name"`
	Group              string  `json:"group"`
	ExpiredTime        *int64  `json:"expired_time"`
	AllowIps           *string `json:"allow_ips"`
	ModelLimitsEnabled bool    `json:"model_limits_enabled"`
	ModelLimits        string  `json:"model_limits"`
	CrossGroupRetry    *bool   `json:"cross_group_retry"`
}

type integrationTokenResponse struct {
	Id                 int    `json:"id"`
	UserId             int    `json:"user_id"`
	Name               string `json:"name"`
	Key                string `json:"key"`
	MaskedKey          string `json:"masked_key"`
	Status             int    `json:"status"`
	Group              string `json:"group"`
	CreatedTime        int64  `json:"created_time"`
	AccessedTime       int64  `json:"accessed_time"`
	ExpiredTime        int64  `json:"expired_time"`
	RemainQuota        int    `json:"remain_quota"`
	UsedQuota          int    `json:"used_quota"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

type integrationRedeemRequest struct {
	Key string `json:"key"`
}

func CreateIntegrationUser(c *gin.Context) {
	var req integrationCreateUserRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	externalId := strings.TrimSpace(req.ExternalUserId)
	if externalId == "" {
		common.ApiErrorMsg(c, "external_user_id is required")
		return
	}
	if len(externalId) > 128 {
		common.ApiErrorMsg(c, "external_user_id is too long")
		return
	}

	if user, err := model.GetUserByExternalId(externalId, false); err == nil {
		common.ApiSuccess(c, buildIntegrationUserResponse(user, false))
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = integrationUsernameFromExternalId(externalId)
	}
	if username == "" || len(username) > model.UserNameMaxLength {
		common.ApiErrorMsg(c, fmt.Sprintf("username length must be 1-%d", model.UserNameMaxLength))
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 20 {
		common.ApiErrorMsg(c, "display_name is too long")
		return
	}

	email := strings.TrimSpace(req.Email)
	if len(email) > 50 {
		common.ApiErrorMsg(c, "email is too long")
		return
	}

	group := strings.TrimSpace(req.Group)
	if group == "" {
		group = "default"
	}

	exists, err := model.CheckUserExistOrDeleted(username, email)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if exists {
		common.ApiErrorMsg(c, "username or email already exists")
		return
	}

	cleanUser := model.User{
		ExternalId:  &externalId,
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Group:       group,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	if err := cleanUser.Insert(0); err != nil {
		if existingUser, getErr := model.GetUserByExternalId(externalId, false); getErr == nil {
			common.ApiSuccess(c, buildIntegrationUserResponse(existingUser, false))
			return
		}
		common.ApiError(c, err)
		return
	}

	createdUser, err := model.GetUserByExternalId(externalId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildIntegrationUserResponse(createdUser, true))
}

func CreateIntegrationUserToken(c *gin.Context) {
	user, ok := getIntegrationUserByParam(c)
	if !ok {
		return
	}

	var req integrationCreateTokenRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	group := strings.TrimSpace(req.Group)
	if group == "" {
		common.ApiErrorMsg(c, "group is required")
		return
	}
	if err := validateIntegrationTokenGroup(user.Group, group); err != nil {
		common.ApiError(c, err)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "image-site-default"
	}
	if len(name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}

	count, err := model.CountUserTokens(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	maxTokens := operation_setting.GetMaxUserTokens()
	if int(count) >= maxTokens {
		common.ApiErrorMsg(c, fmt.Sprintf("user token limit reached (%d)", maxTokens))
		return
	}

	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate integration token key: " + err.Error())
		return
	}

	expiredTime := int64(-1)
	if req.ExpiredTime != nil {
		expiredTime = *req.ExpiredTime
	}
	crossGroupRetry := false
	if req.CrossGroupRetry != nil {
		crossGroupRetry = *req.CrossGroupRetry
	}

	token := model.Token{
		UserId:             user.Id,
		Name:               name,
		Key:                key,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        expiredTime,
		RemainQuota:        0,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: req.ModelLimitsEnabled,
		ModelLimits:        req.ModelLimits,
		AllowIps:           req.AllowIps,
		Group:              group,
		CrossGroupRetry:    crossGroupRetry,
	}
	if err := token.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildIntegrationTokenResponse(&token))
}

func GetIntegrationUserTokens(c *gin.Context) {
	user, ok := getIntegrationUserByParam(c)
	if !ok {
		return
	}

	group := strings.TrimSpace(c.Query("group"))
	tokens, err := model.GetUserTokensByGroup(user.Id, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	responses := make([]integrationTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		responses = append(responses, buildIntegrationTokenResponse(token))
	}
	common.ApiSuccess(c, responses)
}

func RedeemIntegrationUserCard(c *gin.Context) {
	user, ok := getIntegrationUserByParam(c)
	if !ok {
		return
	}

	var req integrationRedeemRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		common.ApiErrorMsg(c, "key is required")
		return
	}

	quota, err := model.Redeem(key, user.Id)
	if err != nil {
		if errors.Is(err, model.ErrRedeemFailed) {
			common.ApiErrorI18n(c, i18n.MsgRedeemFailed)
			return
		}
		common.ApiError(c, err)
		return
	}
	userQuota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"quota":      quota,
		"user_quota": userQuota,
	})
}

func getIntegrationUserByParam(c *gin.Context) (*model.User, bool) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return nil, false
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return user, true
}

func validateIntegrationTokenGroup(userGroup string, tokenGroup string) error {
	if tokenGroup == "auto" {
		if len(service.GetUserAutoGroup(userGroup)) == 0 || len(setting.GetAutoGroups()) == 0 {
			return errors.New("auto group is not available for this user")
		}
		return nil
	}
	if !service.GroupInUserUsableGroups(userGroup, tokenGroup) {
		return fmt.Errorf("user group %s cannot use token group %s", userGroup, tokenGroup)
	}
	if !ratio_setting.ContainsGroupRatio(tokenGroup) {
		return fmt.Errorf("group %s is disabled", tokenGroup)
	}
	return nil
}

func integrationUsernameFromExternalId(externalId string) string {
	sum := sha256.Sum256([]byte(externalId))
	return "ext_" + hex.EncodeToString(sum[:])[:16]
}

func buildIntegrationUserResponse(user *model.User, created bool) integrationUserResponse {
	externalId := ""
	if user.ExternalId != nil {
		externalId = *user.ExternalId
	}
	return integrationUserResponse{
		Id:             user.Id,
		ExternalUserId: externalId,
		Username:       user.Username,
		DisplayName:    user.DisplayName,
		Email:          user.Email,
		Group:          user.Group,
		Quota:          user.Quota,
		Created:        created,
		CreatedAt:      user.CreatedAt,
		LastLoginAt:    user.LastLoginAt,
	}
}

func buildIntegrationTokenResponse(token *model.Token) integrationTokenResponse {
	return integrationTokenResponse{
		Id:                 token.Id,
		UserId:             token.UserId,
		Name:               token.Name,
		Key:                token.GetFullKey(),
		MaskedKey:          token.GetMaskedKey(),
		Status:             token.Status,
		Group:              token.Group,
		CreatedTime:        token.CreatedTime,
		AccessedTime:       token.AccessedTime,
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UsedQuota:          token.UsedQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		CrossGroupRetry:    token.CrossGroupRetry,
	}
}
