package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"farm/internal/dependencies"
	"farm/internal/ocr"
	"farm/internal/uipage"
	"farm/internal/uixml"
)

// UIXMLProvider 抽象 Accessibility XML 快照服务
type UIXMLProvider interface {
	FetchSnapshot(ctx context.Context, serial string) (uixml.Snapshot, error)
}

// UIPageProvider 抽象页面与显示信息服务
type UIPageProvider interface {
	FetchPageInfo(ctx context.Context, serial string) (*uipage.PageInfo, error)
}

// ObserveRequest 统一感知请求参数
type ObserveRequest struct {
	Serial   string   `json:"serial"`
	Formats  []string `json:"formats,omitempty"`   // 可选: "ref", "ocr", "xml"。默认 ["ref", "ocr"]
	Diff     *bool    `json:"diff,omitempty"`      // 是否优先返回增量差异，默认 true
	SettleMS int      `json:"settle_ms,omitempty"` // 等待画面静止延时 (毫秒)
}

var commonAppNames = map[string]string{
	"com.android.settings":                    "Settings",
	"com.android.systemui":                    "SystemUI",
	"com.android.vending":                     "Play Store",
	"com.android.chrome":                      "Chrome",
	"com.google.android.googlequicksearchbox": "Google",
	"com.android.camera":                      "Camera",
	"com.android.gallery3d":                   "Gallery",
	"com.android.dialer":                      "Phone",
	"com.android.mms":                         "Messages",
	"com.tencent.mm":                          "WeChat",
	"com.tencent.mobileqq":                    "QQ",
	"com.ss.android.ugc.aweme":                "TikTok",
	"com.alibaba.android.rimet":               "DingTalk",
	"com.xunmeng.pinduoduo":                   "Pinduoduo",
	"com.jingdong.app.mall":                   "JD",
	"com.taobao.taobao":                       "Taobao",
}

func resolveAppName(pkg string) string {
	if name, ok := commonAppNames[pkg]; ok {
		return name
	}
	parts := strings.Split(pkg, ".")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if len(last) > 0 {
			return strings.ToUpper(last[:1]) + last[1:]
		}
	}
	return pkg
}

// snapshotDisplayWidth 安全读取快照显示宽度
func snapshotDisplayWidth(snapshot uixml.Snapshot) int {
	if snapshot.DisplaySize == nil {
		return 0
	}
	return snapshot.DisplaySize.Width
}

// snapshotDisplayHeight 安全读取快照显示高度
func snapshotDisplayHeight(snapshot uixml.Snapshot) int {
	if snapshot.DisplaySize == nil {
		return 0
	}
	return snapshot.DisplaySize.Height
}

// SetUIXMLProvider 注入自定义 UIXML 提供者
func (s *Service) SetUIXMLProvider(p UIXMLProvider) {
	s.uiXML = p
}

// SetUIPageProvider 注入自定义 UIPage 提供者
func (s *Service) SetUIPageProvider(p UIPageProvider) {
	s.uiPage = p
}

// ResetPreviousScreen 清理指定设备的感知前序缓存（用于测试或强制重置）
func (s *Service) ResetPreviousScreen(serial string) {
	s.screenCache.Delete(serial)
}

// getUIXMLProvider 获取有效 UIXML 提供者，缺失时使用默认实现
func (s *Service) getUIXMLProvider() UIXMLProvider {
	if s.uiXML != nil {
		return s.uiXML
	}
	return uixml.New(s.adbPath)
}

// getUIPageProvider 获取有效 UIPage 提供者，缺失时使用默认实现
func (s *Service) getUIPageProvider() UIPageProvider {
	if s.uiPage != nil {
		return s.uiPage
	}
	return uipage.New(s.adbPath)
}

// checkKeyboardVisible 检查软键盘是否可见
func (s *Service) checkKeyboardVisible(ctx context.Context, serial string) bool {
	if s.adbPath == "" || serial == "" {
		return false
	}
	ctxTimeout, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	out, err := dependencies.CommandContext(ctxTimeout, s.adbPath, "-s", serial, "shell", "dumpsys", "input_method").Output()
	if err == nil && strings.Contains(string(out), "mInputShown=true") {
		return true
	}
	return false
}

