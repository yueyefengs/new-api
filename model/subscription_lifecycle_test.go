package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForSubscriptionLifecycleTest(t *testing.T, id int, group string) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "subscription_lifecycle_user",
		Status:   common.UserStatusEnabled,
		Group:    group,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertPlanForSubscriptionLifecycleTest(t *testing.T, id int, title string, price float64, upgradeGroup string) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         title,
		PriceAmount:   price,
		Currency:      "CNY",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		UpgradeGroup:  upgradeGroup,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionForLifecycleTest(t *testing.T, sub *UserSubscription) {
	t.Helper()
	require.NoError(t, DB.Create(sub).Error)
}

func insertOrderForLifecycleTest(t *testing.T, order *SubscriptionOrder, lifecycle SubscriptionOrderLifecycle) {
	t.Helper()
	require.NoError(t, order.SetLifecycle(lifecycle))
	require.NoError(t, order.Insert())
}

func getUserGroupForLifecycleTest(t *testing.T, userId int) string {
	t.Helper()
	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userId).Select(commonGroupCol).Find(&group).Error)
	return group
}

func TestCompleteSubscriptionOrder_SamePlanRenewalCreatesScheduledSubscription(t *testing.T) {
	truncateTables(t)

	insertUserForSubscriptionLifecycleTest(t, 9001, "yix_basic_subscription")
	plan := insertPlanForSubscriptionLifecycleTest(t, 9101, "Basic Monthly", 99, "yix_basic_subscription")

	now := time.Now().Unix()
	currentSub := &UserSubscription{
		UserId:        9001,
		PlanId:        plan.Id,
		StartTime:     now - 3600,
		EndTime:       now + 86400,
		Status:        SubscriptionStatusActive,
		Source:        "order",
		UpgradeGroup:  "yix_basic_subscription",
		PrevUserGroup: "default",
	}
	insertSubscriptionForLifecycleTest(t, currentSub)

	order := &SubscriptionOrder{
		UserId:          9001,
		PlanId:          plan.Id,
		Money:           99,
		TradeNo:         "sub-renew-lifecycle",
		PaymentMethod:   PaymentMethodAlipayPcWeb,
		PaymentProvider: PaymentProviderAlipayPcWeb,
		Status:          common.TopUpStatusPending,
		CreateTime:      now,
	}
	insertOrderForLifecycleTest(t, order, SubscriptionOrderLifecycle{
		Scenario:              SubscriptionScenarioSamePlanRenewal,
		CurrentSubscriptionId: currentSub.Id,
		CurrentPlanId:         plan.Id,
		CurrentEndTime:        now + 86400,
		FallbackGroup:         "default",
	})

	err := CompleteNativeSubscriptionOrder(
		order.TradeNo,
		`{"provider":"alipay"}`,
		PaymentProviderAlipayPcWeb,
		PaymentMethodAlipayPcWeb,
		99,
	)
	require.NoError(t, err)

	var subs []UserSubscription
	require.NoError(t, DB.Where("user_id = ?", 9001).Order("id asc").Find(&subs).Error)
	require.Len(t, subs, 2)

	assert.Equal(t, SubscriptionStatusActive, subs[0].Status)
	assert.Equal(t, SubscriptionStatusScheduled, subs[1].Status)
	assert.Equal(t, now+86400, subs[1].StartTime)
	assert.True(t, subs[1].EndTime > subs[1].StartTime)
	assert.Equal(t, "default", subs[1].PrevUserGroup)
	assert.Equal(t, "yix_basic_subscription", getUserGroupForLifecycleTest(t, 9001))
}

