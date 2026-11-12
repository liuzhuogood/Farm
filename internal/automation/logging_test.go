package automation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"farm/internal/uixml"
)

func TestRecordAutomationLogWritesToRing(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, "")

	svc.recordAutomationLog(AutomationLog{
		Type:    LogTypeObserve,
		Serial:  "device-a",
		Message: "感知请求开始",
		Fields:  AutomationLogFields("formats", "ref", "settle_ms", 100),
	})

	resp := svc.ListAutomationLogs(AutomationLogQuery{Serial: "device-a"})
	if resp.Count != 1 {
		t.Fatalf("期望 1 条日志，实际 %d 条", resp.Count)
	}

	entry := resp.Logs[0]
	if entry.Type != LogTypeObserve {
		t.Errorf("期望类型 %s，实际 %s", LogTypeObserve, entry.Type)
	}
	if entry.Serial != "device-a" {
		t.Errorf("期望 serial=device-a，实际 %s", entry.Serial)
	}
	if entry.Fields["formats"] != "ref" {
		t.Errorf("期望 formats=ref，实际 %v", entry.Fields["formats"])
	}
	if entry.Fields["settle_ms"] != 100 {
		t.Errorf("期望 settle_ms=100，实际 %v", entry.Fields["settle_ms"])
	}
	if entry.At.IsZero() {
		t.Error("日志时间戳不应为零值")
	}
}

func TestListAutomationLogsFiltersByTypeAndSerial(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, "")

	svc.recordAutomationLog(AutomationLog{Type: LogTypeObserve, Serial: "dev-1", Message: "a"})
	svc.recordAutomationLog(AutomationLog{Type: LogTypeDiff, Serial: "dev-1", Message: "b"})
	svc.recordAutomationLog(AutomationLog{Type: LogTypeObserve, Serial: "dev-2", Message: "c"})

	// 按 serial 过滤
	resp := svc.ListAutomationLogs(AutomationLogQuery{Serial: "dev-1"})
	if resp.Count != 2 {
		t.Fatalf("dev-1 期望 2 条日志，实际 %d 条", resp.Count)
	}

	// 按 type 过滤
	resp = svc.ListAutomationLogs(AutomationLogQuery{Serial: "dev-1", Type: LogTypeDiff})
	if resp.Count != 1 {
		t.Fatalf("dev-1 + diff 期望 1 条日志，实际 %d 条", resp.Count)
	}
	if resp.Logs[0].Message != "b" {
		t.Errorf("期望消息 b，实际 %s", resp.Logs[0].Message)
	}

	// limit 生效
	resp = svc.ListAutomationLogs(AutomationLogQuery{Serial: "", Limit: 2})
	if resp.Count != 2 {
		t.Fatalf("期望 limit=2 生效，实际 %d 条", resp.Count)
	}
}

func TestObserveEmitsStructuredLogs(t *testing.T) {
	xmlContent := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="设置" resource-id="com.android.settings:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[120,84][960,172]" />
  </node>
</hierarchy>`

	svc := NewService(nil, nil, nil, nil, "")
	svc.SetUIXMLProvider(&mockUIXMLProvider{
		snapshot: uixml.Snapshot{
			XML:         xmlContent,
			DisplaySize: &uixml.DisplaySize{Width: 1080, Height: 2400},
		},
	})

	out, err := svc.Observe(context.Background(), ObserveRequest{
		Serial:  "log-device",
		Formats: []string{"ref"},
	})
	if err != nil {
		t.Fatalf("Observe 失败: %v", err)
	}
	if !strings.Contains(out, "[1] text \"设置\"") {
		t.Errorf("期望包含元素行，实际输出:\n%s", out)
	}

	logs := svc.ListAutomationLogs(AutomationLogQuery{Serial: "log-device"})
	if logs.Count == 0 {
		t.Fatal("期望产生日志记录，实际为空")
	}

	// 必须覆盖请求、阶段、差异、响应四类日志
	types := map[string]bool{}
	for _, entry := range logs.Logs {
		types[entry.Type] = true
	}
	for _, want := range []string{LogTypeObserve, LogTypeObserveStage, LogTypeDiff, LogTypeResponse} {
		if !types[want] {
			t.Errorf("期望日志类型 %s 未出现，实际类型: %v", want, types)
		}
	}
}

func TestHandleObserveLogsErrorPaths(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, "")

	// 缺少 serial
	req := httptest.NewRequest(http.MethodPost, "/api/automation/observe", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.handleObserve(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d", rec.Code)
	}

	logs := svc.ListAutomationLogs(AutomationLogQuery{Type: LogTypeError})
	if logs.Count == 0 {
		t.Fatal("期望记录错误日志，实际为空")
	}
}

func TestHandleAutomationLogsEndpoint(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, "")
	svc.recordAutomationLog(AutomationLog{
		Type:    LogTypeObserve,
		Serial:  "endpoint-device",
		Message: "接口日志自省验证",
	})

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/automation/logs?serial=endpoint-device", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}

	var resp AutomationLogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("期望 1 条日志，实际 %d 条", resp.Count)
	}
	if resp.Logs[0].Message != "接口日志自省验证" {
		t.Errorf("期望消息匹配，实际 %s", resp.Logs[0].Message)
	}
}

func TestFormatAutomationFieldsStableOrder(t *testing.T) {
	out := formatAutomationFields(AutomationLogFields("b", 2, "a", 1, "c", 3))
	if out != " a=1 b=2 c=3" {
		t.Errorf("期望字段按字典序输出，实际 %q", out)
	}

	if got := formatAutomationFields(nil); got != "" {
		t.Errorf("期望空字段输出空串，实际 %q", got)
	}
}
