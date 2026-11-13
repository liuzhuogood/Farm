package safety

import (
	"testing"
)

func TestPolicyEnginePasswordDenial(t *testing.T) {
	eng := NewEngine(PresetPayments)

	// 密码框输入一律绝对拒绝
	v := eng.Evaluate(Input{
		Op:           OpType,
		IsPassword:   true,
		ElementLabel: "请输入支付密码",
		Text:         "123456",
	})

	if v.Decision != Deny {
		t.Fatalf("期望密码字段 Deny，实际 %s", v.Decision)
	}
	if v.Rule != "credentials" {
		t.Errorf("期望规则 credentials，实际 %s", v.Rule)
	}
}

func TestPolicyEnginePresetPayments(t *testing.T) {
	eng := NewEngine(PresetPayments)

	// 1. 支付动作应判定为 Ask
	vPay := eng.Evaluate(Input{
		Op:           OpTap,
		ElementLabel: "立即付款 ¥88.00",
		PackageName:  "com.taobao.taobao",
	})
	if vPay.Decision != Ask {
		t.Fatalf("期望支付动作 Ask，实际 %s", vPay.Decision)
	}
	if vPay.Rule != "Payments" {
		t.Errorf("期望规则 Payments，实际 %s", vPay.Rule)
	}

	// 2. 普通点击应直接放行 Allow
	vNormal := eng.Evaluate(Input{
		Op:           OpTap,
		ElementLabel: "查看商品详情",
		PackageName:  "com.taobao.taobao",
	})
	if vNormal.Decision != Allow {
		t.Fatalf("期望普通动作 Allow，实际 %s", vNormal.Decision)
	}
}

func TestPolicyEnginePresetCommits(t *testing.T) {
	eng := NewEngine(PresetCommits)

	// 发送消息
	vMsg := eng.Evaluate(Input{
		Op:           OpTap,
		ElementLabel: "发送",
		PackageName:  "com.tencent.mm",
	})
	if vMsg.Decision != Ask || vMsg.Rule != "Messages" {
		t.Fatalf("期望发送消息 Ask(Messages)，实际 %s (%s)", vMsg.Decision, vMsg.Rule)
	}

	// 提交删除
	vDel := eng.Evaluate(Input{
		Op:           OpTap,
		ElementLabel: "确认清空相册",
		PackageName:  "com.android.gallery3d",
	})
	if vDel.Decision != Ask || vDel.Rule != "Commits" {
		t.Fatalf("期望提交清空 Ask(Commits)，实际 %s (%s)", vDel.Decision, vDel.Rule)
	}
}

func TestPolicyEngineAlwaysAllowExemption(t *testing.T) {
	eng := NewEngine(PresetPayments)
	pkg := "com.dinely.app"

	// 第一次应 Ask
	v1 := eng.Evaluate(Input{
		Op:           OpTap,
		ElementLabel: "立即支付 $12.80",
		PackageName:  pkg,
	})
	if v1.Decision != Ask {
		t.Fatalf("期望首次支付 Ask，实际 %s", v1.Decision)
	}

	// 设置该应用下的 Payments 始终允许
	eng.SetAlwaysAllow(pkg, "Payments", true)

	v2 := eng.Evaluate(Input{
		Op:           OpTap,
		ElementLabel: "立即支付 $12.80",
		PackageName:  pkg,
	})
	if v2.Decision != Allow {
		t.Fatalf("豁免后期望 Allow，实际 %s (reason: %s)", v2.Decision, v2.Reason)
	}
}

func TestPolicyEngineReadOnlyMode(t *testing.T) {
	eng := NewEngine(PresetPayments)

	// 只读模式下，若操作需要 Ask 则强制降级为 Deny
	vAsk := eng.Evaluate(Input{
		Op:           OpTap,
		ElementLabel: "确认支付",
		ReadOnly:     true,
	})
	if vAsk.Decision != Deny {
		t.Fatalf("只读模式下需要审核的动作应被 Deny，实际 %s", vAsk.Decision)
	}

	// 只读模式下非搜索框输入一律 Deny
	vType := eng.Evaluate(Input{
		Op:           OpType,
		ElementLabel: "输入个人简介",
		Text:         "hello",
		ReadOnly:     true,
	})
	if vType.Decision != Deny {
		t.Fatalf("只读模式下非搜索输入应被 Deny，实际 %s", vType.Decision)
	}

	// 只读模式下搜索框输入允许
	vSearch := eng.Evaluate(Input{
		Op:           OpType,
		ElementLabel: "搜索设置项",
		Text:         "wifi",
		ReadOnly:     true,
	})
	if vSearch.Decision != Allow {
		t.Fatalf("只读模式下搜索框应被 Allow，实际 %s", vSearch.Decision)
	}
}
