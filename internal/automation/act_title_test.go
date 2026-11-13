package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"farm/internal/device"
	"farm/internal/uixml"
)

type actTitleBody struct{ reads int }

func (b *actTitleBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("body must not be read")
}
func (b *actTitleBody) Close() error { return nil }

func callActTitle(svc *Service, serial, title, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	query := url.Values{"serial": {serial}, "title": {title}}
	svc.handleAct(w, httptest.NewRequest(http.MethodPost, "/api/automation/act?"+query.Encode(), strings.NewReader(body)))
	return w
}

func TestActTitleRequiredAndValidatedBeforeExecution(t *testing.T) {
	for _, tc := range []struct{ query, code string }{
		{"serial=phone", "missing_title"},
		{"serial=phone&task_title=legacy", "missing_title"},
		{"serial=phone&title=", "invalid_title"},
		{"serial=phone&title=%20%20", "invalid_title"},
		{"serial=phone&title=%0Atitle", "invalid_title"},
		{"serial=phone&title=%09title", "invalid_title"},
		{"serial=phone&title=title%7F", "invalid_title"},
		{"serial=phone&title=%FF", "invalid_title"},
		{"serial=phone&title=%GG", "invalid_title"},
		{"serial=phone&title=one&title=two", "invalid_title"},
		{"serial=phone&title=" + url.QueryEscape(strings.Repeat("中", 121)), "invalid_title"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			svc, transport := newActTestService()
			deviceCalls := 0
			svc.SetDeviceProvider(func() []device.Device { deviceCalls++; return nil })
			body := &actTitleBody{}
			w := httptest.NewRecorder()
			svc.handleAct(w, httptest.NewRequest(http.MethodPost, "/api/automation/act?"+tc.query, body))
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "错误 "+tc.code) || deviceCalls != 0 || body.reads != 0 || len(transport.requests) != 0 {
				t.Fatalf("invalid title reached execution: %d %s reads=%d devices=%d", w.Code, w.Body.String(), body.reads, deviceCalls)
			}
			for _, entry := range svc.ListAutomationLogs(AutomationLogQuery{}).Logs {
				if entry.Title != "" {
					t.Fatalf("invalid request acquired a title: %+v", entry)
				}
			}
		})
	}
}

func TestActTitleUnicodeLengthTrimAndEncoding(t *testing.T) {
	for _, tc := range []struct{ input, title string }{
		{"  核对今天 下班打卡 + 截图  ", "核对今天 下班打卡 + 截图"},
		{"\u00a0设备🙂检查\u3000", "设备🙂检查"},
		{strings.Repeat("中", 120), strings.Repeat("中", 120)},
	} {
		svc, _ := newActTestService()
		w := callActTitle(svc, "phone", tc.input, "sleep 0")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), fmt.Sprintf("任务 %q", tc.title)) {
			t.Fatalf("valid encoded title failed: %d %s", w.Code, w.Body.String())
		}
		for _, entry := range svc.ListAutomationLogs(AutomationLogQuery{}).Logs {
			if entry.Title != tc.title {
				t.Fatalf("title lost after ring roundtrip: %+v", entry)
			}
		}
	}
}

