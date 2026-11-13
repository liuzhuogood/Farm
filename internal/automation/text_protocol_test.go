package automation

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSkillTextErrorsKeepLegacyDefaults(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, "")
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	for _, tc := range []struct{ query, content, body string }{
		{"?format=text", "text/plain", "状态 失败"},
		{"", "application/json", `"code":"missing_serial"`},
	} {
		r := httptest.NewRequest("GET", "/api/automation/observe"+tc.query, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 400 || !strings.Contains(w.Header().Get("Content-Type"), tc.content) || !strings.Contains(w.Body.String(), tc.body) {
			t.Fatalf("query=%s code=%d body=%s", tc.query, w.Code, w.Body.String())
		}
	}
}

func TestAutomationLogsTextFiltersAndEmpty(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, "")
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	svc.recordAutomationLog(AutomationLog{Type: LogTypeAction, Serial: "phone", Message: "动作完成"})
	for _, tc := range []struct{ query, want, absent string }{
		{"?format=text&serial=phone&limit=1", "动作完成", `"logs":`},
		{"?format=text&serial=absent", "没有匹配的调用记录。", "动作完成"},
		{"?format=text&type=automation.error", "没有匹配的调用记录。", "动作完成"},
		{"?serial=phone", `"logs":`, "没有匹配的调用记录。"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/automation/logs"+tc.query, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), tc.absent) {
			t.Fatalf("query=%s body=%s", tc.query, w.Body.String())
		}
	}
}
