package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/smartwalle/alipay/v3"
	"github.com/thanhpk/randstr"
)

var (
	alipayClient    *alipay.Client
	alipayClientMu  sync.Mutex
	alipayClientKey string
)

type AlipayPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

func getAlipayClient() (*alipay.Client, error) {
	alipayClientMu.Lock()
	defer alipayClientMu.Unlock()

	clientKey := strings.Join([]string{
		setting.AlipayAppId,
		setting.AlipayPrivateKey,
		setting.AlipayPublicKey,
		setting.AlipayAppCertPath,
		setting.AlipayCertPath,
		setting.AlipayRootCertPath,
		fmt.Sprintf("%t", setting.AlipaySandbox),
	}, "\x00")

	if alipayClient != nil && alipayClientKey == clientKey {
		return alipayClient, nil
	}

	client, err := alipay.New(setting.AlipayAppId, setting.AlipayPrivateKey, !setting.AlipaySandbox)
	if err != nil {
		return nil, fmt.Errorf("创建支付宝客户端失败: %w", err)
	}

	if setting.AlipayAppCertPath != "" && setting.AlipayCertPath != "" && setting.AlipayRootCertPath != "" {
		if err := client.LoadAppCertPublicKeyFromFile(setting.AlipayAppCertPath); err != nil {
			return nil, fmt.Errorf("加载应用公钥证书失败: %w", err)
		}
		if err := client.LoadAlipayCertPublicKeyFromFile(setting.AlipayCertPath); err != nil {
			return nil, fmt.Errorf("加载支付宝公钥证书失败: %w", err)
		}
		if err := client.LoadAliPayRootCertFromFile(setting.AlipayRootCertPath); err != nil {
			return nil, fmt.Errorf("加载支付宝根证书失败: %w", err)
		}
	} else if setting.AlipayPublicKey != "" {
		if err := client.LoadAliPayPublicKey(setting.AlipayPublicKey); err != nil {
			return nil, fmt.Errorf("加载支付宝公钥失败: %w", err)
		}
	}

	alipayClient = client
	alipayClientKey = clientKey
	return alipayClient, nil
}

func ResetAlipayClient() {
	alipayClientMu.Lock()
	defer alipayClientMu.Unlock()
	alipayClient = nil
	alipayClientKey = ""
}

func getAlipayMoney(amount float64, group string) float64 {
	dAmount := decimal.NewFromFloat(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(setting.AlipayUnitPrice)

	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)
	return payMoney.InexactFloat64()
}

