package automation

import (
	"fmt"
	"net/http"
)

// Existing clients retain their default response contract; the skill explicitly
// selects text, including failures, without requiring special request headers.
func writeAutomationProtocolError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if r.URL.Query().Get("format") != "text" {
		writeError(w, status, code, message)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprintf(w, "状态 失败\n错误 %s: %q\n", code, message)
}

func (s *Service) serveAutomationLogsText(w http.ResponseWriter, response AutomationLogResponse) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if response.Count == 0 {
		fmt.Fprintln(w, "没有匹配的调用记录。")
		return
	}
	for _, entry := range response.Logs {
		title := ""
		if entry.Title != "" {
			title = fmt.Sprintf(" · title=%q", entry.Title)
		}
		fmt.Fprintf(w, "%s · 设备 %q%s · %s · %q%s\n", entry.At.Format("2006-01-02T15:04:05.000Z07:00"), entry.Serial, title, entry.Type, entry.Message, formatAutomationFields(entry.Fields))
	}
}
