package safety

import (
	"fmt"
	"strings"
	"sync"
)

// PolicyEngine 核心策略评估引擎
type PolicyEngine struct {
	mu          sync.RWMutex
	preset      Preset
	customRules []Rule
	alwaysAllow map[string]bool // map[pkg:rule] -> bool 持久化免审核记录
}

// NewEngine 创建策略引擎实例
func NewEngine(preset Preset) *PolicyEngine {
	if preset < PresetNone || preset > PresetEvery {
		preset = PresetPayments
	}
	return &PolicyEngine{
		preset:      preset,
		alwaysAllow: make(map[string]bool),
	}
}

// SetPreset 设置安全预设等级
func (e *PolicyEngine) SetPreset(p Preset) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.preset = p
}

// Preset 获取当前安全预设等级
func (e *PolicyEngine) Preset() Preset {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.preset
}

// AddRule 添加自定义规则
func (e *PolicyEngine) AddRule(r Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.customRules = append(e.customRules, r)
}

// SetAlwaysAllow 设置某应用下某规则始终放行
func (e *PolicyEngine) SetAlwaysAllow(pkg, rule string, allow bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := fmt.Sprintf("%s:%s", pkg, rule)
	if allow {
		e.alwaysAllow[key] = true
	} else {
		delete(e.alwaysAllow, key)
	}
}

// isAlwaysAllowed 检查是否处于免审批记录中
func (e *PolicyEngine) isAlwaysAllowed(pkg, rule string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", pkg, rule)
	return e.alwaysAllow[key]
}

// Evaluate 对单次操作执行全流程策略裁定
func (e *PolicyEngine) Evaluate(in Input) Verdict {
	e.mu.RLock()
	preset := e.preset
	rules := e.customRules
	e.mu.RUnlock()

	kinds := DetectKinds(in)

	// 1. 绝对防御：禁止向密码或敏感凭据输入框写入内容
	if in.Op == OpType && (in.IsPassword || in.ElementRole == "password" || in.HasKind(kinds, KindCredentials)) {
		return Verdict{
			Decision: Deny,
			Rule:     "credentials",
			Kinds:    kinds,
			Reason:   "禁止向密码或验证码字段自动注入输入；请调用 phone_handoff 交由人工完成输入。",
		}
	}

	// 2. 预设策略评估
	var presetVerdict *Verdict
	switch preset {
	case PresetEvery:
		// 最严模式：除简单的读操作外，所有下发操作均需人工确认
		if in.Op != "" {
			presetVerdict = &Verdict{
				Decision: Ask,
				Rule:     "Every action",
				Kinds:    kinds,
				Reason:   fmt.Sprintf("当前启用『每步审核』模式，操作 [%s] 需要人工确认。", in.Op),
			}
		}

	case PresetCommits:
		// 提交与破坏性操作审查（支付、发消息、提交订单、删除数据等）
		if in.HasKind(kinds, KindPayment) {
			presetVerdict = &Verdict{
				Decision: Ask,
				Rule:     "Payments",
				Kinds:    kinds,
				Reason:   fmt.Sprintf("检测到支付/转账行为：%q，需要人工确认。", in.ElementLabel),
			}
		} else if in.HasKind(kinds, KindSendMessage) {
			presetVerdict = &Verdict{
				Decision: Ask,
				Rule:     "Messages",
				Kinds:    kinds,
				Reason:   fmt.Sprintf("检测到消息发送/发布行为：%q，需要人工确认。", in.ElementLabel),
			}
		} else if in.HasKind(kinds, KindCommit) {
			presetVerdict = &Verdict{
				Decision: Ask,
				Rule:     "Commits",
				Kinds:    kinds,
				Reason:   fmt.Sprintf("检测到关键提交/确认行为：%q，需要人工确认。", in.ElementLabel),
			}
		}

	case PresetPayments:
		// 默认推荐：仅财产支付与转账拦截
		if in.HasKind(kinds, KindPayment) {
			presetVerdict = &Verdict{
				Decision: Ask,
				Rule:     "Payments",
				Kinds:    kinds,
				Reason:   fmt.Sprintf("检测到支付/转账行为：%q，需要人工确认。", in.ElementLabel),
			}
		}

	case PresetNone:
		// 不加载默认预设，全靠自定义规则
	}

	// 3. 自定义规则评估 (最严优先: Deny > Ask > Allow)
	finalVerdict := Verdict{Decision: Allow, Kinds: kinds}
	if presetVerdict != nil {
		finalVerdict = *presetVerdict
	}

	for _, r := range rules {
		if !ruleMatches(r, in, kinds) {
			continue
		}
		if r.Decision > finalVerdict.Decision {
			finalVerdict.Decision = r.Decision
			finalVerdict.Rule = r.Name
			finalVerdict.Reason = fmt.Sprintf("匹配自定义安全规则 %q", r.Name)
		}
	}

	// 4. Always allow 豁免检查 (仅可豁免 Ask，绝对不能豁免 Deny)
	if finalVerdict.Decision == Ask && finalVerdict.Rule != "" && in.PackageName != "" {
		if e.isAlwaysAllowed(in.PackageName, finalVerdict.Rule) {
			finalVerdict.Decision = Allow
			finalVerdict.Reason = fmt.Sprintf("规则 %q 已在应用 %s 中被配置为始终允许。", finalVerdict.Rule, in.PackageName)
		}
	}

	// 5. 只读模式强制门禁
	if in.ReadOnly {
		if finalVerdict.Decision == Ask {
			finalVerdict.Decision = Deny
			finalVerdict.Reason = fmt.Sprintf("会话处于只读模式，已拒绝需要人工审核的动作 (%s)。", finalVerdict.Rule)
		} else if in.Op == OpType {
			// 只读模式下仅允许搜索输入
			if !strings.Contains(strings.ToLower(in.ElementLabel), "search") &&
				!strings.Contains(in.ElementLabel, "搜索") {
				finalVerdict.Decision = Deny
				finalVerdict.Rule = "Read-only"
				finalVerdict.Reason = "会话处于只读模式：仅允许在搜索框中键入文本。"
			}
		}
	}

	return finalVerdict
}

func (in Input) HasKind(kinds []ActionKind, k ActionKind) bool {
	for _, x := range kinds {
		if x == k {
			return true
		}
	}
	return false
}

func ruleMatches(r Rule, in Input, kinds []ActionKind) bool {
	// 匹配应用包名
	if len(r.Packages) > 0 {
		matchedPkg := false
		for _, p := range r.Packages {
			if strings.EqualFold(p, in.PackageName) {
				matchedPkg = true
				break
			}
		}
		if !matchedPkg {
			return false
		}
	}

	// 匹配动作
	if len(r.Actions) > 0 {
		matchedAction := false
		for _, a := range r.Actions {
			if strings.EqualFold(a, string(in.Op)) {
				matchedAction = true
				break
			}
			for _, k := range kinds {
				if strings.EqualFold(a, string(k)) {
					matchedAction = true
					break
				}
			}
		}
		if !matchedAction {
			return false
		}
	}

	// 匹配关键词
	if len(r.Keywords) > 0 {
		if !containsAny(in.ElementLabel, r.Keywords) {
			return false
		}
	}

	return true
}
