package automation

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"farm/internal/action"
	"farm/internal/device"
	"farm/internal/inputconfig"
	"farm/internal/ocr"
	"farm/internal/scrcpy"
	"farm/internal/uipage"
	"farm/internal/uixml"
)

type actTestTransport struct {
	requests []action.Request
	failAt   int
	failErr  error
	onSubmit func(context.Context) error
}

func (f *actTestTransport) SubmitAction(ctx context.Context, serial string, req action.Request) (action.Result, error) {
	f.requests = append(f.requests, req)
	if f.onSubmit != nil {
		if err := f.onSubmit(ctx); err != nil {
			return action.Result{}, err
		}
	}
	if f.failAt > 0 && len(f.requests) == f.failAt {
		return action.Result{ErrorCode: "test_injection_failed"}, f.failErr
	}
	return action.Result{Accepted: true, Executed: true, RequestID: req.RequestID}, nil
}

func newActTestService() (*Service, *actTestTransport) {
	svc := NewService(nil, nil, nil, nil, "")
	svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: `<hierarchy/>`, Source: "uiautomator"}})
	svc.SetDeviceProvider(func() []device.Device {
		return []device.Device{{Serial: "phone", Alias: "红米 手机", State: "device"}}
	})
	svc.SetUIPageProvider(&mockUIPageProvider{pageInfo: &uipage.PageInfo{
		PackageName: "com.android.settings", Display: &uipage.DisplayInfo{Width: 100, Height: 200},
	}})
	transport := &actTestTransport{}
	svc.actTransport = transport
	return svc, transport
}

func callAct(svc *Service, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/automation/act?title=test-task&serial=phone", strings.NewReader(body)))
	return w
}

func TestActTextParsesOperationAndPreservesText(t *testing.T) {
	for _, tc := range []struct{ body, action, params string }{
		{"tap @20,30", "tap", "@20,30"},
		{"sendkey  开始 ", "sendkey", " 开始 "},
	} {
		req, err := parseActText(tc.body)
		if err != nil || req.Action != tc.action || req.Params != tc.params {
			t.Fatalf("parse(%q)=%+v err=%v", tc.body, req, err)
		}
	}
	if _, err := parseActText("   "); err == nil {
		t.Fatal("accepted empty operation")
	}
}

