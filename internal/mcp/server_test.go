package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"farm/internal/safety"
)

func TestMCPServerToolsListing(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "1.0.0")
	if s.SDK() == nil {
		t.Fatal("SDK() 不应返回 nil")
	}

	// 验证 HTTP 处理句柄初始化正常
	handler := s.HTTPHandler()
	if handler == nil {
		t.Fatal("HTTPHandler() 不应返回 nil")
	}
}

func TestMCPPhoneTapInterceptedByPolicy(t *testing.T) {
	// 初始化策略引擎为 PresetPayments 模式
	policy := safety.NewEngine(safety.PresetPayments)
	approvals := safety.NewApprovalCenter(5 * time.Second)

	s := &Server{
		autoService: nil,
		policy:      policy,
		approvals:   approvals,
		tasks:       make(map[string]string),
		devices:     make(map[string]string),
		stepIndices: make(map[string]int),
	}

	ctx := context.Background()
	req := &sdk.CallToolRequest{}

	// 1. 发起涉及支付的点击，期望返回 approval_pending
	res, _, err := s.handleTap(ctx, req, TapInput{
		Device: "emulator-5554",
		Ref:    10,
		Intent: "购买商品并立即付款",
	})
	if err != nil {
		t.Fatalf("handleTap 抛错: %v", err)
	}

	text := res.Content[0].(*sdk.TextContent).Text
	if !strings.Contains(text, "approval_pending") {
		t.Fatalf("期望返回 approval_pending 拦截，实际: %s", text)
	}
	if !strings.Contains(text, "approval_id: ap_") {
		t.Fatalf("期望包含生成的 approval_id，实际: %s", text)
	}

	// 提取 approval_id
	var appID string
	for _, part := range strings.Fields(text) {
		if strings.HasPrefix(part, "ap_") {
			appID = part
			break
		}
	}
	if appID == "" {
		t.Fatalf("未找到生成的 approval_id: %s", text)
	}

	// 2. 模拟用户人工审批放行
	err = approvals.Decide(appID, safety.DecisionAllowOnce, "放行测试")
	if err != nil {
		t.Fatalf("Decide 失败: %v", err)
	}

	// 3. 带已批准的 approval_id 再次调用，此时不再被拦截
	// （由于 autoService 为 nil，坐标计算阶段会正常抛出 device_error，证明已经跨越安全拦截网关）
	res2, _, _ := s.handleTap(ctx, req, TapInput{
		Device:     "emulator-5554",
		Ref:        10,
		Intent:     "购买商品并立即付款",
		ApprovalID: appID,
	})
	text2 := res2.Content[0].(*sdk.TextContent).Text
	if strings.Contains(text2, "approval_pending") {
		t.Fatalf("已放行的审批不应再次处于 pending 状态: %s", text2)
	}
}