func TestCompleteSubscriptionOrder_SameCycleUpgradeReplacesCurrentSubscription(t *testing.T) {
	truncateTables(t)

	insertUserForSubscriptionLifecycleTest(t, 9002, "yix_basic_subscription")
	basicPlan := insertPlanForSubscriptionLifecycleTest(t, 9201, "Basic Monthly", 99, "yix_basic_subscription")
	proPlan := insertPlanForSubscriptionLifecycleTest(t, 9202, "Pro Monthly", 299, "yix_pro_subscription")

	now := time.Now().Unix()
	currentSub := &UserSubscription{
		UserId:        9002,
		PlanId:        basicPlan.Id,
		StartTime:     now - 7200,
		EndTime:       now + 86400,
		Status:        SubscriptionStatusActive,
		Source:        "order",
		UpgradeGroup:  "yix_basic_subscription",
		PrevUserGroup: "default",
	}
	insertSubscriptionForLifecycleTest(t, currentSub)

	order := &SubscriptionOrder{
		UserId:          9002,
		PlanId:          proPlan.Id,
		Money:           66,
		TradeNo:         "sub-upgrade-lifecycle",
		PaymentMethod:   PaymentMethodWechatPay,
		PaymentProvider: PaymentProviderWechatPay,
		Status:          common.TopUpStatusPending,
		CreateTime:      now,
	}
	insertOrderForLifecycleTest(t, order, SubscriptionOrderLifecycle{
		Scenario:              SubscriptionScenarioSameCycleUpgrade,
		CurrentSubscriptionId: currentSub.Id,
		CurrentPlanId:         basicPlan.Id,
		CurrentEndTime:        now + 86400,
		FallbackGroup:         "default",
	})

	err := CompleteNativeSubscriptionOrder(
		order.TradeNo,
		`{"provider":"wechatpay"}`,
		PaymentProviderWechatPay,
		PaymentMethodWechatPay,
		66,
	)
	require.NoError(t, err)

	var subs []UserSubscription
	require.NoError(t, DB.Where("user_id = ?", 9002).Order("id asc").Find(&subs).Error)
	require.Len(t, subs, 2)

	assert.Equal(t, SubscriptionStatusCancelled, subs[0].Status)
	assert.True(t, subs[0].EndTime >= now)
	assert.Equal(t, SubscriptionStatusActive, subs[1].Status)
	assert.Equal(t, proPlan.Id, subs[1].PlanId)
	assert.Equal(t, now+86400, subs[1].EndTime)
	assert.Equal(t, "default", subs[1].PrevUserGroup)
	assert.Equal(t, "yix_pro_subscription", getUserGroupForLifecycleTest(t, 9002))
}

func TestSyncDueUserSubscriptions_ActivatesScheduledSubscription(t *testing.T) {
	truncateTables(t)

	insertUserForSubscriptionLifecycleTest(t, 9003, "yix_pro_subscription")
	proPlan := insertPlanForSubscriptionLifecycleTest(t, 9301, "Pro Monthly", 299, "yix_pro_subscription")
	basicPlan := insertPlanForSubscriptionLifecycleTest(t, 9302, "Basic Monthly", 99, "yix_basic_subscription")

	now := time.Now().Unix()
	insertSubscriptionForLifecycleTest(t, &UserSubscription{
		UserId:        9003,
		PlanId:        proPlan.Id,
		StartTime:     now - 86400,
		EndTime:       now - 10,
		Status:        SubscriptionStatusActive,
		Source:        "order",
		UpgradeGroup:  "yix_pro_subscription",
		PrevUserGroup: "default",
	})
	insertSubscriptionForLifecycleTest(t, &UserSubscription{
		UserId:        9003,
		PlanId:        basicPlan.Id,
		StartTime:     now - 10,
		EndTime:       now + 86400,
		Status:        SubscriptionStatusScheduled,
		Source:        "order",
		UpgradeGroup:  "yix_basic_subscription",
		PrevUserGroup: "default",
	})

	activated, expired, err := SyncDueUserSubscriptions(10)
	require.NoError(t, err)
	assert.Equal(t, 1, activated)
	assert.Equal(t, 1, expired)

	var subs []UserSubscription
	require.NoError(t, DB.Where("user_id = ?", 9003).Order("id asc").Find(&subs).Error)
	require.Len(t, subs, 2)

	assert.Equal(t, SubscriptionStatusExpired, subs[0].Status)
	assert.Equal(t, SubscriptionStatusActive, subs[1].Status)
	assert.Equal(t, "yix_basic_subscription", getUserGroupForLifecycleTest(t, 9003))
}