func TestActTapAndSwipeUseCurrentRotatedPixelDimensions(t *testing.T) {
	svc, transport := newActTestService()
	// An old observation cannot supply dimensions for a new action.
	svc.screenCache.Store("phone", &Screen{Width: 1000, Height: 2000})
	w := callAct(svc, `tap @25,100`)
	if w.Code != 200 || transport.requests[0].X != .25 || transport.requests[0].Y != .5 {
		t.Fatalf("tap status=%d body=%s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
	svc.SetUIPageProvider(&mockUIPageProvider{pageInfo: &uipage.PageInfo{
		Display: &uipage.DisplayInfo{Width: 100, Height: 200, Rotation: 90},
	}})
	w = callAct(svc, "swipe @150,25 @50,75 500")
	request := transport.requests[1]
	if w.Code != 200 || request.X != .75 || request.Y != .25 || request.X2 != .25 || request.Y2 != .75 || request.DurationMS != 500 {
		t.Fatalf("rotated swipe status=%d body=%s request=%+v", w.Code, w.Body.String(), request)
	}
	w = callAct(svc, "tap @200,0")
	if w.Code != 400 || len(transport.requests) != 2 {
		t.Fatalf("out-of-bounds injection status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("unexpected response content type: %s", w.Header().Get("Content-Type"))
	}
}

func TestActKeyeventValidatesWholeSequenceAndStopsAtFailure(t *testing.T) {
	svc, transport := newActTestService()
	w := callAct(svc, "keyevent home｜back，app_switch,enter|delete")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "（5/5）") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	for i, code := range []uint32{3, 4, 187, 66, 67} {
		if transport.requests[i].Keycode != code {
			t.Fatalf("step %d code=%d", i, transport.requests[i].Keycode)
		}
	}
	for _, params := range []string{"home|invalid", "home||back", "home,", strings.Repeat("home|", 32) + "home"} {
		w = callAct(svc, "keyevent "+params)
		if w.Code != 400 || len(transport.requests) != 5 {
			t.Fatalf("partially executed invalid sequence %q: %d %s", params, w.Code, w.Body.String())
		}
	}
	for _, failure := range []error{nil, errors.New("connection lost after injection")} {
		svc, transport = newActTestService()
		transport.failAt, transport.failErr = 2, failure
		w = callAct(svc, "keyevent home|back|app_switch")
		if w.Code != 502 || len(transport.requests) != 2 || !strings.Contains(w.Body.String(), "（1/3）") || !strings.Contains(w.Body.String(), "状态 失败") {
			t.Fatalf("failure incorrectly handled: %d %s requests=%+v", w.Code, w.Body.String(), transport.requests)
		}
	}
}

func TestActSendkeyPreservesRawTextAndDoesNotLogIt(t *testing.T) {
	svc, transport := newActTestService()
	text := " 你好｜这,是文本; $(do-not-run) "
	w := callAct(svc, "sendkey "+text)
	if w.Code != 200 || len(transport.requests) != 1 || transport.requests[0].Action != "raw" || transport.requests[0].Mode != action.ModeSDK {
		t.Fatalf("sendkey status=%d body=%s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
	assertActClipboardPayload(t, transport.requests[0].Raw, text)
	for _, entry := range svc.ListAutomationLogs(AutomationLogQuery{}).Logs {
		if strings.Contains(entry.Message, "do-not-run") || strings.Contains(formatAutomationFields(entry.Fields), "do-not-run") {
			t.Fatalf("input text leaked to log: %+v", entry)
		}
	}
}

func assertActClipboardPayload(t *testing.T, raw []byte, text string) uint64 {
	t.Helper()
	if len(raw) < 14 {
		t.Fatalf("clipboard packet is truncated: %d bytes", len(raw))
	}
	sequence := binary.BigEndian.Uint64(raw[1:9])
	if raw[0] != scrcpy.TypeSetClipboard || raw[9] != 1 || sequence == 0 || binary.BigEndian.Uint32(raw[10:14]) != uint32(len([]byte(text))) || string(raw[14:]) != text || !utf8.Valid(raw[14:]) {
		t.Fatalf("invalid clipboard packet type=%d paste=%d sequence=%d length=%d text=%q", raw[0], raw[9], sequence, binary.BigEndian.Uint32(raw[10:14]), string(raw[14:]))
	}
	return sequence
}

func TestActSendkeyClipboardPreservesUnicodeWhitespaceAndCase(t *testing.T) {
	svc, transport := newActTestService()
	var previousSequence uint64
	for _, text := range []string{"验证", "  farmTest 中文 A大小写Z  ", "é🙂Aa 空格\t保留"} {
		w := callAct(svc, "sendkey "+text)
		if w.Code != 200 {
			t.Fatalf("sendkey rejected: %d %s", w.Code, w.Body.String())
		}
		request := transport.requests[len(transport.requests)-1]
		sequence := assertActClipboardPayload(t, request.Raw, text)
		if sequence <= previousSequence || request.Text != "" {
			t.Fatalf("clipboard sequence did not advance or text used old channel: %+v", request)
		}
		previousSequence = sequence
	}
	if len(transport.requests) != 3 {
		t.Fatalf("sendkey replayed clipboard injection: %d requests", len(transport.requests))
	}
}

func TestActKeyAndClipboardUseSDKWhileTouchInheritsMode(t *testing.T) {
	for _, mode := range []string{action.ModeADB, action.ModeHID, action.ModeSDK} {
		t.Run(mode, func(t *testing.T) {
			svc, transport := newActTestService()
			svc.inputConfig = inputconfig.New(nil)
			if err := svc.inputConfig.Set(mode); err != nil {
				t.Fatal(err)
			}
			w := callAct(svc, "keyevent home|back\nsendkey 验证 Ab\ntap @10,20\nswipe @10,20 @20,30")
			if w.Code != 200 || len(transport.requests) != 5 {
				t.Fatalf("batch failed: %d %s requests=%+v", w.Code, w.Body.String(), transport.requests)
			}
			for i, request := range transport.requests {
				expected := action.ModeSDK
				if i >= 3 {
					expected = mode
				}
				if request.Mode != expected {
					t.Fatalf("operation %d mode=%q expected=%q", i, request.Mode, expected)
				}
			}
			assertActClipboardPayload(t, transport.requests[2].Raw, "验证 Ab")
		})
	}
}

func TestActSendkeyClipboardFailureIsNeverRetried(t *testing.T) {
	svc, transport := newActTestService()
	transport.failAt, transport.failErr = 1, errors.New("control socket failed after write")
	w := callAct(svc, "sendkey 验证\nkeyevent home")
	if w.Code != 502 || len(transport.requests) != 1 || !strings.Contains(w.Body.String(), "完成 0/2") {
		t.Fatalf("clipboard failure was retried or batch continued: %d %s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
	assertActClipboardPayload(t, transport.requests[0].Raw, "验证")
}

func TestActWaitAndActionRespectCancellation(t *testing.T) {
	svc, transport := newActTestService()
	w := callAct(svc, "sleep 0")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "完成 1/1") || len(transport.requests) != 0 {
		t.Fatalf("wait result: %d %s", w.Code, w.Body.String())
	}
	for _, input := range []string{"sleep 30000", "tap @1,2"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		transport.onSubmit = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
		req := httptest.NewRequest(http.MethodPost, "/api/automation/act?title=test-task&serial=phone", strings.NewReader(input)).WithContext(ctx)
		w = httptest.NewRecorder()
		svc.handleAct(w, req)
		cancel()
		if w.Code != 504 || !strings.Contains(w.Body.String(), "完成 0/1") {
			t.Fatalf("cancelled request result: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestActRejectsMalformedRequestsBeforeDeviceCalls(t *testing.T) {
	for _, body := range []string{
		`{"action":"tap","device":"phone","params":"@1,2"}`,
		"unknown hello", "tap @-1,1", "tap @1.1,2", "tap @1,2 extra",
		"swipe @1,2", "swipe @1,2 @2,3 0", "swipe @1,2 @2,3 10001",
		"sleep -1", "sleep 30001", "sleep 1.2", "wait 500", "wait 0 hello", "wait 30001 hello", "sendkey", "check", "ocr", "ocr engine",
		"sendkey " + strings.Repeat("a", 16385), strings.Repeat("a", 65537),
	} {
		svc, transport := newActTestService()
		w := callAct(svc, body)
		if w.Code != 400 || len(transport.requests) != 0 || !strings.Contains(w.Body.String(), "状态 失败") {
			t.Fatalf("invalid request result: %.80q %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestAutomationDeviceListAndResolution(t *testing.T) {
	svc, _ := newActTestService()
	svc.SetDeviceProvider(func() []device.Device {
		return []device.Device{
			{Serial: "offline", Alias: "Redmi", State: "offline"},
			{Serial: "first", Alias: "Redmi", State: "device"},
			{Serial: "second", Alias: "REDMI", State: "device"},
			{Serial: "third\nserial", Alias: "line\nbreak", State: "device"},
			{Serial: "collision", Alias: "offline", State: "device"},
		}
	})
	if got, err := svc.resolveActDevice("redmi"); err == nil || got != "" {
		t.Fatalf("identifier resolution=%q err=%v", got, err)
	}
	if got, err := svc.resolveActDevice("second"); err != nil || got != "second" {
		t.Fatalf("exact serial resolution=%q err=%v", got, err)
	}
	for _, input := range []string{"offline", "missing"} {
		if _, err := svc.resolveActDevice(input); err == nil {
			t.Fatalf("accepted unknown/offline serial %q", input)
		}
		w := httptest.NewRecorder()
		svc.handleAct(w, httptest.NewRequest(http.MethodPost, "/api/automation/act?title=test-task&serial="+input, strings.NewReader("sleep 0")))
		if w.Code != 404 {
			t.Fatalf("unavailable device result: %d %s", w.Code, w.Body.String())
		}
	}
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/automation/devices", nil))
	if w.Code != 200 || len(strings.Split(strings.TrimSpace(w.Body.String()), "\n")) != 4 || !strings.Contains(w.Body.String(), `设备 "Redmi" · serial "first" · 在线`) {
		t.Fatalf("device lines: %d %s", w.Code, w.Body.String())
	}
	for _, devices := range [][]device.Device{nil, {{Serial: "off", State: "offline"}}} {
		svc.SetDeviceProvider(func() []device.Device { return devices })
		w = httptest.NewRecorder()
		svc.handleAutomationDevices(w, httptest.NewRequest(http.MethodGet, "/api/automation/devices", nil))
		if w.Body.String() != "当前没有在线设备。\n" {
			t.Fatalf("empty device list: %s", w.Body.String())
		}
	}
}

func TestActBusyAndUnavailableProviders(t *testing.T) {
	svc, transport := newActTestService()
	for i := 0; i < cap(svc.actSlots); i++ {
		svc.actSlots <- struct{}{}
	}
	if w := callAct(svc, "tap @1,2"); w.Code != 429 || len(transport.requests) != 0 {
		t.Fatalf("capacity did not reject: %d %s", w.Code, w.Body.String())
	}
	svc, _ = newActTestService()
	svc.actTransport = nil
	if w := callAct(svc, "tap @1,2"); w.Code != 502 {
		t.Fatalf("missing transport accepted: %d %s", w.Code, w.Body.String())
	}
	svc, _ = newActTestService()
	svc.SetUIPageProvider(&mockUIPageProvider{err: errors.New("page unavailable")})
	if w := callAct(svc, "tap @1,2"); w.Code != 502 {
		t.Fatalf("missing page accepted: %d %s", w.Code, w.Body.String())
	}
	svc, _ = newActTestService()
	svc.SetUIXMLProvider(&mockUIXMLProvider{err: errors.New("tree unavailable")})
	if w := callAct(svc, "check hello"); w.Code != 502 {
		t.Fatalf("missing tree accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestActCheckUsesDefaultObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ocr_result":"设置 保存","ocr_boxes":[{"text":"设置","confidence":0.99,"x":10,"y":20,"w":40,"h":20}]}`))
	}))
	defer server.Close()
	t.Setenv("INDRAOCR_URL", server.URL)
	t.Setenv("OCR_ENGINE", "indraocr")
	var frame bytes.Buffer
	_ = png.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 100, 200)))
	svc, _ := newActTestService()
	svc.ocrService = ocr.NewService(nil, nil)
	svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: `<hierarchy/>`, Source: "uiautomator"}})
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) { return nil, frame.Bytes(), nil })
	var w *httptest.ResponseRecorder
	for _, tc := range []struct{ params, status string }{{"设置", "命中"}, {"未出现", "未命中"}} {
		w = callAct(svc, "check "+tc.params)
		wantCode := 200
		if tc.status == "未命中" {
			wantCode = 409
		}
		if w.Code != wantCode || !strings.Contains(w.Body.String(), "check："+tc.status) {
			t.Fatalf("check status=%d body=%s", w.Code, w.Body.String())
		}
	}
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
		return nil, nil, errors.New("current frame unavailable")
	})
	for _, body := range []string{"check 设置"} {
		w = callAct(svc, body)
		wantCode := 502
		if strings.HasPrefix(body, "check ") {
			wantCode = 409
		}
		if w.Code != wantCode || !strings.Contains(w.Body.String(), "完成 0/1") || !strings.Contains(w.Body.String(), "状态 失败") {
			t.Fatalf("failed frame accepted as observation: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestActBatchPrevalidatesAllLinesBeforeDeviceInteraction(t *testing.T) {
	for _, body := range []string{
		"tap @1,2\n\nkeyevent home|invalid\nsendkey must-not-run",
		strings.Repeat("sleep 0\n", 33),
	} {
		svc, transport := newActTestService()
		providerCalls := 0
		svc.SetDeviceProvider(func() []device.Device { providerCalls++; return nil })
		w := callAct(svc, body)
		if w.Code != 400 || providerCalls != 0 || len(transport.requests) != 0 || !strings.Contains(w.Body.String(), "失败行") {
			t.Fatalf("invalid batch touched device: status=%d calls=%d body=%s", w.Code, providerCalls, w.Body.String())
		}
	}
}

func newActOCRTestService(t *testing.T, result func(int64) string) (*Service, *actTestTransport) {
	t.Helper()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(result(calls.Add(1))))
	}))
	t.Cleanup(server.Close)
	t.Setenv("INDRAOCR_URL", server.URL)
	t.Setenv("OCR_ENGINE", "indraocr")
	var frame bytes.Buffer
	_ = png.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 100, 200)))
	svc, transport := newActTestService()
	svc.ocrService = ocr.NewService(nil, nil)
	svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: `<hierarchy/>`, Source: "uiautomator"}})
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) { return nil, frame.Bytes(), nil })
	return svc, transport
}

func TestActBatchRunsInOrderAndStopsOnUnmatchedCheck(t *testing.T) {
	svc, transport := newActOCRTestService(t, func(int64) string {
		return `{"ocr_result":"当前设置","ocr_boxes":[{"text":"当前设置","confidence":0.99,"x":0.1,"y":0.1,"w":0.5,"h":0.1}]}`
	})
	w := callAct(svc, "tap @1,2\n\ncheck 不存在\nsendkey must-not-run")
	if w.Code != 409 || len(transport.requests) != 1 || !strings.Contains(w.Body.String(), "完成 1/3") || !strings.Contains(w.Body.String(), "失败行 3") || !strings.Contains(w.Body.String(), "第 3 行 check：未命中（0/1）") {
		t.Fatalf("unmatched check did not stop batch: %d %s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
	w = callAct(svc, "tap @1,2\nsleep 0\ncheck 设置\nkeyevent home｜back")
	if w.Code != 200 || len(transport.requests) != 4 || !strings.Contains(w.Body.String(), "完成 4/4") || !strings.Contains(w.Body.String(), "第 4 行 keyevent：已完成（2/2）") {
		t.Fatalf("ordered batch failed: %d %s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
}

func TestActBatchReportsPartialKeyExecution(t *testing.T) {
	svc, transport := newActTestService()
	transport.failAt = 3
	w := callAct(svc, "tap @1,2\nkeyevent home|back|app_switch\nsendkey must-not-run")
	if w.Code != 502 || len(transport.requests) != 3 || !strings.Contains(w.Body.String(), "完成 1/3") || !strings.Contains(w.Body.String(), "第 2 行 keyevent：失败（1/3）") || !strings.Contains(w.Body.String(), "失败行 2") {
		t.Fatalf("partial key failure: %d %s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
}

func TestActWaitMatchesNewOCRAndThenContinues(t *testing.T) {
	svc, transport := newActOCRTestService(t, func(call int64) string {
		if call == 1 {
			return `{"ocr_result":"加载中","ocr_boxes":[{"text":"加载中","confidence":0.99,"x":0.1,"y":0.1,"w":0.5,"h":0.1}]}`
		}
		return `{"ocr_result":"WLAN 已连接","ocr_boxes":[{"text":"WLAN 已连接","confidence":0.99,"x":0.1,"y":0.1,"w":0.5,"h":0.1}]}`
	})
	w := callAct(svc, "wait 2000 WLAN\nkeyevent back")
	if w.Code != 200 || len(transport.requests) != 1 || !strings.Contains(w.Body.String(), "第 1 行 wait：命中（1/1）") || !strings.Contains(w.Body.String(), "完成 2/2") {
		t.Fatalf("wait did not continue after a current match: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `目标 "WLAN"`) || strings.Contains(w.Body.String(), "WLAN 已连接") || strings.Contains(w.Body.String(), "引擎") {
		t.Fatalf("wait response included full-screen OCR instead of compact target: %s", w.Body.String())
	}
}

func TestActWaitTimeoutAndSourceErrorsStopLaterOperations(t *testing.T) {
	svc, transport := newActOCRTestService(t, func(int64) string {
		return `{"ocr_result":"加载中","ocr_boxes":[{"text":"加载中","confidence":0.99,"x":0.1,"y":0.1,"w":0.5,"h":0.1}]}`
	})
	w := callAct(svc, "wait 30 WLAN\nkeyevent back")
	if w.Code != 408 || len(transport.requests) != 0 || !strings.Contains(w.Body.String(), "完成 0/2") || !strings.Contains(w.Body.String(), "错误 wait_timeout") {
		t.Fatalf("wait timeout did not stop batch: %d %s", w.Code, w.Body.String())
	}
	svc.SetUIXMLProvider(&mockUIXMLProvider{err: errors.New("tree service disconnected")})
	started := time.Now()
	w = callAct(svc, "wait 30000 WLAN\nkeyevent back")
	if w.Code != 502 || time.Since(started) > time.Second || len(transport.requests) != 0 || strings.Contains(w.Body.String(), "错误 wait_timeout") || !strings.Contains(w.Body.String(), "tree service disconnected") {
		t.Fatalf("source failure was swallowed: %d %s", w.Code, w.Body.String())
	}
}

func TestActWaitCancellationAndMissingSerial(t *testing.T) {
	svc, transport := newActOCRTestService(t, func(int64) string {
		return `{"ocr_result":"加载中","ocr_boxes":[{"text":"加载中","confidence":0.99,"x":0.1,"y":0.1,"w":0.5,"h":0.1}]}`
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	svc.handleAct(w, httptest.NewRequest(http.MethodPost, "/api/automation/act?title=test-task&serial=phone", strings.NewReader("wait 30000 WLAN\nkeyevent back")).WithContext(ctx))
	if w.Code != 504 || len(transport.requests) != 0 || !strings.Contains(w.Body.String(), "完成 0/2") {
		t.Fatalf("wait cancellation failed: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	svc.handleAct(w, httptest.NewRequest(http.MethodPost, "/api/automation/act?title=test-task", strings.NewReader("sleep 0")))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "serial query parameter is required") {
		t.Fatalf("missing serial accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestActRejectsOverlappingBatchesForDeviceAndReleasesSlot(t *testing.T) {
	svc, transport := newActTestService()
	started, release := make(chan struct{}), make(chan struct{})
	transport.onSubmit = func(ctx context.Context) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- callAct(svc, "keyevent home") }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first batch did not start")
	}
	w := callAct(svc, "sleep 0")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "device_busy") {
		t.Errorf("overlapping batch accepted: %d %s", w.Code, w.Body.String())
	}
	close(release)
	if first := <-finished; first.Code != 200 {
		t.Fatalf("first batch failed: %d %s", first.Code, first.Body.String())
	}
	w = callAct(svc, "sleep 0")
	if w.Code != 200 || len(svc.actSlots) != 0 || len(svc.actDevices) != 0 {
		t.Fatalf("batch slot leaked: %d %s", w.Code, w.Body.String())
	}
}

func TestActWaitSourceDeadlineIsReportedAsObservationFailure(t *testing.T) {
	svc, transport := newActTestService()
	svc.ocrService = ocr.NewService(nil, nil)
	svc.SetFrameFetcher(func(ctx context.Context, _ string) (image.Image, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	w := callAct(svc, "wait 20 WLAN\nkeyevent back")
	if w.Code != 502 || !strings.Contains(w.Body.String(), "observation_failed") || strings.Contains(w.Body.String(), "错误 wait_timeout") || len(transport.requests) != 0 {
		t.Fatalf("observation deadline became a not-found timeout: %d %s", w.Code, w.Body.String())
	}
}
