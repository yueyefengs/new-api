package controller

import (
	"context"
	"crypto/rsa"
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
	"github.com/thanhpk/randstr"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

var (
	wechatPayClient   *core.Client
	wechatPayClientMu sync.Mutex
	wechatPayKey      *rsa.PrivateKey
	wechatPayNotify   *notify.Handler
	wechatPayCertMgr  *downloader.CertificateDownloaderMgr
	wechatPayConfig   string
)

type WechatPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

func getWechatPayClient(ctx context.Context) (*core.Client, error) {
	wechatPayClientMu.Lock()
	defer wechatPayClientMu.Unlock()

	configKey := strings.Join([]string{
		setting.WechatPayMchId,
		setting.WechatPayAppId,
		setting.WechatPayApiV3Key,
		setting.WechatPayPrivateKey,
		setting.WechatPaySerialNo,
	}, "\x00")

	if wechatPayClient != nil && wechatPayConfig == configKey {
		return wechatPayClient, nil
	}
	if wechatPayCertMgr != nil {
		wechatPayCertMgr.Stop()
	}
	wechatPayClient = nil
	wechatPayKey = nil
	wechatPayNotify = nil
	wechatPayCertMgr = nil
	wechatPayConfig = ""

	privateKey, err := utils.LoadPrivateKey(setting.WechatPayPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("加载微信支付私钥失败: %w", err)
	}
	wechatPayKey = privateKey

	mgr := downloader.NewCertificateDownloaderMgr(ctx)
	if err := mgr.RegisterDownloaderWithPrivateKey(
		ctx,
		privateKey,
		setting.WechatPaySerialNo,
		setting.WechatPayMchId,
		setting.WechatPayApiV3Key,
	); err != nil {
		mgr.Stop()
		return nil, fmt.Errorf("注册微信支付平台证书下载器失败: %w", err)
	}

	client, err := core.NewClient(
		ctx,
		option.WithWechatPayAutoAuthCipherUsingDownloaderMgr(
			setting.WechatPayMchId,
			setting.WechatPaySerialNo,
			wechatPayKey,
			mgr,
		),
	)
	if err != nil {
		mgr.Stop()
		return nil, fmt.Errorf("创建微信支付客户端失败: %w", err)
	}

	notifyHandler, err := notify.NewRSANotifyHandler(
		setting.WechatPayApiV3Key,
		verifiers.NewSHA256WithRSAVerifier(mgr.GetCertificateVisitor(setting.WechatPayMchId)),
	)
	if err != nil {
		mgr.Stop()
		return nil, fmt.Errorf("创建微信支付通知处理器失败: %w", err)
	}

	wechatPayClient = client
	wechatPayNotify = notifyHandler
	wechatPayCertMgr = mgr
	wechatPayConfig = configKey
	return wechatPayClient, nil
}

func ResetWechatPayClient() {
	wechatPayClientMu.Lock()
	defer wechatPayClientMu.Unlock()
	if wechatPayCertMgr != nil {
		wechatPayCertMgr.Stop()
	}
	wechatPayClient = nil
	wechatPayKey = nil
	wechatPayNotify = nil
	wechatPayCertMgr = nil
	wechatPayConfig = ""
}

func getWechatPayMoney(amount float64, group string) float64 {
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
	dPrice := decimal.NewFromFloat(setting.WechatPayUnitPrice)

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

func getWechatPayMinTopup() int64 {
	minTopup := setting.WechatPayMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func RequestWechatPay(c *gin.Context) {
	var req WechatPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.PaymentMethod != model.PaymentMethodWechatPay {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}

	if req.Amount < getWechatPayMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getWechatPayMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getWechatPayMoney(float64(req.Amount), group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	ctx := c.Request.Context()
	client, err := getWechatPayClient(ctx)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("微信支付客户端初始化失败: %s", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	tradeNo := fmt.Sprintf("WX%d%s%d", id, randstr.String(6), time.Now().Unix())

	notifyUrl := setting.WechatPayNotifyUrl
	if notifyUrl == "" {
		callbackAddr := service.GetCallbackAddress()
		notifyUrl = callbackAddr + "/api/wechatpay/notify"
	}

	totalFen := int64(payMoney*100 + 0.5)
	if totalFen < 1 {
		totalFen = 1
	}

	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
		Appid:       core.String(setting.WechatPayAppId),
		Mchid:       core.String(setting.WechatPayMchId),
		Description: core.String("账户充值"),
		OutTradeNo:  core.String(tradeNo),
		NotifyUrl:   core.String(notifyUrl),
		Amount: &native.Amount{
			Total:    core.Int64(totalFen),
			Currency: core.String("CNY"),
		},
	})
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("微信支付下单失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}
	if resp == nil || resp.CodeUrl == nil || *resp.CodeUrl == "" {
		logger.LogError(ctx, fmt.Sprintf("微信支付下单未返回 code_url user_id=%d trade_no=%s", id, tradeNo))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	topUp := &model.TopUp{
		UserId:          id,
		Amount:          req.Amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWechatPay,
		PaymentProvider: model.PaymentProviderWechatPay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("微信支付创建充值订单失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("微信支付订单创建成功 user_id=%d trade_no=%s amount=%d money=%.2f", id, tradeNo, req.Amount, payMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"code_url": *resp.CodeUrl,
			"trade_no": tradeNo,
		},
	})
}

func WechatPayNotify(c *gin.Context) {
	ctx := c.Request.Context()

	if !isWechatPayTopUpEnabled() {
		logger.LogWarn(ctx, "微信支付回调被拒绝: 支付通道未启用")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	if _, err := getWechatPayClient(ctx); err != nil {
		logger.LogError(ctx, fmt.Sprintf("微信支付回调初始化客户端失败: %s", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "支付服务暂不可用"})
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("微信支付回调收到请求 client_ip=%s", c.ClientIP()))

	transaction := new(payments.Transaction)
	_, err := wechatPayNotify.ParseNotifyRequest(ctx, c.Request, transaction)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("微信支付回调验签/解密失败: %s", err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "验签失败"})
		return
	}

	if transaction.OutTradeNo == nil || transaction.TradeState == nil || transaction.TransactionId == nil || transaction.Amount == nil || transaction.Amount.Total == nil {
		logger.LogWarn(ctx, "微信支付回调关键字段缺失")
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "通知字段缺失"})
		return
	}

	tradeNo := *transaction.OutTradeNo
	tradeState := *transaction.TradeState

	logger.LogInfo(ctx, fmt.Sprintf("微信支付回调验签解密成功 trade_no=%s trade_state=%s transaction_id=%s", tradeNo, tradeState, *transaction.TransactionId))

	if tradeState != "SUCCESS" {
		logger.LogInfo(ctx, fmt.Sprintf("微信支付回调非成功状态，忽略 trade_no=%s trade_state=%s", tradeNo, tradeState))
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": ""})
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	paidMoney := decimal.NewFromInt(*transaction.Amount.Total).Div(decimal.NewFromInt(100)).InexactFloat64()
	if err := model.CompleteNativeTopUp(tradeNo, model.PaymentProviderWechatPay, paidMoney, c.ClientIP()); err != nil {
		if errors.Is(err, model.ErrTopUpNotFound) ||
			errors.Is(err, model.ErrPaymentMethodMismatch) ||
			errors.Is(err, model.ErrTopUpStatusInvalid) ||
			errors.Is(err, model.ErrPaymentAmountMismatch) {
			logger.LogWarn(ctx, fmt.Sprintf("微信支付回调业务校验失败 trade_no=%s amount=%d error=%q", tradeNo, *transaction.Amount.Total, err.Error()))
			c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": ""})
			return
		}
		logger.LogError(ctx, fmt.Sprintf("微信支付完成充值失败 trade_no=%s amount=%d error=%q", tradeNo, *transaction.Amount.Total, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "充值失败"})
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("微信支付充值成功 trade_no=%s money=%.2f", tradeNo, paidMoney))
	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": ""})
}