func getAlipayMinTopup() int64 {
	minTopup := setting.AlipayMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func RequestAlipay(c *gin.Context) {
	var req AlipayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.PaymentMethod != model.PaymentMethodAlipay {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}

	if req.Amount < getAlipayMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getAlipayMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getAlipayMoney(float64(req.Amount), group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	ctx := c.Request.Context()
	client, err := getAlipayClient()
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("支付宝客户端初始化失败: %s", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	tradeNo := fmt.Sprintf("ALI%d%s%d", id, randstr.String(6), time.Now().Unix())

	notifyUrl := setting.AlipayNotifyUrl
	if notifyUrl == "" {
		callbackAddr := service.GetCallbackAddress()
		notifyUrl = callbackAddr + "/api/alipay/notify"
	}

	totalAmount := fmt.Sprintf("%.2f", payMoney)

	trade := alipay.TradePreCreate{
		Trade: alipay.Trade{
			NotifyURL:   notifyUrl,
			Subject:     "账户充值",
			OutTradeNo:  tradeNo,
			TotalAmount: totalAmount,
		},
	}

	resp, err := client.TradePreCreate(context.Background(), trade)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("支付宝下单失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	if !resp.IsSuccess() {
		logger.LogError(ctx, fmt.Sprintf("支付宝下单返回失败 user_id=%d trade_no=%s code=%s msg=%s", id, tradeNo, resp.Code, resp.SubMsg))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败: " + resp.SubMsg})
		return
	}
	if resp.QRCode == "" {
		logger.LogError(ctx, fmt.Sprintf("支付宝下单未返回 qr_code user_id=%d trade_no=%s", id, tradeNo))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	topUp := &model.TopUp{
		UserId:          id,
		Amount:          req.Amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAlipay,
		PaymentProvider: model.PaymentProviderAlipay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("支付宝创建充值订单失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("支付宝订单创建成功 user_id=%d trade_no=%s amount=%d money=%.2f", id, tradeNo, req.Amount, payMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"qr_code":  resp.QRCode,
			"trade_no": tradeNo,
		},
	})
}

func AlipayNotify(c *gin.Context) {
	ctx := c.Request.Context()

	if !isAlipayTopUpEnabled() {
		logger.LogWarn(ctx, "支付宝回调被拒绝: 支付通道未启用")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	if err := c.Request.ParseForm(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("支付宝回调解析表单失败: %s", err.Error()))
		c.String(http.StatusBadRequest, "fail")
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("支付宝回调收到请求 client_ip=%s", c.ClientIP()))

	client, err := getAlipayClient()
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("支付宝回调获取客户端失败: %s", err.Error()))
		c.String(http.StatusInternalServerError, "fail")
		return
	}

	notification, err := client.DecodeNotification(context.Background(), c.Request.Form)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("支付宝回调验签失败: %s", err.Error()))
		c.String(http.StatusBadRequest, "fail")
		return
	}

	tradeNo := notification.OutTradeNo
	tradeStatus := notification.TradeStatus

	logger.LogInfo(ctx, fmt.Sprintf("支付宝回调验签成功 trade_no=%s trade_status=%s trade_id=%s", tradeNo, tradeStatus, notification.TradeNo))

	if tradeStatus != alipay.TradeStatusSuccess && tradeStatus != alipay.TradeStatusFinished {
		logger.LogInfo(ctx, fmt.Sprintf("支付宝回调非成功状态，忽略 trade_no=%s trade_status=%s", tradeNo, tradeStatus))
		c.String(http.StatusOK, "success")
		return
	}

	if notification.AppId != "" && notification.AppId != setting.AlipayAppId {
		logger.LogWarn(ctx, fmt.Sprintf("支付宝回调 app_id 不匹配 trade_no=%s app_id=%s", tradeNo, notification.AppId))
		c.String(http.StatusBadRequest, "fail")
		return
	}

	paidAmount := notification.ReceiptAmount
	if paidAmount == "" {
		paidAmount = notification.TotalAmount
	}
	if paidAmount == "" {
		logger.LogWarn(ctx, fmt.Sprintf("支付宝回调金额为空 trade_no=%s", tradeNo))
		c.String(http.StatusBadRequest, "fail")
		return
	}

	paidMoney, err := decimal.NewFromString(paidAmount)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("支付宝回调金额解析失败 trade_no=%s amount=%s error=%q", tradeNo, paidAmount, err.Error()))
		c.String(http.StatusBadRequest, "fail")
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	if err := model.CompleteNativeTopUp(tradeNo, model.PaymentProviderAlipay, paidMoney.InexactFloat64(), c.ClientIP()); err != nil {
		if errors.Is(err, model.ErrTopUpNotFound) ||
			errors.Is(err, model.ErrPaymentMethodMismatch) ||
			errors.Is(err, model.ErrTopUpStatusInvalid) ||
			errors.Is(err, model.ErrPaymentAmountMismatch) {
			logger.LogWarn(ctx, fmt.Sprintf("支付宝回调业务校验失败 trade_no=%s amount=%s error=%q", tradeNo, paidAmount, err.Error()))
			c.String(http.StatusOK, "success")
			return
		}
		logger.LogError(ctx, fmt.Sprintf("支付宝完成充值失败 trade_no=%s amount=%s error=%q", tradeNo, paidAmount, err.Error()))
		c.String(http.StatusInternalServerError, "fail")
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("支付宝充值成功 trade_no=%s money=%s", tradeNo, paidAmount))

	c.String(http.StatusOK, "success")
}
