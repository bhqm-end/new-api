package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func setupRedemptionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	if DB == nil {
		t.Fatalf("test database is not initialized")
	}
	db := DB
	if err := db.AutoMigrate(&User{}, &Redemption{}, &Log{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	return db
}

func TestRedeemInvalidUserDoesNotConsumeRedemption(t *testing.T) {
	db := setupRedemptionTestDB(t)

	redemption := Redemption{
		UserId:      1,
		Key:         "invalid-user-card",
		Status:      common.RedemptionCodeStatusEnabled,
		Name:        "card",
		Quota:       123,
		CreatedTime: common.GetTimestamp(),
	}
	if err := db.Create(&redemption).Error; err != nil {
		t.Fatalf("failed to create redemption: %v", err)
	}

	quota, err := Redeem(redemption.Key, 999)
	if quota != 0 {
		t.Fatalf("expected no quota to be redeemed, got %d", quota)
	}
	if !errors.Is(err, ErrRedeemFailed) {
		t.Fatalf("expected ErrRedeemFailed, got %v", err)
	}

	var fetched Redemption
	if err := db.First(&fetched, "id = ?", redemption.Id).Error; err != nil {
		t.Fatalf("failed to fetch redemption: %v", err)
	}
	if fetched.Status != common.RedemptionCodeStatusEnabled {
		t.Fatalf("expected redemption to remain enabled, got status %d", fetched.Status)
	}
	if fetched.UsedUserId != 0 {
		t.Fatalf("expected redemption used_user_id to remain 0, got %d", fetched.UsedUserId)
	}
	if fetched.RedeemedTime != 0 {
		t.Fatalf("expected redemption redeemed_time to remain 0, got %d", fetched.RedeemedTime)
	}
}
