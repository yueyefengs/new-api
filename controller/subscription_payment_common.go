package controller

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

type SubscriptionPaymentLifecycleRequest struct {
	PlanId                 int     `json:"plan_id"`
	Scenario               string  `json:"scenario"`
	CurrentSubscriptionId  int     `json:"current_subscription_id"`
	ExpectedCurrentPlanId  int     `json:"expected_current_plan_id"`
	ExpectedCurrentEndTime int64   `json:"expected_current_end_time"`
	OverrideMoney          float64 `json:"override_money"`
}

func subscriptionPurchaseErrorMessage(err error) string {
	switch {
	case errors.Is(err, model.ErrSubscriptionPendingChangeExists):
		return "当前账号已有待生效的订阅变更，请等待生效后再继续调整"
	case errors.Is(err, model.ErrSubscriptionStateChanged):
		return "当前订阅状态已变化，请刷新页面后重试"
	case errors.Is(err, model.ErrSubscriptionScenarioInvalid):
		return "当前订阅变更场景无效，请刷新页面后重试"
	default:
		message := strings.TrimSpace(err.Error())
		if message == "" {
			return "订阅购买请求无效"
		}
		return message
	}
}