// isOCRRedundant 检查 OCR 文字是否已由 Accessibility 元素所涵盖
func isOCRRedundant(box ocr.Box, cx, cy int, elements []Element) bool {
	boxText := strings.TrimSpace(box.Text)
	if boxText == "" {
		return true
	}

	for _, elem := range elements {
		label := strings.TrimSpace(elem.Label)
		if label == "" {
			continue
		}

		textMatches := label == boxText ||
			strings.Contains(label, boxText) ||
			strings.Contains(boxText, label)

		if textMatches {
			// 坐标在元素边界内或中心临近
			inBounds := cx >= elem.Bounds[0] && cx <= elem.Bounds[2] &&
				cy >= elem.Bounds[1] && cy <= elem.Bounds[3]
			nearCenter := centerDistance(cx, cy, elem.CenterX, elem.CenterY) <= 40.0

			if inBounds || nearCenter {
				return true
			}
		}
	}
	return false
}

// Observe 统一感知入口：聚合 Accessibility 树与 OCR，执行空间中心坐标计算与增量 Diff
func (s *Service) Observe(ctx context.Context, req ObserveRequest) (string, error) {
	text, _, err := s.observeWithScreen(ctx, req, true)
	return text, err
}

// observeWithScreen returns this invocation's structured snapshot directly.
// Checks use the same collection, filtering and fallback behavior without
// consuming the caller's previous-observation diff cache.
func (s *Service) observeWithScreen(ctx context.Context, req ObserveRequest, commitCache bool) (string, *Screen, error) {
	if strings.TrimSpace(req.Serial) == "" {
		return "", nil, fmt.Errorf("missing serial")
	}

	started := time.Now()
	s.recordAutomationLogContext(ctx, AutomationLog{
		Type:    LogTypeObserve,
		Serial:  req.Serial,
		Message: "感知请求开始",
		Fields: AutomationLogFields(
			"formats", strings.Join(req.Formats, "|"),
			"diff", req.Diff == nil || *req.Diff,
			"settle_ms", req.SettleMS,
		),
	})

	// 1. 静止延时等待
	if req.SettleMS > 0 {
		settle := time.Duration(req.SettleMS) * time.Millisecond
		if settle > 5*time.Second {
			settle = 5 * time.Second
		}
		select {
		case <-time.After(settle):
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
	}

	formats := req.Formats
	if len(formats) == 0 {
		formats = []string{"ref", "ocr"}
	}

	// 若开发者仅请求调试 xml 格式，直接返回 XML Dump
	isOnlyXML := len(formats) == 1 && strings.EqualFold(formats[0], "xml")
	includeRef, includeOCR := parseFormats(formats)
	var snapshot uixml.Snapshot
	if includeRef || isOnlyXML {
		xmlStarted := time.Now()
		var err error
		snapshot, err = s.getUIXMLProvider().FetchSnapshot(ctx, req.Serial)
		if err != nil {
			s.recordAutomationLogContext(ctx, AutomationLog{
				Type:    LogTypeError,
				Serial:  req.Serial,
				Message: "获取 Accessibility 快照失败",
				Fields:  AutomationLogFields("stage", "fetch_ui_xml", "error", err.Error()),
			})
			return "", nil, fmt.Errorf("fetch ui xml: %w", err)
		}
		s.recordAutomationLogContext(ctx, AutomationLog{
			Type:    LogTypeObserveStage,
			Serial:  req.Serial,
			Message: "Accessibility 快照获取完成",
			Fields: AutomationLogFields(
				"stage", "fetch_ui_xml",
				"elapsed_ms", time.Since(xmlStarted).Milliseconds(),
				"source", snapshot.Source,
				"xml_bytes", len(snapshot.XML),
				"display_width", snapshotDisplayWidth(snapshot),
				"display_height", snapshotDisplayHeight(snapshot),
			),
		})

	}

	if isOnlyXML {
		s.recordAutomationLogContext(ctx, AutomationLog{
			Type:    LogTypeResponse,
			Serial:  req.Serial,
			Message: "返回 XML 调试快照",
			Fields: AutomationLogFields(
				"mode", "xml",
				"bytes", len(snapshot.XML),
				"elapsed_ms", time.Since(started).Milliseconds(),
			),
		})
		return snapshot.XML, nil, nil
	}

	// 2. 收集屏幕与页面元数据
	pageProvider := s.getUIPageProvider()
	pageInfo, pageErr := pageProvider.FetchPageInfo(ctx, req.Serial)
	if includeOCR && !includeRef && (pageErr != nil || pageInfo == nil || pageInfo.Display == nil || pageInfo.Display.Width <= 0 || pageInfo.Display.Height <= 0) {
		return "", nil, fmt.Errorf("OCR-only observation requires current page and display information: %v", pageErr)
	}
	if pageErr != nil {
		s.recordAutomationLogContext(ctx, AutomationLog{
			Type:    LogTypeObserveStage,
			Serial:  req.Serial,
			Message: "页面信息获取失败（降级使用默认值）",
			Fields:  AutomationLogFields("stage", "fetch_page_info", "error", pageErr.Error()),
		})
	}

	width := 1080
	height := 2400
	packageName := ""
	activity := ""
	appName := ""
	orientation := "portrait"

	if pageInfo != nil {
		packageName = pageInfo.PackageName
		activity = pageInfo.Activity
		appName = resolveAppName(packageName)
		if pageInfo.Display != nil {
			if pageInfo.Display.Width > 0 && pageInfo.Display.Height > 0 {
				width = pageInfo.Display.Width
				height = pageInfo.Display.Height
			}
			if pageInfo.Display.Rotation == 90 || pageInfo.Display.Rotation == 270 || width > height {
				orientation = "landscape"
			}
		}
	}

	if snapshot.DisplaySize != nil && snapshot.DisplaySize.Width > 0 && snapshot.DisplaySize.Height > 0 {
		width = snapshot.DisplaySize.Width
		height = snapshot.DisplaySize.Height
		if width > height {
			orientation = "landscape"
		}
	}

	// 3. 解析 Accessibility 树
	var elements []Element
	var focusRef int
	if includeRef {
		var err error
		elements, focusRef, err = ParseHierarchyXML([]byte(snapshot.XML), width, height)
		if err != nil {
			s.recordAutomationLogContext(ctx, AutomationLog{
				Type:    LogTypeError,
				Serial:  req.Serial,
				Message: "解析 Accessibility 树失败",
				Fields:  AutomationLogFields("stage", "parse_hierarchy", "error", err.Error()),
			})
			return "", nil, fmt.Errorf("parse hierarchy: %w", err)
		}
		s.recordAutomationLogContext(ctx, AutomationLog{
			Type:    LogTypeObserveStage,
			Serial:  req.Serial,
			Message: "Accessibility 树解析完成",
			Fields: AutomationLogFields(
				"stage", "parse_hierarchy",
				"elements", len(elements),
				"focus_ref", focusRef,
			),
		})

	}

	// 4. OCR 处理 (若 requested)
	var ocrItems []OCRItem

	if includeOCR && s.ocrService != nil {
		ocrStarted := time.Now()
		var rawImg []byte
		var frameErr error
		if s.frameFetcher != nil {
			_, rawImg, frameErr = s.frameFetcher(ctx, req.Serial)
		}
		if !includeRef && frameErr != nil {
			return "", nil, fmt.Errorf("fetch OCR frame: %w", frameErr)
		}
		if len(rawImg) == 0 {
			if !includeRef {
				return "", nil, fmt.Errorf("fetch OCR frame: no screen frame available")
			}
			s.recordAutomationLogContext(ctx, AutomationLog{
				Type:    LogTypeObserveStage,
				Serial:  req.Serial,
				Message: "屏幕帧获取为空，OCR 将被跳过",
				Fields:  AutomationLogFields("stage", "ocr_frame", "frame_bytes", 0),
			})
		}
		if len(rawImg) > 0 {
			detRes, err := s.ocrService.Detect(ctx, rawImg, ocr.DetectRequest{
				Serial: req.Serial,
			})
			if err != nil {
				if !includeRef {
					return "", nil, fmt.Errorf("OCR detection: %w", err)
				}
				s.recordAutomationLogContext(ctx, AutomationLog{
					Type:    LogTypeObserveStage,
					Serial:  req.Serial,
					Message: "OCR 识别失败（降级为仅 Accessibility 感知）",
					Fields:  AutomationLogFields("stage", "ocr_detect", "error", err.Error()),
				})
			}
			if detRes == nil && err == nil && !includeRef {
				return "", nil, fmt.Errorf("OCR detection: no result returned")
			}
			if err == nil && detRes != nil {
				rawOCRCount := len(detRes.Boxes)
				for _, box := range detRes.Boxes {
					// OCR's confirmed X/Y/W/H contract is normalized top-left
					// bounds. Map it to the physical display used by the tree;
					// Pixel* belongs to the possibly downscaled video frame.
					left := int(math.Round(box.X * float64(width)))
					top := int(math.Round(box.Y * float64(height)))
					right := int(math.Round((box.X + box.W) * float64(width)))
					bottom := int(math.Round((box.Y + box.H) * float64(height)))
					cx, cy := (left+right)/2, (top+bottom)/2

					// 空间去重：过滤已存在独立 Accessibility 节点的文字
					if !isOCRRedundant(box, cx, cy, elements) {
						ocrItems = append(ocrItems, OCRItem{
							Text:       box.Text,
							CenterX:    cx,
							CenterY:    cy,
							Bounds:     [4]int{left, top, right, bottom},
							Confidence: box.Confidence,
						})
					}
				}
				s.recordAutomationLogContext(ctx, AutomationLog{
					Type:    LogTypeObserveStage,
					Serial:  req.Serial,
					Message: "OCR 识别完成",
					Fields: AutomationLogFields(
						"stage", "ocr_detect",
						"elapsed_ms", time.Since(ocrStarted).Milliseconds(),
						"engine", detRes.Engine,
						"raw_boxes", rawOCRCount,
						"kept_boxes", len(ocrItems),
						"deduped_boxes", rawOCRCount-len(ocrItems),
					),
				})
			}
		}
	} else if includeOCR {
		if !includeRef {
			return "", nil, fmt.Errorf("OCR service is unavailable")
		}
		s.recordAutomationLogContext(ctx, AutomationLog{
			Type:    LogTypeObserveStage,
			Serial:  req.Serial,
			Message: "OCR 服务未初始化，OCR 通道跳过",
			Fields:  AutomationLogFields("stage", "ocr_detect", "ocr_service", "unavailable"),
		})
	}

	// 5. 键盘状态
	keyboardVisible := s.checkKeyboardVisible(ctx, req.Serial)
	s.recordAutomationLogContext(ctx, AutomationLog{
		Type:    LogTypeObserveStage,
		Serial:  req.Serial,
		Message: "键盘状态检测完成",
		Fields:  AutomationLogFields("stage", "keyboard_state", "keyboard_visible", keyboardVisible),
	})

	// 6. 构造最新 Screen
	nextScreen := &Screen{
		Width:           width,
		Height:          height,
		AppName:         appName,
		PackageName:     packageName,
		Activity:        activity,
		Orientation:     orientation,
		KeyboardVisible: keyboardVisible,
		FocusRef:        focusRef,
		Elements:        elements,
		OCR:             ocrItems,
		RawXML:          snapshot.XML,
		TreeSource:      snapshot.Source,
		CapturedAt:      time.Now().UTC(),
	}

	// 7. Diff 对比与缓存更新
	preferDiff := true
	if req.Diff != nil {
		preferDiff = *req.Diff
	}

	var output string
	if preferDiff {
		var prevScreen *Screen
		if val, ok := s.screenCache.Load(req.Serial); ok {
			prevScreen = val.(*Screen)
		}
		diffText, isDiff := Diff(prevScreen, nextScreen, formats)
		output = diffText
		s.recordAutomationLogContext(ctx, AutomationLog{
			Type:    LogTypeDiff,
			Serial:  req.Serial,
			Message: "差异对比完成",
			Fields: AutomationLogFields(
				"mode", "diff",
				"is_diff", isDiff,
				"has_prev", prevScreen != nil,
				"prev_elements", elementCount(prevScreen),
				"next_elements", len(nextScreen.Elements),
				"next_ocr", len(nextScreen.OCR),
				"output_lines", countLines(output),
				"output_bytes", len(output),
			),
		})
	} else {
		output = nextScreen.Render(formats)
		s.recordAutomationLogContext(ctx, AutomationLog{
			Type:    LogTypeDiff,
			Serial:  req.Serial,
			Message: "全量渲染完成",
			Fields: AutomationLogFields(
				"mode", "full",
				"is_diff", false,
				"next_elements", len(nextScreen.Elements),
				"next_ocr", len(nextScreen.OCR),
				"output_lines", countLines(output),
				"output_bytes", len(output),
			),
		})
	}

	// 写入设备前序快照缓存
	if commitCache {
		s.screenCache.Store(req.Serial, nextScreen)
	}

	s.recordAutomationLogContext(ctx, AutomationLog{
		Type:    LogTypeResponse,
		Serial:  req.Serial,
		Message: "感知响应完成",
		Fields: AutomationLogFields(
			"app", nextScreen.AppName,
			"package", nextScreen.PackageName,
			"orientation", nextScreen.Orientation,
			"screen", fmt.Sprintf("%dx%d", nextScreen.Width, nextScreen.Height),
			"elapsed_ms", time.Since(started).Milliseconds(),
		),
	})

	return output, nextScreen, nil
}

// elementCount 安全统计前序快照的元素数量
func elementCount(screen *Screen) int {
	if screen == nil {
		return 0
	}
	return len(screen.Elements)
}

// countLines 统计文本行数
func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// handleObserve 处理 POST/GET /api/automation/observe 统一感知请求
func (s *Service) handleObserve(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req ObserveRequest

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.Serial = q.Get("serial")
		if fmts := q.Get("formats"); fmts != "" {
			req.Formats = strings.Split(fmts, ",")
		}
		if diffStr := q.Get("diff"); diffStr != "" {
			d := strings.EqualFold(diffStr, "true") || diffStr == "1"
			req.Diff = &d
		}
		if settleStr := q.Get("settle_ms"); settleStr != "" {
			req.SettleMS, _ = strconv.Atoi(settleStr)
		}
	} else if r.Method == http.MethodPost {
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeAutomationProtocolError(w, r, http.StatusBadRequest, "invalid_json", "请求体 JSON 格式错误: "+err.Error())
				s.recordAutomationLog(AutomationLog{
					Type:     LogTypeError,
					Endpoint: "/api/automation/observe",
					Method:   r.Method,
					Status:   http.StatusBadRequest,
					Code:     "invalid_json",
					Message:  "请求体 JSON 格式错误: " + err.Error(),
					Fields:   AutomationLogFields("elapsed_ms", time.Since(started).Milliseconds()),
				})
				return
			}
		}
		// 允许 query 参数补全缺失项
		if req.Serial == "" {
			req.Serial = r.URL.Query().Get("serial")
		}
	} else {
		writeAutomationProtocolError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 POST 或 GET 请求")
		s.recordAutomationLog(AutomationLog{
			Type:     LogTypeError,
			Endpoint: "/api/automation/observe",
			Method:   r.Method,
			Status:   http.StatusMethodNotAllowed,
			Code:     "method_not_allowed",
			Message:  "仅支持 POST 或 GET 请求",
			Fields:   AutomationLogFields("elapsed_ms", time.Since(started).Milliseconds()),
		})
		return
	}

	if req.Serial == "" {
		writeAutomationProtocolError(w, r, http.StatusBadRequest, "missing_serial", "缺少 serial 参数")
		s.recordAutomationLog(AutomationLog{
			Type:     LogTypeError,
			Endpoint: "/api/automation/observe",
			Method:   r.Method,
			Status:   http.StatusBadRequest,
			Code:     "missing_serial",
			Message:  "缺少 serial 参数",
			Fields:   AutomationLogFields("elapsed_ms", time.Since(started).Milliseconds()),
		})
		return
	}

	s.recordAutomationLog(AutomationLog{
		Type:     LogTypeRequest,
		Endpoint: "/api/automation/observe",
		Method:   r.Method,
		Serial:   req.Serial,
		Message:  "收到统一感知请求",
		Fields: AutomationLogFields(
			"formats", strings.Join(req.Formats, "|"),
			"diff", req.Diff == nil || *req.Diff,
			"settle_ms", req.SettleMS,
			"remote_addr", r.RemoteAddr,
		),
	})

	output, err := s.Observe(r.Context(), req)
	if err != nil {
		writeAutomationProtocolError(w, r, http.StatusBadGateway, "observe_failed", err.Error())
		s.recordAutomationLog(AutomationLog{
			Type:     LogTypeError,
			Endpoint: "/api/automation/observe",
			Method:   r.Method,
			Serial:   req.Serial,
			Status:   http.StatusBadGateway,
			Code:     "observe_failed",
			Message:  err.Error(),
			Fields:   AutomationLogFields("elapsed_ms", time.Since(started).Milliseconds()),
		})
		return
	}

	isOnlyXML := len(req.Formats) == 1 && strings.EqualFold(req.Formats[0], "xml")
	if isOnlyXML {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(output))

	s.recordAutomationLog(AutomationLog{
		Type:     LogTypeResponse,
		Endpoint: "/api/automation/observe",
		Method:   r.Method,
		Serial:   req.Serial,
		Status:   http.StatusOK,
		Message:  "统一感知响应已返回",
		Fields: AutomationLogFields(
			"content_type", w.Header().Get("Content-Type"),
			"output_lines", countLines(output),
			"output_bytes", len(output),
			"elapsed_ms", time.Since(started).Milliseconds(),
		),
	})
}

// handleAutomationLogs 处理 GET /api/automation/logs 查询自动化接口调用日志
func (s *Service) handleAutomationLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := AutomationLogQuery{
		Serial: q.Get("serial"),
		Type:   q.Get("type"),
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			query.Limit = v
		}
	}

	resp := s.ListAutomationLogs(query)
	if q.Get("format") == "text" {
		s.serveAutomationLogsText(w, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
