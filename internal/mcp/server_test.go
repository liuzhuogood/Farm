package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"farm/internal/automation"
	"farm/internal/device"
	"farm/internal/safety"
	"farm/internal/uipage"
	"farm/internal/uixml"
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

	s.SetDeviceProvider(func() []device.Device { return []device.Device{{Serial: "emulator-5554", State: "device"}} })
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

type failingXMLProvider struct{}

func (failingXMLProvider) FetchSnapshot(context.Context, string) (uixml.Snapshot, error) {
	return uixml.Snapshot{}, errors.New("dump failed")
}

type launchPageProvider struct{}

func (launchPageProvider) FetchPageInfo(context.Context, string) (*uipage.PageInfo, error) {
	return &uipage.PageInfo{PackageName: "com.android.settings", Display: &uipage.DisplayInfo{Width: 1080, Height: 2400}}, nil
}

func TestResolveContextRejectsUnknownOfflineAndBoundDevice(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "test")
	online := true
	s.SetDeviceProvider(func() []device.Device {
		state := "offline"
		if online {
			state = "device"
		}
		return []device.Device{{Serial: "10.0.0.30:5555", State: state}, {Serial: "10.0.0.104:5555", State: "offline"}}
	})
	s.devices["session"] = "10.0.0.30:5555"
	if serial, _, err := s.resolveContext("session", ""); err != nil || serial != "10.0.0.30:5555" {
		t.Fatalf("valid bound device: %s %v", serial, err)
	}
	if _, _, err := s.resolveContext("session", "redmi"); err == nil {
		t.Fatal("alias accepted as serial")
	}
	online = false
	if _, _, err := s.resolveContext("session", ""); err == nil {
		t.Fatal("offline bound device accepted")
	}
}

func TestConnectFailureDoesNotBindDevice(t *testing.T) {
	svc := &automation.Service{}
	svc.SetUIXMLProvider(failingXMLProvider{})
	svc.SetUIPageProvider(launchPageProvider{})
	s := NewServer(svc, nil, nil, nil, "test")
	s.SetDeviceProvider(func() []device.Device {
		return []device.Device{{Serial: "10.0.0.30:5555", State: "device"}, {Serial: "10.0.0.104:5555", State: "offline"}}
	})
	for _, serial := range []string{"redmi", "10.0.0.30:5555"} {
		res, _, err := s.handleConnect(context.Background(), &sdk.CallToolRequest{}, ConnectInput{Device: serial, Task: "test"})
		if err != nil || !res.IsError {
			t.Fatalf("connect should fail: %v %#v", err, res)
		}
		if serial == "redmi" {
			text := res.Content[0].(*sdk.TextContent).Text
			for _, want := range []string{`identifier="10.0.0.30:5555" serial=10.0.0.30:5555 (state=device)`} {
				if !strings.Contains(text, want) {
					t.Fatalf("device list missing %q: %s", want, text)
				}
			}
			if strings.Contains(text, "connected to") {
				t.Fatalf("invalid serial reported connected: %s", text)
			}
		}
		if len(s.devices) != 0 || len(s.tasks) != 0 {
			t.Fatalf("failed connect bound session: %#v", s.devices)
		}
	}
}

func TestLaunchReportsPostLaunchObservationFailureSeparately(t *testing.T) {
	svc := &automation.Service{}
	svc.SetUIXMLProvider(failingXMLProvider{})
	svc.SetUIPageProvider(launchPageProvider{})
	s := NewServer(svc, nil, nil, nil, "test")
	s.SetDeviceProvider(func() []device.Device { return []device.Device{{Serial: "device-1", State: "device"}} })
	calls := 0
	s.launchApp = func(_ context.Context, serial, pkg string) error {
		calls++
		if serial != "device-1" || pkg != "com.android.settings" {
			t.Fatal("wrong launch target")
		}
		return nil
	}
	res, _, err := s.handleLaunch(context.Background(), &sdk.CallToolRequest{}, LaunchInput{Device: "device-1", App: "com.android.settings"})
	text := res.Content[0].(*sdk.TextContent).Text
	if err != nil || !res.IsError || calls != 1 || !strings.Contains(text, "observe_failed") || !strings.Contains(text, "launched com.android.settings") || strings.Contains(text, "launch_failed") {
		t.Fatalf("unexpected result: %s %v, launches=%d", text, err, calls)
	}
	s.launchApp = func(context.Context, string, string) error { return errors.New("no launcher") }
	res, _, _ = s.handleLaunch(context.Background(), &sdk.CallToolRequest{}, LaunchInput{Device: "device-1", App: "com.android.settings"})
	text = res.Content[0].(*sdk.TextContent).Text
	if !strings.Contains(text, "launch_failed") || !strings.Contains(text, "no launcher") {
		t.Fatalf("launch error: %s", text)
	}
}

