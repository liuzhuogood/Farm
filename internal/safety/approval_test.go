package safety

import (
	"context"
	"testing"
	"time"
)

func TestApprovalLifecycleAllowOnce(t *testing.T) {
	ac := NewApprovalCenter(10 * time.Second)

	bind := ApprovalBinding{
		Device:   "phone-01",
		Tool:     "tap",
		ArgsHash: ComputeArgsHash("ref=12"),
	}

	info := ac.Open(ApprovalRequest{
		Binding: bind,
		Rule:    "Payments",
		Title:   "支付 $12.80",
		AppName: "外卖App",
	})

	if info.State != StateOpen {
		t.Fatalf("期望状态 open，实际 %s", info.State)
	}

	// 1. 模拟人工点击允许
	err := ac.Decide(info.ID, DecisionAllowOnce, "用户手动放行")
	if err != nil {
		t.Fatalf("Decide 失败: %v", err)
	}

	// 2. 动作端调用 Consume 校验并消费
	consumed, err := ac.Consume(info.ID, bind)
	if err != nil {
		t.Fatalf("Consume 失败: %v", err)
	}
	if consumed.Decision != DecisionAllowOnce {
		t.Errorf("期望决策 allow_once，实际 %s", consumed.Decision)
	}

	// 3. 再次消费必须拒绝（单次有效性）
	_, errSecond := ac.Consume(info.ID, bind)
	if errSecond != ErrApprovalUsed {
		t.Fatalf("二次消费期望 ErrApprovalUsed，实际 %v", errSecond)
	}
}

func TestApprovalBindingTamperProtection(t *testing.T) {
	ac := NewApprovalCenter(10 * time.Second)

	bindOrigin := ApprovalBinding{
		Device:   "phone-01",
		Tool:     "tap",
		ArgsHash: ComputeArgsHash("ref=12"),
	}

	info := ac.Open(ApprovalRequest{
		Binding: bindOrigin,
		Rule:    "Payments",
		Title:   "支付 $12.80",
	})

	_ = ac.Decide(info.ID, DecisionAllowOnce, "ok")

	// 篡改参数哈希
	bindTampered := ApprovalBinding{
		Device:   "phone-01",
		Tool:     "tap",
		ArgsHash: ComputeArgsHash("ref=999"), // 篡改了目标 ref
	}

	_, err := ac.Consume(info.ID, bindTampered)
	if err != ErrBindingMismatch {
		t.Fatalf("篡改参数期望 ErrBindingMismatch，实际 %v", err)
	}
}

func TestApprovalTimeoutAutoExpire(t *testing.T) {
	ac := NewApprovalCenter(50 * time.Millisecond) // 短超时测试

	bind := ApprovalBinding{
		Device:   "phone-01",
		Tool:     "tap",
		ArgsHash: "test",
	}

	info := ac.Open(ApprovalRequest{
		Binding: bind,
		Rule:    "Every action",
		Title:   "测试超时",
	})

	// 等待超时触发
	time.Sleep(100 * time.Millisecond)

	consumed, err := ac.Consume(info.ID, bind)
	if err != nil {
		t.Fatalf("读取过期单据失败: %v", err)
	}
	if consumed.Decision != DecisionDeny {
		t.Fatalf("超时单据期望自动 Deny，实际 %s", consumed.Decision)
	}
}

func TestApprovalWaitBlock(t *testing.T) {
	ac := NewApprovalCenter(5 * time.Second)

	bind := ApprovalBinding{
		Device:   "phone-01",
		Tool:     "swipe",
		ArgsHash: "hash1",
	}

	info := ac.Open(ApprovalRequest{
		Binding: bind,
		Rule:    "Commits",
		Title:   "滑动确认",
	})

	// 异步延迟做出决策
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = ac.Decide(info.ID, DecisionAllowTask, "批量放行")
	}()

	res, err := ac.Wait(context.Background(), info.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("Wait 出错: %v", err)
	}
	if res.State != StateDecided {
		t.Fatalf("期望状态 decided，实际 %s", res.State)
	}
	if res.Decision != DecisionAllowTask {
		t.Fatalf("期望决策 allow_task，实际 %s", res.Decision)
	}
}
