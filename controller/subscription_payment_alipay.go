package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/smartwalle/alipay/v3"
	"github.com/thanhpk/randstr"
)

type SubscriptionAlipayPayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestAlipayPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionAlipayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.PriceAmount < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	if !isAlipayTopUpEnabled() {
		common.ApiErrorMsg(c, "支付宝支付未启用")
		return
	}

	userId := c.GetInt("id")
	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	client, err := getAlipayClient()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝订阅客户端初始化失败 user_id=%d plan_id=%d error=%q", userId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	tradeNo := fmt.Sprintf("SUBALI%d%s%d", userId, randstr.String(6), time.Now().Unix())
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAlipayPcWeb,
		PaymentProvider: model.PaymentProviderAlipayPcWeb,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝订阅订单创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	notifyUrl := strings.TrimSpace(setting.AlipayNotifyUrl)
	if notifyUrl == "" {
		notifyUrl = service.GetCallbackAddress() + "/api/alipay/notify"
	}

	trade := alipay.TradePagePay{
		Trade: alipay.Trade{
			NotifyURL:   notifyUrl,
			Subject:     fmt.Sprintf("订阅套餐 - %s", strings.TrimSpace(plan.Title)),
			OutTradeNo:  tradeNo,
			TotalAmount: fmt.Sprintf("%.2f", plan.PriceAmount),
			ProductCode: "FAST_INSTANT_TRADE_PAY",
			GoodsType:   "0",
		},
		IntegrationType: "PCWEB",
	}

	payURL, err := client.TradePagePay(trade)
	if err != nil {
		order.Status = common.TopUpStatusFailed
		order.CompleteTime = common.GetTimestamp()
		_ = order.Update()
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝订阅下单失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}
	if payURL == nil || strings.TrimSpace(payURL.String()) == "" {
		order.Status = common.TopUpStatusFailed
		order.CompleteTime = common.GetTimestamp()
		_ = order.Update()
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝订阅下单未返回 pay_url user_id=%d plan_id=%d trade_no=%s", userId, plan.Id, tradeNo))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝订阅订单创建成功 user_id=%d plan_id=%d trade_no=%s money=%.2f", userId, plan.Id, tradeNo, plan.PriceAmount))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_url":  payURL.String(),
			"trade_no": tradeNo,
		},
	})
}
