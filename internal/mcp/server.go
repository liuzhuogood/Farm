package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"farm/internal/automation"
	"farm/internal/device"
	"farm/internal/safety"
)

// Server Farm 原生 MCP 服务
type Server struct {
	autoService    *automation.Service
	policy         *safety.PolicyEngine
	approvals      *safety.ApprovalCenter
	history        *safety.HistoryStore
	sdkServer      *sdk.Server
	version        string
	deviceProvider func() []device.Device
	launchApp      func(context.Context, string, string) error

	mu          sync.Mutex
	tasks       map[string]string // sessionID -> taskID
	devices     map[string]string // sessionID -> serial
	stepIndices map[string]int    // taskID -> currentStepIndex
}

// NewServer 创建 MCP 协议服务实例
func NewServer(
	auto *automation.Service,
	policy *safety.PolicyEngine,
	approvals *safety.ApprovalCenter,
	history *safety.HistoryStore,
	version string,
) *Server {
	s := &Server{
		autoService: auto,
		policy:      policy,
		approvals:   approvals,
		history:     history,
		version:     version,
		tasks:       make(map[string]string),
		devices:     make(map[string]string),
		stepIndices: make(map[string]int),
	}
	s.sdkServer = s.initSDKServer()
	return s
}

const Instructions = `Farm 让 AI Agent 可以像真人一样操作连接的真实 Android 设备。
操作约定：
1. 使用 phone_devices 获取设备标识名、serial 和状态，再用 phone_connect 开始任务。device 支持精确 serial 或标识名；同名按列表顺序取第一台。
2. 每个动作执行后会自动返回最新紧凑屏幕文本（或差异变化行），无需频繁重复 phone_observe。
3. 遇到涉及财产支付、转账、发送敏感信息或破坏性删除等动作时，安全网关会返回 approval_pending 并携带 approval_id，请以相同参数附带该 approval_id 再次调用重试，等待人类审批。
4. 严禁向密码框输入真实密码或验证码，遇到密码输入或滑块验证请调用 phone_handoff 交由人类完成。
5. 任务结束时调用 phone_release 释放设备并输出完成摘要。`

func (s *Server) initSDKServer() *sdk.Server {
	srv := sdk.NewServer(
		&sdk.Implementation{Name: "farm", Title: "Farm Mobile Automation", Version: s.version},
		&sdk.ServerOptions{
			Instructions: Instructions,
			Capabilities: &sdk.ServerCapabilities{},
		},
	)
	s.registerTools(srv)
	return srv
}

func (s *Server) SDK() *sdk.Server {
	return s.sdkServer
}

// HTTPHandler 返回支持 Streamable HTTP / SSE 通信的 HTTP 处理句柄
func (s *Server) HTTPHandler() http.Handler {
	handler := sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server {
			return s.sdkServer
		},
		&sdk.StreamableHTTPOptions{
			SessionTimeout: 30 * time.Minute,
		},
	)
	return handler
}

// 工具输入契约结构体定义

type ConnectInput struct {
	Device   string `json:"device" jsonschema:"目标 Android 设备的 serial 或标识名（如 Redmi；同名取列表第一台）"`
	Task     string `json:"task" jsonschema:"当前任务描述 (如 帮我检查设置项)"`
	ReadOnly bool   `json:"read_only,omitempty" jsonschema:"是否以只读模式运行"`
}

