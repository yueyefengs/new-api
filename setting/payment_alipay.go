package setting

var (
	AlipayEnabled      bool
	AlipayAppId        string
	AlipayPrivateKey   string
	AlipayPublicKey    string
	AlipayAppCertPath  string
	AlipayCertPath     string
	AlipayRootCertPath string
	AlipaySandbox      bool
	AlipayNotifyUrl    string
	AlipayMinTopUp     int     = 1
	AlipayUnitPrice    float64 = 1.0
)