func TestActTitleLogsJSONTextStdoutAndNestedObservation(t *testing.T) {
	var stdout bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&stdout)
	t.Cleanup(func() { log.SetOutput(previous) })
	svc, _ := newActTestService()
	svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: `<hierarchy><node text="已打卡" bounds="[1,1][90,30]"/></hierarchy>`, Source: "companion"}})
	title := "核对下班打卡"
	w := callActTitle(svc, "phone", title, "check 已打卡")
	if w.Code != http.StatusOK {
		t.Fatalf("check failed: %d %s", w.Code, w.Body.String())
	}
	logs := svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone", Limit: 1000})
	seenObserve := false
	for _, entry := range logs.Logs {
		if entry.Title != title {
			t.Fatalf("nested log lost title: %+v", entry)
		}
		if entry.Type == LogTypeObserveStage {
			seenObserve = true
		}
	}
	if !seenObserve {
		t.Fatal("nested observation logs missing")
	}
	if !strings.Contains(stdout.String(), `title="核对下班打卡"`) {
		t.Fatalf("stdout lost title: %s", stdout.String())
	}
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	for _, format := range []string{"", "&format=text"} {
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/automation/logs?serial=phone"+format, nil))
		if w.Code != 200 {
			t.Fatalf("logs endpoint failed: %d %s", w.Code, w.Body.String())
		}
		if format != "" {
			if !strings.Contains(w.Body.String(), `title="核对下班打卡"`) {
				t.Fatalf("text logs lost title: %s", w.Body.String())
			}
		} else {
			var response struct {
				Logs []struct {
					Title string `json:"title"`
				} `json:"logs"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || len(response.Logs) == 0 {
				t.Fatalf("invalid JSON logs: %v %s", err, w.Body.String())
			}
			for _, entry := range response.Logs {
				if entry.Title != title {
					t.Fatalf("JSON log lost title: %+v", entry)
				}
			}
		}
	}
	// Independent Observe calls do not inherit a previous act's title.
	if _, err := svc.Observe(t.Context(), ObserveRequest{Serial: "phone", Formats: []string{"ref"}}); err != nil {
		t.Fatal(err)
	}
	logs = svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone", Type: LogTypeObserve, Limit: 1})
	if logs.Count != 1 || logs.Logs[0].Title != "" {
		t.Fatalf("independent observation inherited title: %+v", logs)
	}
}

func TestActTitleErrorLogs(t *testing.T) {
	svc, _ := newActTestService()
	svc.SetUIXMLProvider(&mockUIXMLProvider{err: errors.New("tree source failed")})
	w := callActTitle(svc, "phone", "检查失败场景", "check 未出现")
	if w.Code != 502 {
		t.Fatalf("expected source failure: %d %s", w.Code, w.Body.String())
	}
	errors := svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone", Type: LogTypeError})
	if errors.Count < 2 {
		t.Fatalf("nested and request failures missing: %+v", errors)
	}
	for _, entry := range errors.Logs {
		if entry.Title != "检查失败场景" {
			t.Fatalf("error lost task title: %+v", entry)
		}
	}
}

func TestActTitleNestedADBAndArtifactLogs(t *testing.T) {
	svc, _ := artifactTestService(t)
	svc.SetDeviceProvider(func() []device.Device { return []device.Device{{Serial: "phone", State: "device"}} })
	title := "导出设备画面"
	w := callActTitle(svc, "phone", title, "adb shell echo fixture\nmkdir title-results\nscreenshot title-results\nrecord 1 title-results")
	if w.Code != 200 {
		t.Fatalf("artifact task failed: %d %s", w.Code, w.Body.String())
	}
	actions := svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone", Type: LogTypeAction})
	if actions.Count != 4 {
		t.Fatalf("nested ADB/screenshot/record logs missing: %+v", actions)
	}
	for _, entry := range actions.Logs {
		if entry.Title != title {
			t.Fatalf("nested file/ADB log lost title: %+v", entry)
		}
	}
}

func TestAutomationLogContextConcurrentTitleIsolation(t *testing.T) {
	svc, _ := newActTestService()
	var ready sync.WaitGroup
	ready.Add(2)
	release := make(chan struct{})
	var workers sync.WaitGroup
	for _, title := range []string{"任务甲", "任务乙"} {
		workers.Add(1)
		go func(title string) {
			defer workers.Done()
			ctx := context.WithValue(context.Background(), automationTitleContextKey{}, title)
			ready.Done()
			<-release
			for i := 0; i < 10; i++ {
				svc.recordAutomationLogContext(ctx, AutomationLog{Type: LogTypeAction, Serial: "same-phone", Message: title})
			}
		}(title)
	}
	ready.Wait()
	close(release)
	workers.Wait()
	logs := svc.ListAutomationLogs(AutomationLogQuery{Serial: "same-phone", Limit: 1000})
	if logs.Count != 20 {
		t.Fatalf("concurrent logs missing: %d", logs.Count)
	}
	for _, entry := range logs.Logs {
		if entry.Title != entry.Message {
			t.Fatalf("concurrent task titles crossed: %+v", entry)
		}
	}
}

func TestActTitleIndependentConcurrentObservation(t *testing.T) {
	svc, _ := newActTestService()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var once sync.Once
	svc.SetUIXMLProvider(actSnapshotFunc(func(ctx context.Context, _ string) (uixml.Snapshot, error) {
		if ctx.Value(automationTitleContextKey{}) == "正在检查" {
			once.Do(func() { close(entered) })
			<-release
		}
		return uixml.Snapshot{XML: `<hierarchy><node text="目标" bounds="[1,1][90,30]"/></hierarchy>`, Source: "companion"}, nil
	}))
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- callActTitle(svc, "phone", "正在检查", "check 目标") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("act did not reach the nested observation")
	}
	if _, err := svc.Observe(t.Context(), ObserveRequest{Serial: "phone", Formats: []string{"ref"}}); err != nil {
		t.Fatal(err)
	}
	unblock()
	select {
	case w := <-done:
		if w.Code != 200 {
			t.Fatalf("check failed: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("act did not finish after releasing observation")
	}
	observes := svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone", Type: LogTypeObserve, Limit: 100})
	seenIndependent, seenTask := false, false
	for _, entry := range observes.Logs {
		switch entry.Title {
		case "":
			seenIndependent = true
		case "正在检查":
			seenTask = true
		default:
			t.Fatalf("unexpected title %+v", entry)
		}
	}
	if !seenIndependent || !seenTask {
		t.Fatalf("concurrent title isolation lost: %+v", observes)
	}
}

// Ensure a source that records while fetching retains the task context without
// a global serial-to-title mapping.
func TestActTitleContextReachesFrameFetcher(t *testing.T) {
	svc, _ := newActOCRTestService(t, func(int64) string { return `{"ocr_result":"","ocr_boxes":[]}` })
	var got string
	svc.SetFrameFetcher(func(ctx context.Context, _ string) (image.Image, []byte, error) {
		got, _ = ctx.Value(automationTitleContextKey{}).(string)
		return nil, nil, nil
	})
	callActTitle(svc, "phone", "采集页面", "check missing")
	if got != "采集页面" {
		t.Fatalf("nested source did not receive title context: %q", got)
	}
}

func TestActTitleRecordingCleanupErrorLog(t *testing.T) {
	svc, _ := artifactTestService(t)
	t.Setenv("FARM_ARTIFACT_MODE", "cleanup_fail")
	svc.SetDeviceProvider(func() []device.Device { return []device.Device{{Serial: "phone", State: "device"}} })
	w := callActTitle(svc, "phone", "录屏并清理", "mkdir title-results\nrecord 1 title-results")
	if w.Code != 200 {
		t.Fatalf("fixture recording failed: %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, entry := range svc.ListAutomationLogs(AutomationLogQuery{Type: LogTypeError}).Logs {
		if entry.Message == "设备产物临时文件清理失败" {
			found = true
			if entry.Title != "录屏并清理" {
				t.Fatalf("cleanup log lost task title: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("cleanup failure was not recorded")
	}
}