func TestResolveDeviceIdentifierUsesOnlineListOrderAndSerialPrecedence(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "test")
	devices := []device.Device{
		{Serial: "offline-first", Brand: "Redmi", State: "offline"},
		{Serial: "first", Brand: "Redmi", State: "device"},
		{Serial: "second", Brand: "Redmi", State: "device"},
		{Serial: "Redmi", Brand: "Other", State: "device"},
		{Serial: "model-only", Model: "Pixel", State: "device"},
	}
	s.SetDeviceProvider(func() []device.Device { return devices })
	for _, tc := range []struct{ input, want string }{{" redmi ", "first"}, {"REDMI", "first"}, {"Redmi", "Redmi"}, {"second", "second"}, {"model-only", "model-only"}} {
		got, err := s.resolveDevice(tc.input, true)
		if err != nil || got != tc.want {
			t.Fatalf("resolve %q got %q %v want %q", tc.input, got, err, tc.want)
		}
	}
	if _, err := s.resolveDevice("Pixel", true); err == nil {
		t.Fatal("model guessed as identifier")
	}
	if _, err := s.resolveDevice("offline-first", true); err == nil {
		t.Fatal("explicit offline serial accepted")
	}
	devices[1].Alias = "Desk"
	got, err := s.resolveDevice("redmi", true)
	if err != nil || got != "second" {
		t.Fatalf("renamed first still used old brand alias: %s %v", got, err)
	}
	devices[2].State = "offline"
	// Exact serial Redmi still wins even if it is offline; do not redirect to an alias.
	devices[3].State = "offline"
	devices = append(devices, device.Device{Serial: "other", Alias: "Redmi", State: "device"})
	if _, err := s.resolveDevice("Redmi", true); err == nil {
		t.Fatal("offline serial redirected to another identifier")
	}
}

func TestBoundDeviceKeepsSerialAfterIdentifierRenameOrRemoval(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "test")
	devices := []device.Device{{Serial: "first", Alias: "Desk", State: "device"}, {Serial: "second", Alias: "first", State: "device"}}
	s.SetDeviceProvider(func() []device.Device { return devices })
	s.devices["session"] = "first"
	devices[0].Alias = "Renamed"
	if got, _, err := s.resolveContext("session", ""); err != nil || got != "first" {
		t.Fatalf("renamed bound device: %s %v", got, err)
	}
	devices = devices[1:]
	if got, _, err := s.resolveContext("session", ""); err == nil {
		t.Fatalf("removed bound serial redirected to identifier: %s", got)
	}
}

func TestPhoneDevicesListsOnlyOnlineIdentifiersWithoutBinding(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "test")
	s.SetDeviceProvider(func() []device.Device {
		return []device.Device{
			{Serial: "one", Alias: "  Desk  ", Brand: "Redmi", State: "device"},
			{Serial: "two", Brand: "Vivo", State: "offline"},
			{Serial: "three", Brand: "Redmi", State: "device"},
		}
	})
	res, _, err := s.handleDevices(context.Background(), &sdk.CallToolRequest{}, DevicesInput{})
	if err != nil || res.IsError {
		t.Fatalf("list: %v %#v", err, res)
	}
	var entries []DeviceListEntry
	if err := json.Unmarshal([]byte(res.Content[0].(*sdk.TextContent).Text), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Identifier != "Desk" || entries[0].Serial != "one" || entries[1].Identifier != "Redmi" || entries[1].State != "device" {
		t.Fatalf("entries: %#v", entries)
	}
	if len(s.devices) != 0 || len(s.tasks) != 0 {
		t.Fatal("read-only listing bound a session")
	}
	_, err = s.resolveDevice("unknown", true)
	if err == nil {
		t.Fatal("unknown accepted")
	}
	for _, want := range []string{`identifier="Desk" serial=one (state=device)`, `identifier="Redmi" serial=three (state=device)`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing list %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "serial=two") {
		t.Fatalf("offline device exposed in current list: %v", err)
	}
}

type recordingXMLProvider struct{ serial string }

func (p *recordingXMLProvider) FetchSnapshot(_ context.Context, serial string) (uixml.Snapshot, error) {
	p.serial = serial
	return uixml.Snapshot{XML: `<hierarchy><node text="Settings" class="android.widget.TextView" bounds="[0,0][100,100]"/></hierarchy>`}, nil
}

func TestConnectIdentifierBindsResolvedSerial(t *testing.T) {
	svc := &automation.Service{}
	xml := &recordingXMLProvider{}
	svc.SetUIXMLProvider(xml)
	svc.SetUIPageProvider(launchPageProvider{})
	s := NewServer(svc, nil, nil, nil, "test")
	s.SetDeviceProvider(func() []device.Device {
		return []device.Device{{Serial: "10.0.0.30:5555", Brand: "Redmi", State: "device"}}
	})
	res, _, err := s.handleConnect(context.Background(), &sdk.CallToolRequest{}, ConnectInput{Device: "redmi", Task: "test"})
	if err != nil || res.IsError || s.devices["default"] != "10.0.0.30:5555" || xml.serial != "10.0.0.30:5555" {
		t.Fatalf("connect not resolved: %v %#v binding=%s observed=%s", err, res, s.devices["default"], xml.serial)
	}
}
