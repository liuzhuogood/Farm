package safety

import (
	"strings"
)

// 多语言支付与财产转账关键词库
var paymentKeywords = []string{
	// 中文
	"支付", "付款", "转账", "买入", "立即支付", "确认付款", "去支付", "买单", "结算", "充值", "提现",
	// 英文
	"pay", "payment", "checkout", "buy now", "purchase", "transfer", "subscribe",
	// 常见变体
	"confirm payment", "place order",
}

// 承诺与破坏性操作关键词库（提交、删除、确认等）
var commitKeywords = []string{
	// 中文
	"提交", "确认", "下单", "删除", "清空", "移除", "注销", "发货", "同意并授权", "授权并登录",
	// 英文
	"commit", "confirm", "submit", "delete", "remove", "erase", "clear all", "order now", "authorize",
}

// 发送消息关键词库
var sendMessageKeywords = []string{
	// 中文
	"发送", "发表", "发布", "发帖", "评论", "回复", "私信",
	// 英文
	"send", "post", "publish", "reply", "tweet",
}

// 密码与凭据输入字段特征词
var credentialKeywords = []string{
	"密码", "密碼", "password", "pin", "安全码", "验证码", "驗證碼", "passcode", "cvv", "cvc", "otp", "code",
}

// 常见敏感金融/支付应用包名
var SensitivePackages = map[string]string{
	"com.eg.android.AlipayGphone":        "Alipay",
	"com.tencent.mm":                     "WeChat",
	"com.unionpay":                       "UnionPay",
	"com.chinamworld.main":               "CCB",
	"com.icbc":                           "ICBC",
	"com.bankofchina.mobilebanking":      "BOC",
	"com.abchina.mobile.mbank":           "ABC",
	"com.cmbchina.ccd.pluto.cmbActivity": "CMB",
	"com.paypal.android.p2pmobile":       "PayPal",
	"com.binance.dev":                    "Binance",
	"com.okex.android":                   "OKX",
}

// containsAny 检查文本是否包含关键词数组中的任意一项（忽略大小写）
func containsAny(text string, keywords []string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// DetectKinds 分析输入操作，提取匹配的 ActionKind 语义集合
func DetectKinds(in Input) []ActionKind {
	var kinds []ActionKind

	label := strings.TrimSpace(in.ElementLabel)

	// 1. 凭证/密码字段检测
	if in.IsPassword || in.ElementRole == "password" || (in.Op == OpType && containsAny(label, credentialKeywords)) {
		kinds = append(kinds, KindCredentials)
	}

	// 2. 支付行为检测
	if containsAny(label, paymentKeywords) {
		kinds = append(kinds, KindPayment)
	}

	// 3. 消息发送检测
	if in.Op == OpType && in.Key == "enter" {
		kinds = append(kinds, KindSendMessage)
	} else if containsAny(label, sendMessageKeywords) {
		kinds = append(kinds, KindSendMessage)
	}

	// 4. 提交/破坏性动作检测
	if containsAny(label, commitKeywords) {
		kinds = append(kinds, KindCommit)
	}

	return kinds
}

// IsSensitiveApp 检查是否为受保护的敏感应用（银行、钱包、支付工具）
func IsSensitiveApp(pkg string) (string, bool) {
	name, ok := SensitivePackages[pkg]
	return name, ok
}
