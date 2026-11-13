package automation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"farm/internal/action"
	"farm/internal/device"
	"farm/internal/scrcpy"
)

const actTimeout = 60 * time.Second

var actClipboardSequence atomic.Uint64

type actTransport interface {
	SubmitAction(context.Context, string, action.Request) (action.Result, error)
}

// ActRequest keeps the query serial separate from the action and opaque params.
// Params is never evaluated by a host shell.
type ActRequest struct {
	Action string `json:"action"`
	Device string `json:"device"`
	Params string `json:"params"`
}

type ActResponse struct {
	Action    string `json:"action"`
	Device    string `json:"device"`
	Status    string `json:"status"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Result    any    `json:"result,omitempty"`
	Code      string `json:"code,omitempty"`
	Error     string `json:"error,omitempty"`
}

type actCheckResult struct {
	Matched bool
	Source  string
}

type actPlan struct {
	points    [][2]int
	keys      []uint32
	duration  int64
	keyword   string
	args      []string
	directory string
}

// SetDeviceProvider supplies the same ordered registry used by the MCP tools.
// Configure it before serving requests.
func (s *Service) SetDeviceProvider(provider func() []device.Device) {
	s.deviceProvider = provider
}

func (s *Service) resolveActDevice(input string) (string, error) {
	if s.deviceProvider == nil {
		return "", fmt.Errorf("device registry is unavailable")
	}
	devices := s.deviceProvider()
	for _, d := range devices {
		if d.Serial == input {
			if d.State != "device" {
				return "", fmt.Errorf("device %q is offline", input)
			}
			return d.Serial, nil
		}
	}
	return "", fmt.Errorf("serial %q is unknown or offline; read /api/automation/devices", input)
}

func (s *Service) handleAutomationDevices(w http.ResponseWriter, r *http.Request) {
	if s.deviceProvider == nil {
		writeAutomationText(w, http.StatusServiceUnavailable, "错误 device_registry_unavailable: 设备注册表不可用。\n")
		return
	}
	var out strings.Builder
	count := 0
	for _, d := range s.deviceProvider() {
		if d.State != "device" {
			continue
		}
		fmt.Fprintf(&out, "设备 %q · serial %q · 在线\n", d.IdentifierName(), d.Serial)
		count++
	}
	if count == 0 {
		out.WriteString("当前没有在线设备。\n")
	}
	s.recordAutomationLog(AutomationLog{Type: LogTypeResponse, Endpoint: r.URL.Path, Method: r.Method,
		Status: http.StatusOK, Message: "返回自动化在线设备列表", Fields: AutomationLogFields("count", count)})
	writeAutomationText(w, http.StatusOK, out.String())
}

func writeAutomationText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, text)
}

type actStep struct {
	line     int
	command  string
	req      ActRequest
	plan     actPlan
	response ActResponse
}

type actBatchResponse struct {
	taskID     string
	title      string
	device     string
	status     string
	completed  int
	total      int
	failedLine int
	code       string
	err        error
	steps      []actStep
}

func formatActBatch(resp actBatchResponse) string {
	var out strings.Builder
	status := "失败"
	if resp.status == "executed" {
		status = "已完成"
	}
	if resp.title != "" {
		fmt.Fprintf(&out, "任务 %q\n", resp.title)
	}
	if resp.taskID != "" {
		fmt.Fprintf(&out, "任务 ID %s\n", resp.taskID)
	}
	fmt.Fprintf(&out, "设备 %q\n状态 %s\n完成 %d/%d\n", resp.device, status, resp.completed, resp.total)
	names := map[string]string{"failed": "失败", "executed": "已完成", "matched": "命中", "not_matched": "未命中"}
	for _, step := range resp.steps {
		r := step.response
		fmt.Fprintf(&out, "第 %d 行 %s：%s（%d/%d）\n", step.line, step.req.Action, names[r.Status], r.Completed, r.Total)
		switch result := r.Result.(type) {
		case string:
			out.WriteString("结果\n")
			out.WriteString(result)
			if !strings.HasSuffix(result, "\n") {
				out.WriteByte('\n')
			}
		case actCheckResult:
			target := step.req.Params
			if step.req.Action == "wait" {
				target = step.plan.keyword
			}
			fmt.Fprintf(&out, "目标 %q\n", target)
			if result.Matched {
				fmt.Fprintf(&out, "匹配来源 %s\n", result.Source)
			}
		}
	}
	if resp.failedLine > 0 {
		fmt.Fprintf(&out, "失败行 %d\n", resp.failedLine)
	}
	if resp.err != nil {
		fmt.Fprintf(&out, "错误 %s: %q\n", resp.code, resp.err.Error())
	}
	return out.String()
}

// parseActText reads one action token and preserves all remaining params bytes.
// The device is supplied separately through the serial query parameter.
func parseActText(body string) (ActRequest, error) {
	body = strings.TrimLeft(body, " \t\r\n")
	if body == "" {
		return ActRequest{}, fmt.Errorf("request must contain an action")
	}
	end := strings.IndexAny(body, " \t\r\n")
	if end < 0 {
		return ActRequest{Action: body}, nil
	}
	params := body[end:]
	if strings.HasPrefix(params, "\r\n") {
		params = params[2:]
	} else {
		params = params[1:]
	}
	return ActRequest{Action: body[:end], Params: params}, nil
}

func parseActPoint(value string) ([2]int, error) {
	var point [2]int
	if !strings.HasPrefix(value, "@") {
		return point, fmt.Errorf("coordinate must use @X,Y pixels")
	}
	parts := strings.Split(strings.TrimPrefix(value, "@"), ",")
	if len(parts) != 2 {
		return point, fmt.Errorf("coordinate must use @X,Y pixels")
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 100000 {
			return point, fmt.Errorf("coordinate must contain nonnegative integer pixels")
		}
		point[i] = n
	}
	return point, nil
}

// planAct validates an entire sequence before any device interaction occurs.
func planAct(req ActRequest) (actPlan, error) {
	var plan actPlan
	if strings.TrimSpace(req.Device) == "" {
		return plan, fmt.Errorf("serial query parameter is required")
	}
	if len(req.Device) > 512 || len(req.Params) > 16384 {
		return plan, fmt.Errorf("device or params exceeds size limit")
	}
	switch req.Action {
	case "tap", "swipe":
		parts := strings.Fields(req.Params)
		if req.Action == "tap" && len(parts) != 1 {
			return plan, fmt.Errorf("tap params must be @X,Y")
		}
		if req.Action == "swipe" && len(parts) != 2 && len(parts) != 3 {
			return plan, fmt.Errorf("swipe params must be @X1,Y1 @X2,Y2 [duration_ms]")
		}
		count := 1
		if req.Action == "swipe" {
			count = 2
		}
		for _, value := range parts[:count] {
			point, err := parseActPoint(value)
			if err != nil {
				return plan, err
			}
			plan.points = append(plan.points, point)
		}
		plan.duration = 300
		if len(parts) == 3 {
			ms, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || ms < 80 || ms > 10000 {
				return plan, fmt.Errorf("swipe duration_ms must be 80..10000")
			}
			plan.duration = ms
		}
	case "keyevent":
		parts := strings.Split(strings.NewReplacer("｜", "|", ",", "|", "，", "|").Replace(req.Params), "|")
		if len(parts) > 32 {
			return plan, fmt.Errorf("keyevent supports at most 32 keys")
		}
		for _, name := range parts {
			code, ok := NamedEvents[strings.TrimSpace(name)]
			if !ok {
				return plan, fmt.Errorf("invalid keyevent name %q", name)
			}
			plan.keys = append(plan.keys, code)
		}
	case "sendkey", "set-copy", "check":
		if req.Params == "" || (req.Action == "check" && strings.TrimSpace(req.Params) == "") {
			return plan, fmt.Errorf("%s params must be nonempty text", req.Action)
		}
	case "copy", "paste", "clear":
		if strings.TrimSpace(req.Params) != "" {
			return plan, fmt.Errorf("%s does not accept parameters", req.Action)
		}
	case "adb":
		args, err := parseActADB(req.Params)
		if err != nil {
			return plan, err
		}
		plan.args = args
	case "mkdir", "screenshot", "record", "rm":
		directory := strings.TrimSpace(req.Params)
		if req.Action == "record" {
			end := strings.IndexAny(directory, " \t")
			if end < 0 {
				return plan, fmt.Errorf("record params must be seconds relative_directory")
			}
			seconds, err := strconv.ParseInt(directory[:end], 10, 64)
			if err != nil || seconds < 1 || seconds > 30 {
				return plan, fmt.Errorf("record seconds must be 1..30")
			}
			plan.duration = seconds
			directory = strings.TrimSpace(directory[end:])
		}
		normalized, err := deviceArtifactDirectory(directory)
		if err != nil {
			return plan, err
		}
		if req.Action == "rm" && normalized == "." {
			return plan, fmt.Errorf("rm may not delete the device artifact root")
		}
		plan.directory = normalized
	case "sleep":
		ms, err := strconv.ParseInt(strings.TrimSpace(req.Params), 10, 64)
		if err != nil || ms < 0 || ms > 30000 {
			return plan, fmt.Errorf("sleep params must be milliseconds in 0..30000")
		}
		plan.duration = ms
	case "wait":
		params := strings.TrimSpace(req.Params)
		end := strings.IndexAny(params, " \t")
		if end < 0 {
			return plan, fmt.Errorf("wait params must be timeout_ms keyword")
		}
		ms, err := strconv.ParseInt(params[:end], 10, 64)
		keyword := strings.TrimLeft(params[end:], " \t")
		if err != nil || ms <= 0 || ms > 30000 || keyword == "" {
			return plan, fmt.Errorf("wait requires timeout_ms in 1..30000 and nonempty keyword")
		}
		plan.duration, plan.keyword = ms, keyword
	default:
		return plan, fmt.Errorf("action must be tap, keyevent, sendkey, set-copy, copy, paste, clear, swipe, check, wait, sleep, adb, mkdir, screenshot, record or rm")
	}
	return plan, nil
}

// parseActBatch validates every operation before any device provider, frame,
// page or action is touched. Empty lines do not count toward the 32-step limit.
func parseActBatch(body, serial string) ([]actStep, int, error) {
	var steps []actStep
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(steps) >= 32 {
			return nil, i + 1, fmt.Errorf("at most 32 operation lines are allowed")
		}
		req, err := parseActText(line)
		if err != nil {
			return nil, i + 1, err
		}
		req.Device = serial
		plan, err := planAct(req)
		if err != nil {
			return nil, i + 1, err
		}
		steps = append(steps, actStep{line: i + 1, command: line, req: req, plan: plan})
	}
	if len(steps) == 0 {
		return nil, 0, fmt.Errorf("request must contain an operation")
	}
	return steps, 0, nil
}

// parseActTitle validates the sole title query value before any body read or
// device interaction. The caller supplies the title; the server never invents it.
func parseActTitle(rawQuery string) (title, code string, err error) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", "invalid_title", fmt.Errorf("invalid query encoding: %w", err)
	}
	values, exists := query["title"]
	if !exists {
		return "", "missing_title", fmt.Errorf("title query parameter is required")
	}
	if len(values) != 1 {
		return "", "invalid_title", fmt.Errorf("title must occur exactly once")
	}
	title = values[0]
	if !utf8.ValidString(title) {
		return "", "invalid_title", fmt.Errorf("title must be valid UTF-8")
	}
	for _, char := range title {
		if unicode.IsControl(char) {
			return "", "invalid_title", fmt.Errorf("title must not contain control characters")
		}
	}
	title = strings.TrimSpace(title)
	if length := utf8.RuneCountInString(title); length < 1 || length > 120 {
		return "", "invalid_title", fmt.Errorf("title must contain 1..120 Unicode characters after trimming")
	}
	return title, "", nil
}

func (s *Service) handleAct(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	ctx := r.Context()
	resp := actBatchResponse{device: r.URL.Query().Get("serial"), status: "failed"}
	httpStatus := http.StatusOK
	fail := func(status int, code string, line int, err error) {
		httpStatus, resp.code, resp.failedLine, resp.err = status, code, line, err
	}
	defer func() {
		kind := LogTypeResponse
		if httpStatus >= 400 {
			kind = LogTypeError
		}
		s.recordAutomationLogContext(ctx, AutomationLog{Type: kind, Endpoint: r.URL.Path, Method: r.Method,
			Serial: resp.device, Status: httpStatus, Code: resp.code, ElapsedMS: time.Since(started).Milliseconds(),
			Message: "统一自动化批次完成", Fields: AutomationLogFields("completed", resp.completed, "total", resp.total, "failed_line", resp.failedLine)})
		writeAutomationText(w, httpStatus, formatActBatch(resp))
	}()
	title, titleCode, titleErr := parseActTitle(r.URL.RawQuery)
	if titleErr != nil {
		fail(http.StatusBadRequest, titleCode, 0, titleErr)
		return
	}
	resp.title = title
	ctx = context.WithValue(ctx, automationTitleContextKey{}, title)
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fail(400, "invalid_request", 0, err)
		return
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) != "" {
			resp.total++
		}
	}
	steps, invalidLine, err := parseActBatch(string(body), resp.device)
	if err != nil {
		fail(400, "invalid_params", invalidLine, err)
		return
	}
	resp.total = len(steps)
	if _, err := s.resolveActDevice(resp.device); err != nil {
		fail(404, "device_unavailable", 0, err)
		return
	}
	if s.actSlots != nil {
		select {
		case s.actSlots <- struct{}{}:
			defer func() { <-s.actSlots }()
		default:
			fail(429, "automation_busy", 0, fmt.Errorf("automation capacity is full; retry later"))
			return
		}
	}
	// Keep tap/sleep/check sequences atomic relative to other act batches for
	// the same device. There is no waiting queue and active state is bounded.
	s.actMu.Lock()
	_, busy := s.actDevices[resp.device]
	full := len(s.actDevices) >= 16
	if !busy && !full {
		if s.actDevices == nil {
			s.actDevices = make(map[string]struct{})
		}
		s.actDevices[resp.device] = struct{}{}
	}
	s.actMu.Unlock()
	if busy {
		fail(409, "device_busy", 0, fmt.Errorf("device already has an active automation batch"))
		return
	}
	if full {
		fail(429, "automation_busy", 0, fmt.Errorf("automation capacity is full; retry later"))
		return
	}
	defer func() {
		s.actMu.Lock()
		delete(s.actDevices, resp.device)
		s.actMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, actTimeout)
	defer cancel()
	taskIDs, present := r.URL.Query()["task_id"]
	taskID := ""
	if present {
		if len(taskIDs) != 1 || strings.TrimSpace(taskIDs[0]) == "" || len(taskIDs[0]) > 128 {
			fail(400, "invalid_task_id", 0, fmt.Errorf("task_id 必须是单个有效任务标识"))
			return
		}
		taskID = taskIDs[0]
	}
	runtime := s.taskRuntime()
	taskID, token, startIndex, taskErr := runtime.beginBatch(taskID, resp.device, title, steps, cancel)
	if taskErr != nil {
		status, code := taskStatusError(taskErr)
		fail(status, code, 0, taskErr)
		return
	}
	resp.taskID = taskID
	ctx = context.WithValue(ctx, automationTaskContextKey{}, automationTaskExecution{taskID: taskID, token: token})
	defer func() { runtime.endBatch(taskID, token, resp.err) }()
	s.recordAutomationLogContext(ctx, AutomationLog{Type: LogTypeRequest, Endpoint: r.URL.Path, Method: r.Method, Serial: resp.device,
		Message: "统一自动化批次开始", Fields: AutomationLogFields("total", resp.total)})
	for stepOffset, step := range steps {
		stepIndex := startIndex + stepOffset
		stepCtx := context.WithValue(ctx, automationTaskContextKey{}, automationTaskExecution{taskID: taskID, token: token, index: stepIndex})
		runtime.startStep(taskID, token, stepIndex)
		step.response = ActResponse{Action: step.req.Action, Device: resp.device, Status: "failed", Total: 1}
		if len(step.plan.keys) > 0 {
			step.response.Total = len(step.plan.keys)
		}
		if step.req.Action == "clear" {
			step.response.Total = 2
		}
		if err := ctx.Err(); err != nil {
			fail(504, "automation_cancelled", step.line, err)
			return
		}
		if _, err := s.resolveActDevice(resp.device); err != nil {
			fail(404, "device_unavailable", step.line, err)
			return
		}
		err := s.runAct(stepCtx, step.req, step.plan, &step.response)
		if err == nil && step.plan.directory != "" {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			link := url.URL{Scheme: scheme, Host: r.Host, Path: "/api/automation/artifacts"}
			browseDirectory := step.plan.directory
			if step.req.Action == "rm" {
				browseDirectory = path.Dir(browseDirectory)
			}
			q := url.Values{"serial": []string{resp.device}, "directory": []string{browseDirectory}}
			link.RawQuery = q.Encode()
			step.response.Result = fmt.Sprintf("%s\n目录 %s", step.response.Result, link.String())
		}
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			runtime.settleStep(taskID, token, stepIndex, "failed", taskResultText(step.response.Result), err)
			resp.steps = append(resp.steps, step)
			status, code := http.StatusBadGateway, "execution_failed"
			if errors.Is(err, errActWaitTimeout) {
				status, code = http.StatusRequestTimeout, "wait_timeout"
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				status, code = http.StatusGatewayTimeout, "automation_cancelled"
			}
			var sourceErr *actSourceError
			if errors.As(err, &sourceErr) && ctx.Err() == nil {
				status, code = http.StatusBadGateway, "observation_failed"
			}
			var paramsErr *actParamsError
			if errors.As(err, &paramsErr) {
				status, code = http.StatusBadRequest, "invalid_params"
			}
			fail(status, code, step.line, err)
			return
		}
		if step.response.Status == "not_matched" {
			runtime.settleStep(taskID, token, stepIndex, "failed", taskResultText(step.response.Result), fmt.Errorf("check target was not found"))
			resp.steps = append(resp.steps, step)
			fail(409, "check_not_matched", step.line, fmt.Errorf("check target was not found; later operations were not executed"))
			return
		}
		if step.response.Status == "failed" {
			step.response.Status = "executed"
		}
		runtime.settleStep(taskID, token, stepIndex, "done", taskResultText(step.response.Result), nil)
		resp.steps = append(resp.steps, step)
		resp.completed++
	}
	resp.status = "executed"
}

var errActWaitTimeout = errors.New("wait target was not found before timeout")

type actParamsError struct{ error }
type actSourceError struct{ error }

func (s *Service) runAct(ctx context.Context, req ActRequest, plan actPlan, resp *ActResponse) error {
	serial := resp.Device
	if req.Action == "adb" {
		result, err := s.runActADB(ctx, serial, plan.args)
		resp.Result = result
		if err == nil {
			resp.Completed = 1
		}
		return err
	}
	if req.Action == "mkdir" || req.Action == "screenshot" || req.Action == "record" || req.Action == "rm" {
		result, err := s.runActArtifact(ctx, serial, req.Action, plan.directory, plan.duration)
		if err == nil {
			resp.Result, resp.Completed = result, 1
		}
		return err
	}
	if req.Action == "sleep" {
		timer := time.NewTimer(time.Duration(plan.duration) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			resp.Completed = 1
			return nil
		}
	}
	if req.Action == "check" || req.Action == "wait" {
		keyword := req.Params
		checkCtx := ctx
		var cancel context.CancelFunc
		if req.Action == "wait" {
			keyword = plan.keyword
			checkCtx, cancel = context.WithTimeout(ctx, time.Duration(plan.duration)*time.Millisecond)
			defer cancel()
		}
		for {
			result, err := s.checkActText(checkCtx, serial, keyword)
			if err != nil {
				return &actSourceError{err}
			}
			resp.Result = result
			if result.Matched {
				resp.Status, resp.Completed = "matched", 1
				return nil
			}
			if req.Action == "check" {
				resp.Status, resp.Completed = "not_matched", 0
				return nil
			}
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-checkCtx.Done():
				timer.Stop()
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return errActWaitTimeout
			case <-timer.C:
			}
		}
	}

	// Use the existing bounded action queue exactly once per operation. A Hub
	// error may follow injection, so never retry it through another transport.
	if s.actTransport == nil {
		return fmt.Errorf("device action transport is unavailable")
	}
	var requests []action.Request
	switch req.Action {
	case "tap", "swipe":
		page, err := s.getUIPageProvider().FetchPageInfo(ctx, serial)
		if err != nil {
			return fmt.Errorf("fetch current display: %w", err)
		}
		if page == nil || page.Display == nil || page.Display.Width <= 0 || page.Display.Height <= 0 {
			return fmt.Errorf("current display dimensions are unavailable")
		}
		width, height := page.Display.Width, page.Display.Height
		// UIPage exposes wm size in the display's natural orientation. Pixels
		// from the visible screen rotate with SurfaceOrientation.
		if page.Display.Rotation == 90 || page.Display.Rotation == 270 {
			width, height = height, width
		}
		for _, point := range plan.points {
			if point[0] >= width || point[1] >= height {
				return &actParamsError{fmt.Errorf("coordinate @%d,%d is outside current %dx%d screen", point[0], point[1], width, height)}
			}
		}
		request := action.Request{Action: req.Action, X: float64(plan.points[0][0]) / float64(width), Y: float64(plan.points[0][1]) / float64(height)}
		if req.Action == "swipe" {
			request.X2, request.Y2 = float64(plan.points[1][0])/float64(width), float64(plan.points[1][1])/float64(height)
			request.DurationMS = plan.duration
		}
		requests = append(requests, request)
	case "keyevent":
		for _, key := range plan.keys {
			requests = append(requests, action.Request{Action: "key", Keycode: key, Mode: action.ModeSDK})
		}
	case "clear":
		diff := false
		_, screen, err := s.observeWithScreen(ctx, ObserveRequest{Serial: serial, Formats: []string{"ref"}, Diff: &diff}, false)
		if err != nil {
			return &actSourceError{err}
		}
		focusedEditable := false
		if screen != nil {
			for _, element := range screen.Elements {
				if element.Focused && element.Editable && !element.Disabled {
					focusedEditable = true
					break
				}
			}
		}
		if !focusedEditable {
			return &actParamsError{fmt.Errorf("clear requires a focused editable input")}
		}
		// Android KeyEvent: KEYCODE_A=29, META_CTRL_ON=0x1000,
		// KEYCODE_DEL=67. Select all, then delete without changing clipboard.
		requests = append(requests,
			action.Request{Action: "key", Keycode: 29, MetaState: 0x1000, Mode: action.ModeSDK},
			action.Request{Action: "key", Keycode: 67, Mode: action.ModeSDK})
	case "copy", "paste":
		// Android KEYCODE_COPY / KEYCODE_PASTE operate on the current selection
		// and focus. Inject once through the existing SDK action transport.
		keycode := uint32(278)
		if req.Action == "paste" {
			keycode = 279
		}
		requests = append(requests, action.Request{Action: "key", Keycode: keycode, Mode: action.ModeSDK})
	case "sendkey", "set-copy":
		// Android's injected TextEvent cannot represent all Unicode characters.
		// set-copy only changes the clipboard; sendkey additionally pastes once.
		requests = append(requests, action.Request{Action: "raw", Mode: action.ModeSDK,
			Raw: scrcpy.SetClipboardEvent(req.Params, req.Action == "sendkey", actClipboardSequence.Add(1))})
	}
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return err
		}
		request.RequestID, request.DeviceID, request.Source = newActionID(), serial, "automation.act"
		if request.Mode == "" {
			request.Mode = s.resolveMode("")
		}
		request.ExpiresMS = 15000
		actionCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := s.actTransport.SubmitAction(actionCtx, serial, request)
		actionErr := actionCtx.Err()
		cancel()
		if actionErr != nil {
			s.recordAutomationLogContext(ctx, AutomationLog{Type: LogTypeError, Serial: serial,
				Message: "动作传输返回时执行期限已结束；不会重试或继续后续步骤",
				Fields:  AutomationLogFields("action", req.Action, "executed", result.Executed, "request_id", request.RequestID)})
		}
		if err != nil {
			return fmt.Errorf("step %d/%d failed; operation was not retried: %w", resp.Completed+1, resp.Total, err)
		}
		if !result.Executed {
			if actionErr != nil {
				return actionErr
			}
			return fmt.Errorf("step %d/%d was not executed: %s", resp.Completed+1, resp.Total, result.ErrorCode)
		}
		resp.Completed++
	}
	return nil
}

// checkActText matches only the same visible label and OCR fields rendered by
// default Observe. Metadata, coordinate strings and raw XML are never searched.
func (s *Service) checkActText(ctx context.Context, serial, keyword string) (actCheckResult, error) {
	if err := ctx.Err(); err != nil {
		return actCheckResult{}, err
	}
	diff := false
	_, screen, err := s.observeWithScreen(ctx, ObserveRequest{Serial: serial, Diff: &diff}, false)
	if err != nil {
		return actCheckResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return actCheckResult{}, err
	}
	if screen == nil {
		return actCheckResult{}, fmt.Errorf("observation returned no screen")
	}
	treeSource := "tree"
	switch screen.TreeSource {
	case "companion", "uiautomator":
		treeSource += ":" + screen.TreeSource
	}
	for _, element := range screen.Elements {
		if strings.Contains(element.Label, keyword) {
			return actCheckResult{Matched: true, Source: treeSource}, nil
		}
	}
	for _, item := range screen.OCR {
		if strings.Contains(item.Text, keyword) {
			return actCheckResult{Matched: true, Source: "ocr"}, nil
		}
	}
	return actCheckResult{}, nil
}