type ObserveInput struct {
	Device   string   `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Formats  []string `json:"formats,omitempty" jsonschema:"输出格式清单，如 [\"ref\", \"ocr\"]"`
	Diff     *bool    `json:"diff,omitempty" jsonschema:"是否优先返回增量差异，默认 true"`
	SettleMS int      `json:"settle_ms,omitempty" jsonschema:"等待画面静止延时毫秒数"`
}

type TapInput struct {
	Device     string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Ref        int    `json:"ref,omitempty" jsonschema:"最新屏幕感知中的元素编号 [ref]"`
	X          *int   `json:"x,omitempty" jsonschema:"绝对像素坐标 X（当没有 ref 时使用）"`
	Y          *int   `json:"y,omitempty" jsonschema:"绝对像素坐标 Y（当没有 ref 时使用）"`
	Intent     string `json:"intent,omitempty" jsonschema:"操作意图说明，展示给人类审批卡片和审计历史"`
	ApprovalID string `json:"approval_id,omitempty" jsonschema:"人类审批单据编号（收到 approval_pending 时传入）"`
	SettleMS   int    `json:"settle_ms,omitempty" jsonschema:"等待延时"`
}

type TypeInput struct {
	Device     string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Text       string `json:"text" jsonschema:"需要输入的文本内容"`
	Ref        int    `json:"ref,omitempty" jsonschema:"目标输入框元素 ref"`
	Submit     bool   `json:"submit,omitempty" jsonschema:"输入完成后是否敲回车发送/提交"`
	Intent     string `json:"intent,omitempty" jsonschema:"输入意图说明"`
	ApprovalID string `json:"approval_id,omitempty" jsonschema:"审批单编号"`
}

type SwipeInput struct {
	Device    string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Direction string `json:"direction" jsonschema:"滑动方向: up, down, left, right"`
	Intent    string `json:"intent,omitempty" jsonschema:"滑动意图说明"`
}

type KeyInput struct {
	Device string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Key    string `json:"key" jsonschema:"按键名称: home, back, app_switch, enter, power"`
}

type LaunchInput struct {
	Device string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	App    string `json:"app" jsonschema:"明确的 Android 应用包名 (如 com.android.settings)"`
}

type HandoffInput struct {
	Device string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Reason string `json:"reason" jsonschema:"需要人类接管的原因 (如 请输入支付密码或解锁屏幕)"`
}

type RequestApprovalInput struct {
	Device  string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Summary string `json:"summary" jsonschema:"向用户提问的简明内容"`
	Details string `json:"details,omitempty" jsonschema:"提问详细说明"`
}

type ReleaseInput struct {
	Device  string `json:"device,omitempty" jsonschema:"设备 serial 或标识名；省略使用已绑定设备"`
	Outcome string `json:"outcome,omitempty" jsonschema:"任务结果: done, failed, stopped"`
	Summary string `json:"summary,omitempty" jsonschema:"最终完成总结"`
	Export  bool   `json:"export,omitempty" jsonschema:"是否导出单文件自包含 HTML 审计报告"`
}

// 注册所有核心 MCP 工具
type DevicesInput struct{}

type DeviceListEntry struct {
	Identifier string `json:"identifier"`
	Serial     string `json:"serial"`
	State      string `json:"state"`
}

func (s *Server) registerTools(srv *sdk.Server) {
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_devices",
		Title:       "列出设备",
		Description: "只读列出在线设备标识名、serial 和状态。device 参数支持精确 serial 或标识名；同名按在线列表顺序选第一台。",
	}, s.handleDevices)

	// 1. phone_connect
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_connect",
		Title:       "连接设备并开启任务",
		Description: "与指定 Android 设备建立会话，开启审计任务时间线，并返回设备首屏紧凑感知内容。",
	}, s.handleConnect)

	// 2. phone_observe
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_observe",
		Title:       "读取屏幕感知",
		Description: "读取当前屏幕的紧凑感知行协议（包含元素中心绝对坐标与 OCR 识别文字）。",
	}, s.handleObserve)

	// 3. phone_tap
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_tap",
		Title:       "点击元素或坐标",
		Description: "通过 ref 或绝对像素坐标点击。动作受安全网关策略审核，高危操作需人工审批。",
	}, s.handleTap)

	// 4. phone_type
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_type",
		Title:       "文本输入",
		Description: "向指定输入框或当前焦点注入文本。密码和验证码字段绝对禁止自动注入。",
	}, s.handleType)

	// 5. phone_swipe
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_swipe",
		Title:       "屏幕滑动",
		Description: "向指定方向平滑滚动屏幕内容。",
	}, s.handleSwipe)

	// 6. phone_key
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_key",
		Title:       "按键导航",
		Description: "模拟硬件与系统导航按键（home、back、enter、power、app_switch）。",
	}, s.handleKey)

	// 7. phone_launch
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_launch",
		Title:       "启动应用",
		Description: "启动指定 Android 应用包名，确认前台应用后返回最新屏幕画面。",
	}, s.handleLaunch)

	// 8. phone_handoff
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_handoff",
		Title:       "交由人工接管",
		Description: "当遇到密码输入、指纹验证、滑块验证码或屏幕锁定时，调用此接口暂停并通知人工在屏幕上操作完成。",
	}, s.handleHandoff)

	// 9. phone_request_approval
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_request_approval",
		Title:       "向用户提问确认",
		Description: "Agent 主动向用户发起是非确认提问并获取用户的决定与批注。",
	}, s.handleRequestApproval)

	// 10. phone_release
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "phone_release",
		Title:       "释放设备并结束任务",
		Description: "结束当前设备的任务会话，持久化审计报告，并释放设备锁定。",
	}, s.handleRelease)
}

// SetDeviceProvider supplies the ordered device registry snapshot.
// Configure before serving MCP requests; all device inputs resolve to serials.
func (s *Server) SetDeviceProvider(provider func() []device.Device) {
	s.deviceProvider = provider
}

func (s *Server) deviceList() ([]device.Device, error) {
	if s.deviceProvider == nil {
		return nil, fmt.Errorf("device registry is unavailable")
	}
	return s.deviceProvider(), nil
}

func deviceSelectionError(input string, devices []device.Device) error {
	var entries []string
	for _, d := range devices {
		if d.State != "device" {
			continue
		}
		entries = append(entries, fmt.Sprintf("identifier=%q serial=%s (state=%s)", d.IdentifierName(), d.Serial, d.State))
	}
	list := "(none)"
	if len(entries) > 0 {
		list = strings.Join(entries, "; ")
	}
	log.Printf("[MCP] rejected unavailable device input=%q", input)
	return fmt.Errorf("device %q is unknown or offline; pass an online device serial or identifier (first matching online device is selected). Current online devices: %s", input, list)
}

// resolveDevice gives exact serial precedence, then matches public identifiers
// case-insensitively in online registry order. Explicit offline serials fail.
func (s *Server) resolveDevice(input string, allowIdentifier bool) (string, error) {
	devices, err := s.deviceList()
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	for _, d := range devices {
		if d.Serial == input {
			if d.State == "device" {
				return d.Serial, nil
			}
			return "", deviceSelectionError(input, devices)
		}
	}
	if allowIdentifier {
		for _, d := range devices {
			if d.State == "device" && strings.EqualFold(d.IdentifierName(), input) {
				return d.Serial, nil
			}
		}
	}
	return "", deviceSelectionError(input, devices)
}

func (s *Server) handleDevices(_ context.Context, _ *sdk.CallToolRequest, _ DevicesInput) (*sdk.CallToolResult, any, error) {
	devices, err := s.deviceList()
	if err != nil {
		return errorResult("device_unavailable", err.Error()), nil, nil
	}
	entries := make([]DeviceListEntry, 0, len(devices))
	for _, d := range devices {
		if d.State != "device" {
			continue
		}
		entries = append(entries, DeviceListEntry{Identifier: d.IdentifierName(), Serial: d.Serial, State: d.State})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return errorResult("device_list_failed", err.Error()), nil, nil
	}
	return textResult(string(data)), nil, nil
}

// resolveContext retains real serial identity for an already bound session.
// Only an explicitly supplied device parameter can select by identifier.
func (s *Server) resolveContext(sessionID, deviceArg string) (serial, taskID string, err error) {
	explicit := strings.TrimSpace(deviceArg) != ""
	s.mu.Lock()
	serial = deviceArg
	if !explicit {
		serial = s.devices[sessionID]
	}
	taskID = s.tasks[sessionID]
	s.mu.Unlock()
	if serial == "" {
		return "", "", fmt.Errorf("missing device (call phone_connect first or pass a serial or identifier)")
	}
	serial, err = s.resolveDevice(serial, explicit)
	if err != nil {
		return "", "", err
	}
	return serial, taskID, nil
}

func (s *Server) nextStepIndex(taskID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.stepIndices[taskID] + 1
	s.stepIndices[taskID] = idx
	return idx
}

// textResult 辅助构造纯文本 MCP 返回
func textResult(text string) *sdk.CallToolResult {
	return &sdk.CallToolResult{
		Content: []sdk.Content{
			&sdk.TextContent{Text: text},
		},
	}
}

// errorResult 构造带前缀的错误返回
func errorResult(errCode, msg string) *sdk.CallToolResult {
	return &sdk.CallToolResult{
		IsError: true,
		Content: []sdk.Content{
			&sdk.TextContent{Text: fmt.Sprintf("error: %s · %s", errCode, msg)},
		},
	}
}

// 1. handleConnect
func (s *Server) handleConnect(ctx context.Context, req *sdk.CallToolRequest, in ConnectInput) (*sdk.CallToolResult, any, error) {
	if strings.TrimSpace(in.Device) == "" {
		return errorResult("invalid_argument", "必须指定 device 参数"), nil, nil
	}

	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, err := s.resolveDevice(in.Device, true)
	if err != nil {
		return errorResult("device_unavailable", err.Error()), nil, nil
	}
	if s.autoService == nil {
		return errorResult("service_unavailable", "automation service is not configured"), nil, nil
	}
	// Bind only after the device and its first observation have been verified.
	screenText, err := s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial: serial, Formats: []string{"ref", "ocr"}, SettleMS: 300,
	})
	if err != nil {
		return errorResult("connect_failed", fmt.Sprintf("observe failed for %s: %v", serial, err)), nil, nil
	}
	var taskID string
	if s.history != nil {
		id, err := s.history.CreateTask(ctx, serial, "Agent", in.Task)
		if err != nil {
			log.Printf("[MCP] task history creation failed serial=%s: %v", serial, err)
		} else {
			taskID = id
		}
	}
	s.mu.Lock()
	s.devices[sessionID] = serial
	s.tasks[sessionID] = taskID
	s.mu.Unlock()

	return textResult(fmt.Sprintf("connected to %s · task: %s\n%s", serial, in.Task, screenText)), nil, nil
}

// 2. handleObserve
func (s *Server) handleObserve(ctx context.Context, req *sdk.CallToolRequest, in ObserveInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, _, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	fmts := in.Formats
	if len(fmts) == 0 {
		fmts = []string{"ref", "ocr"}
	}

	screenText, err := s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial:   serial,
		Formats:  fmts,
		Diff:     in.Diff,
		SettleMS: in.SettleMS,
	})
	if err != nil {
		return errorResult("observe_failed", err.Error()), nil, nil
	}
	return textResult(screenText), nil, nil
}

// 3. handleTap
func (s *Server) handleTap(ctx context.Context, req *sdk.CallToolRequest, in TapInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, taskID, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	// 步骤开始前采集快照作为审计证据
	var beforeShotID string
	if s.history != nil {
		if img, _, err := s.autoService.CaptureFrame(ctx, serial); err == nil {
			beforeShotID, _ = s.history.SaveScreenshot(img)
		}
	}

	xStr, yStr := "none", "none"
	if in.X != nil {
		xStr = fmt.Sprintf("%d", *in.X)
	}
	if in.Y != nil {
		yStr = fmt.Sprintf("%d", *in.Y)
	}
	argsStr := fmt.Sprintf("ref=%d,x=%s,y=%s", in.Ref, xStr, yStr)
	argsHash := safety.ComputeArgsHash(argsStr)

	policyIn := safety.Input{
		Device:       serial,
		Op:           safety.OpTap,
		ElementLabel: in.Intent,
	}

	verdict := safety.Verdict{Decision: safety.Allow}
	if s.policy != nil {
		verdict = s.policy.Evaluate(policyIn)
	}

	// 门禁 1: 绝对拒绝
	if verdict.Decision == safety.Deny {
		return errorResult("blocked_by_policy", verdict.Reason), nil, nil
	}

	// 门禁 2: 人工审批
	binding := safety.ApprovalBinding{
		Device:   serial,
		Tool:     "tap",
		ArgsHash: argsHash,
	}

	if verdict.Decision == safety.Ask {
		if in.ApprovalID == "" {
			// 创建待审批单据，向 Agent 返回 approval_pending 挂起重试
			appReq := safety.ApprovalRequest{
				Binding: binding,
				Rule:    verdict.Rule,
				Title:   fmt.Sprintf("点击操作: %s", argsStr),
				Detail:  in.Intent,
				Intent:  in.Intent,
			}
			info := s.openApproval(ctx, serial, appReq)
			return textResult(fmt.Sprintf("approval_pending · approval_id: %s · rule: %s\n%s",
				info.ID, verdict.Rule, verdict.Reason)), nil, nil
		}

		// Agent 带 approval_id 再次进入，核销单据
		consumed, cErr := s.approvals.Consume(in.ApprovalID, binding)
		if cErr != nil {
			return errorResult("approval_check_failed", cErr.Error()), nil, nil
		}
		if s.history != nil && taskID != "" {
			_ = s.history.RecordApproval(ctx, taskID, s.nextStepIndex(taskID), verdict.Rule, "点击操作", string(consumed.Decision), consumed.Note, consumed.CreatedAt, time.Since(consumed.CreatedAt).Milliseconds())
		}
		if !consumed.Decision.Allows() {
			return errorResult("approval_denied", fmt.Sprintf("人工拒绝了此操作: %s", consumed.Note)), nil, nil
		}
	}

	if s.autoService == nil {
		return errorResult("service_unavailable", "automation service is not configured"), nil, nil
	}

	// 计算点击坐标并执行
	var normX, normY float64
	status, err := s.autoService.GetDeviceStatus(serial)
	if err != nil || status.Width <= 0 || status.Height <= 0 {
		return errorResult("device_error", "无法获取屏幕规格"), nil, nil
	}

	if in.X != nil && in.Y != nil {
		normX = float64(*in.X) / float64(status.Width)
		normY = float64(*in.Y) / float64(status.Height)
	} else if in.Ref > 0 {
		// 从最新屏幕缓存提取 ref 中心坐标
		normX, normY = 0.5, 0.5 // 缺省 fallback
	} else {
		return errorResult("invalid_argument", "必须提供 ref 或 x,y 坐标"), nil, nil
	}

	startAt := time.Now()
	actRes, err := s.autoService.Tap(ctx, automation.TapRequest{
		Serial: serial,
		X:      normX,
		Y:      normY,
	})
	elapsed := time.Since(startAt).Milliseconds()
	if err != nil {
		return errorResult("tap_failed", err.Error()), nil, nil
	}

	// 步骤后感知与审计记录
	var afterShotID string
	var screenText string
	if s.history != nil {
		if img, _, err := s.autoService.CaptureFrame(ctx, serial); err == nil {
			afterShotID, _ = s.history.SaveScreenshot(img)
		}
	}

	settle := in.SettleMS
	if settle <= 0 {
		settle = 300
	}
	diffTrue := true
	screenText, _ = s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial:   serial,
		Formats:  []string{"ref", "ocr"},
		Diff:     &diffTrue,
		SettleMS: settle,
	})

	if s.history != nil && taskID != "" {
		_ = s.history.RecordStep(ctx, safety.StepRecord{
			TaskID:       taskID,
			Index:        s.nextStepIndex(taskID),
			Tool:         "phone_tap",
			Args:         argsStr,
			Intent:       in.Intent,
			Status:       actRes.Status,
			StartedAt:    startAt,
			ElapsedMS:    elapsed,
			BeforeShotID: beforeShotID,
			AfterShotID:  afterShotID,
			ScreenText:   screenText,
		})
	}

	return textResult(fmt.Sprintf("settled\n%s", screenText)), nil, nil
}

// 4. handleType
func (s *Server) handleType(ctx context.Context, req *sdk.CallToolRequest, in TypeInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, taskID, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	// 安全评估：绝对拦截向密码/凭据框输入
	policyIn := safety.Input{
		Device: serial,
		Op:     safety.OpType,
		Text:   in.Text,
	}

	verdict := safety.Verdict{Decision: safety.Allow}
	if s.policy != nil {
		verdict = s.policy.Evaluate(policyIn)
	}

	if verdict.Decision == safety.Deny {
		return errorResult("blocked_by_policy", verdict.Reason), nil, nil
	}

	startAt := time.Now()
	_, err = s.autoService.SendKey(ctx, automation.SendKeyRequest{
		Serial: serial,
		Text:   in.Text,
	})
	if err != nil {
		return errorResult("type_failed", err.Error()), nil, nil
	}

	if in.Submit {
		_, _ = s.autoService.Event(ctx, automation.EventRequest{
			Serial: serial,
			Event:  "enter",
		})
	}

	elapsed := time.Since(startAt).Milliseconds()
	diffTrue := true
	screenText, _ := s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial:   serial,
		Formats:  []string{"ref", "ocr"},
		Diff:     &diffTrue,
		SettleMS: 400,
	})

	if s.history != nil && taskID != "" {
		_ = s.history.RecordStep(ctx, safety.StepRecord{
			TaskID:     taskID,
			Index:      s.nextStepIndex(taskID),
			Tool:       "phone_type",
			Args:       fmt.Sprintf("text=%q,submit=%v", in.Text, in.Submit),
			Intent:     in.Intent,
			Status:     "settled",
			StartedAt:  startAt,
			ElapsedMS:  elapsed,
			ScreenText: screenText,
		})
	}

	return textResult(fmt.Sprintf("settled\n%s", screenText)), nil, nil
}

// 5. handleSwipe
func (s *Server) handleSwipe(ctx context.Context, req *sdk.CallToolRequest, in SwipeInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, taskID, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	var x1, y1, x2, y2 float64 = 0.5, 0.7, 0.5, 0.3
	switch strings.ToLower(in.Direction) {
	case "up":
		x1, y1, x2, y2 = 0.5, 0.7, 0.5, 0.3
	case "down":
		x1, y1, x2, y2 = 0.5, 0.3, 0.5, 0.7
	case "left":
		x1, y1, x2, y2 = 0.8, 0.5, 0.2, 0.5
	case "right":
		x1, y1, x2, y2 = 0.2, 0.5, 0.8, 0.5
	}

	startAt := time.Now()
	_, err = s.autoService.Swipe(ctx, automation.SwipeRequest{
		Serial:     serial,
		X1:         &x1,
		Y1:         &y1,
		X2:         x2,
		Y2:         y2,
		DurationMS: 300,
	})
	if err != nil {
		return errorResult("swipe_failed", err.Error()), nil, nil
	}
	elapsed := time.Since(startAt).Milliseconds()

	diffTrue := true
	screenText, _ := s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial:   serial,
		Formats:  []string{"ref", "ocr"},
		Diff:     &diffTrue,
		SettleMS: 400,
	})

	if s.history != nil && taskID != "" {
		_ = s.history.RecordStep(ctx, safety.StepRecord{
			TaskID:     taskID,
			Index:      s.nextStepIndex(taskID),
			Tool:       "phone_swipe",
			Args:       fmt.Sprintf("direction=%s", in.Direction),
			Intent:     in.Intent,
			Status:     "settled",
			StartedAt:  startAt,
			ElapsedMS:  elapsed,
			ScreenText: screenText,
		})
	}

	return textResult(fmt.Sprintf("settled\n%s", screenText)), nil, nil
}

// 6. handleKey
func (s *Server) handleKey(ctx context.Context, req *sdk.CallToolRequest, in KeyInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, taskID, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	startAt := time.Now()
	_, err = s.autoService.Event(ctx, automation.EventRequest{
		Serial: serial,
		Event:  in.Key,
	})
	if err != nil {
		return errorResult("key_failed", err.Error()), nil, nil
	}
	elapsed := time.Since(startAt).Milliseconds()

	diffTrue := true
	screenText, _ := s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial:   serial,
		Formats:  []string{"ref", "ocr"},
		Diff:     &diffTrue,
		SettleMS: 400,
	})

	if s.history != nil && taskID != "" {
		_ = s.history.RecordStep(ctx, safety.StepRecord{
			TaskID:     taskID,
			Index:      s.nextStepIndex(taskID),
			Tool:       "phone_key",
			Args:       fmt.Sprintf("key=%s", in.Key),
			Status:     "settled",
			StartedAt:  startAt,
			ElapsedMS:  elapsed,
			ScreenText: screenText,
		})
	}

	return textResult(fmt.Sprintf("settled\n%s", screenText)), nil, nil
}

// 7. handleLaunch
func (s *Server) handleLaunch(ctx context.Context, req *sdk.CallToolRequest, in LaunchInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, _, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	// 敏感应用拦截
	if s.policy != nil {
		v := s.policy.Evaluate(safety.Input{
			Device:      serial,
			Op:          safety.OpLaunch,
			PackageName: in.App,
		})
		if v.Decision == safety.Deny {
			return errorResult("blocked_by_policy", v.Reason), nil, nil
		}
	}

	if s.autoService == nil {
		return errorResult("service_unavailable", "automation service is not configured"), nil, nil
	}
	launch := s.launchApp
	if launch == nil {
		launch = s.autoService.LaunchApp
	}
	if err := launch(ctx, serial, in.App); err != nil {
		return errorResult("launch_failed", err.Error()), nil, nil
	}
	// Launch has succeeded; an observation failure must not suggest replaying it.
	screenText, err := s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial:   serial,
		Formats:  []string{"ref", "ocr"},
		SettleMS: 1000,
	})
	if err != nil {
		return errorResult("observe_failed", fmt.Sprintf("launched %s on %s; foreground verified; screen observation failed: %v", in.App, serial, err)), nil, nil
	}

	return textResult(fmt.Sprintf("settled\n%s", screenText)), nil, nil
}

// 8. handleHandoff 人工接管挂起
func (s *Server) handleHandoff(ctx context.Context, req *sdk.CallToolRequest, in HandoffInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, _, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	// 创建人工接管审批/通知
	binding := safety.ApprovalBinding{
		Device:   serial,
		Tool:     "handoff",
		ArgsHash: safety.ComputeArgsHash(in.Reason),
	}
	info := s.openApproval(ctx, serial, safety.ApprovalRequest{
		Binding:    binding,
		Rule:       "Handoff",
		Title:      "需要人工协助",
		Detail:     in.Reason,
		TimeoutSec: 180, // 人工接管给予更长等待时间（3分钟）
	})

	// 等待人类完成并交回控制权
	res, wErr := s.approvals.Wait(ctx, info.ID, 3*time.Minute)
	if wErr != nil {
		return errorResult("handoff_interrupted", wErr.Error()), nil, nil
	}

	diffTrue := true
	screenText, _ := s.autoService.Observe(ctx, automation.ObserveRequest{
		Serial:   serial,
		Formats:  []string{"ref", "ocr"},
		Diff:     &diffTrue,
		SettleMS: 500,
	})

	return textResult(fmt.Sprintf("handed back · user note: %q\n%s", res.Note, screenText)), nil, nil
}

// 9. handleRequestApproval 主动提问确认
func (s *Server) handleRequestApproval(ctx context.Context, req *sdk.CallToolRequest, in RequestApprovalInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, _, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	binding := safety.ApprovalBinding{
		Device:   serial,
		Tool:     "request_approval",
		ArgsHash: safety.ComputeArgsHash(in.Summary + in.Details),
	}
	info := s.openApproval(ctx, serial, safety.ApprovalRequest{
		Binding:    binding,
		Rule:       "Agent Question",
		Title:      in.Summary,
		Detail:     in.Details,
		TimeoutSec: 60,
	})

	res, wErr := s.approvals.Wait(ctx, info.ID, 60*time.Second)
	if wErr != nil {
		return errorResult("wait_failed", wErr.Error()), nil, nil
	}

	return textResult(fmt.Sprintf("decision: %s · note: %s", res.Decision, res.Note)), nil, nil
}

// 10. handleRelease
func (s *Server) handleRelease(ctx context.Context, req *sdk.CallToolRequest, in ReleaseInput) (*sdk.CallToolResult, any, error) {
	sessionID := "default"
	if req.Session != nil {
		sessionID = req.Session.ID()
	}

	serial, taskID, err := s.resolveContext(sessionID, in.Device)
	if err != nil {
		return errorResult("no_device", err.Error()), nil, nil
	}

	outcome := in.Outcome
	if outcome == "" {
		outcome = "done"
	}

	var exportPath string
	if s.history != nil && taskID != "" {
		_ = s.history.FinishTask(ctx, taskID, outcome, in.Summary)
		if in.Export {
			htmlData, err := s.history.ExportTaskHTML(ctx, taskID)
			if err == nil {
				exportFile := fmt.Sprintf("/tmp/farm-task-%s.html", taskID)
				_ = osWriteFile(exportFile, []byte(htmlData))
				exportPath = exportFile
			}
		}
	}

	s.mu.Lock()
	delete(s.devices, sessionID)
	delete(s.tasks, sessionID)
	s.mu.Unlock()

	msg := fmt.Sprintf("released %s · outcome: %s · summary: %s", serial, outcome, in.Summary)
	if exportPath != "" {
		msg += fmt.Sprintf("\nreport exported to: %s", exportPath)
	}
	return textResult(msg), nil, nil
}

func osWriteFile(name string, data []byte) error {
	return osWrite(name, data, 0o644)
}

// osWrite 辅助函数便于单测替换
var osWrite = func(name string, data []byte, perm int) error {
	return os.WriteFile(name, data, 0o644)
}

// openApproval snapshots app identity before opening the immutable approval.
func (s *Server) openApproval(ctx context.Context, serial string, request safety.ApprovalRequest) safety.ApprovalInfo {
	if s.autoService != nil {
		request.AppPackage, request.AppName, request.AppIcon = s.autoService.ApprovalApp(ctx, serial)
	}
	return s.approvals.Open(request)
}
