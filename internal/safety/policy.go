package safety

import (
	"fmt"
	"strings"
)

// Decision 表示策略对动作的审核结论
type Decision int

const (
	// Allow 直接放行
	Allow Decision = iota
	// Ask 需要人工审批授权
	Ask
	// Deny 绝对禁止执行
	Deny
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Ask:
		return "ask"
	case Deny:
		return "deny"
	default:
		return fmt.Sprintf("decision(%d)", int(d))
	}
}

// ParseDecision 解析字符串形式的决策
func ParseDecision(s string) (Decision, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return Allow, nil
	case "ask":
		return Ask, nil
	case "deny":
		return Deny, nil
	default:
		return Allow, fmt.Errorf("unknown decision %q (want allow, ask or deny)", s)
	}
}

// Preset 表示安全审核预设等级
type Preset int

const (
	// PresetNone 关闭预设，仅使用自定义规则
	PresetNone Preset = 0
	// PresetCommits 关键操作均需审批（支付、发送、提交、删除、设置变更等）
	PresetCommits Preset = 1
	// PresetPayments 仅涉及支付与财产转账的操作需审批（默认推荐）
	PresetPayments Preset = 2
	// PresetEvery 每一个操作均需人工审核
	PresetEvery Preset = 3
)

func (p Preset) String() string {
	switch p {
	case PresetNone:
		return "none"
	case PresetCommits:
		return "commits"
	case PresetPayments:
		return "payments"
	case PresetEvery:
		return "every"
	default:
		return fmt.Sprintf("preset(%d)", int(p))
	}
}

// ActionKind 动作特征语义分类
type ActionKind string

const (
	KindPayment       ActionKind = "payment"        // 支付、转账、购买
	KindCommit        ActionKind = "commit"         // 提交、确认、下单、删除
	KindSendMessage   ActionKind = "send_message"   // 发送消息、发布动态
	KindInstallApp    ActionKind = "install_app"    // 安装/卸载应用
	KindChangeSetting ActionKind = "change_setting" // 修改系统/账号安全设置
	KindCredentials   ActionKind = "credentials"    // 密码或验证码输入
)

// Operation 基础操作类型
type Operation string

const (
	OpTap    Operation = "tap"
	OpType   Operation = "type"
	OpSwipe  Operation = "swipe"
	OpKey    Operation = "key"
	OpLaunch Operation = "launch"
)

// Input 传递给策略引擎的当前操作上下文
type Input struct {
	Device       string    `json:"device"`                  // 设备序列号
	Op           Operation `json:"op"`                      // 操作类型: tap, type, swipe, key, launch
	PackageName  string    `json:"package_name,omitempty"`  // 当前应用包名
	AppName      string    `json:"app_name,omitempty"`      // 当前应用显示名称
	ElementLabel string    `json:"element_label,omitempty"` // 目标元素文字/标签说明
	ElementRole  string    `json:"element_role,omitempty"`  // 目标元素角色（button, input 等）
	IsPassword   bool      `json:"is_password,omitempty"`   // 目标是否为密码字段
	Text         string    `json:"text,omitempty"`          // 输入文本内容 (OpType)
	Key          string    `json:"key,omitempty"`           // 按键名称 (OpKey)
	ReadOnly     bool      `json:"read_only,omitempty"`     // 会话是否声明只读
}

// Verdict 策略评估结论
type Verdict struct {
	Decision Decision     `json:"decision"`
	Rule     string       `json:"rule,omitempty"`   // 触发判定的规则名称
	Kinds    []ActionKind `json:"kinds,omitempty"`  // 命中的语义动作类型
	Reason   string       `json:"reason,omitempty"` // 说明文案（供 Agent 与人工审批卡片展示）
}

// Has 检查结论是否包含指定动作类型
func (v Verdict) Has(k ActionKind) bool {
	for _, x := range v.Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Rule 自定义规则
type Rule struct {
	Name     string   `json:"name" yaml:"name"`
	Packages []string `json:"packages,omitempty" yaml:"packages,omitempty"` // 适用包名列表
	Actions  []string `json:"actions,omitempty" yaml:"actions,omitempty"`   // 适用动作类型
	Keywords []string `json:"keywords,omitempty" yaml:"keywords,omitempty"` // 元素文字关键词
	Decision Decision `json:"decision" yaml:"decision"`
}
