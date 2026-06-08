package setting

var (
	WechatPayEnabled    bool
	WechatPayMchId      string
	WechatPayAppId      string
	WechatPayApiV3Key   string
	WechatPayPrivateKey string
	WechatPaySerialNo   string
	WechatPayNotifyUrl  string
	WechatPayMinTopUp   int     = 1
	WechatPayUnitPrice  float64 = 1.0
)
