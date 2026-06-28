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
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
)

type SubscriptionWechatPayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestWechatPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionWechatPayRequest
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
	if !isWechatPayTopUpEnabled() {
		common.ApiErrorMsg(c, "微信支付未启用")
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

	payMoney := decimal.NewFromFloat(plan.PriceAmount).Round(2)
	if !strings.EqualFold(strings.TrimSpace(plan.Currency), "CNY") {
		payMoney = payMoney.
			Mul(decimal.NewFromFloat(setting.WechatPayUnitPrice)).
			Round(2)
	}
	if payMoney.LessThan(decimal.NewFromFloat(0.01)) {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}

	ctx := c.Request.Context()
	client, err := getWechatPayClient(ctx)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("微信支付订阅客户端初始化失败 user_id=%d plan_id=%d error=%q", userId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	tradeNo := fmt.Sprintf("SUBWX%d%s%d", userId, randstr.String(6), time.Now().Unix())
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           payMoney.InexactFloat64(),
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWechatPay,
		PaymentProvider: model.PaymentProviderWechatPay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("微信支付订阅订单创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	notifyUrl := strings.TrimSpace(setting.WechatPayNotifyUrl)
	if notifyUrl == "" {
		notifyUrl = service.GetCallbackAddress() + "/api/wechatpay/notify"
	}

	totalFen := payMoney.Mul(decimal.NewFromInt(100)).IntPart()
	if totalFen < 1 {
		totalFen = 1
	}

	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
		Appid:       core.String(setting.WechatPayAppId),
		Mchid:       core.String(setting.WechatPayMchId),
		Description: core.String(fmt.Sprintf("订阅套餐 - %s", strings.TrimSpace(plan.Title))),
		OutTradeNo:  core.String(tradeNo),
		NotifyUrl:   core.String(notifyUrl),
		Amount: &native.Amount{
			Total:    core.Int64(totalFen),
			Currency: core.String("CNY"),
		},
	})
	if err != nil {
		order.Status = common.TopUpStatusFailed
		order.CompleteTime = common.GetTimestamp()
		_ = order.Update()
		logger.LogError(ctx, fmt.Sprintf("微信支付订阅下单失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}
	if resp == nil || resp.CodeUrl == nil || strings.TrimSpace(*resp.CodeUrl) == "" {
		order.Status = common.TopUpStatusFailed
		order.CompleteTime = common.GetTimestamp()
		_ = order.Update()
		logger.LogError(ctx, fmt.Sprintf("微信支付订阅下单未返回 code_url user_id=%d plan_id=%d trade_no=%s", userId, plan.Id, tradeNo))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("微信支付订阅订单创建成功 user_id=%d plan_id=%d trade_no=%s money=%.2f", userId, plan.Id, tradeNo, payMoney.InexactFloat64()))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"code_url": *resp.CodeUrl,
			"trade_no": tradeNo,
		},
	})
}
