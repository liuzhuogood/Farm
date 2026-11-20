package automation

import (
	"context"
	"encoding/json"
	"farm/internal/safety"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestTaskRecordsListsAllDevicesWithoutChangingSingleDeviceAPI(t *testing.T) {
	svc, _ := newActTestService()
	history, err := safety.OpenHistoryStore(filepath.Join(t.TempDir(), "history.db"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = history.Close() })
	if err = svc.ConfigureTasks(filepath.Join(t.TempDir(), "tasks.json"), history); err != nil {
		t.Fatal(err)
	}
	live, err := svc.taskRuntime().create("phone", "实时任务")
	if err != nil {
		t.Fatal(err)
	}
	oldID, err := history.CreateTask(context.Background(), "tablet", "mcp", "历史任务")
	if err != nil {
		t.Fatal(err)
	}
	if err = history.FinishTask(context.Background(), oldID, "done", "已完成"); err != nil {
		t.Fatal(err)
	}

	mux := newTestMux(svc)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/automation/task-records", nil))
	if w.Code != 200 {
		t.Fatalf("task records: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Tasks []TaskRecordSummary `json:"tasks"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tasks) != 2 {
		t.Fatalf("records=%+v", out.Tasks)
	}
	serials := map[string]string{}
	for _, task := range out.Tasks {
		serials[task.ID] = task.Serial
	}
	if serials[live.ID] != "phone" || serials[oldID] != "tablet" {
		t.Fatalf("serial mapping=%v", serials)
	}
	if out.Tasks[0].StartedAt.Before(out.Tasks[1].StartedAt) {
		t.Fatalf("records not newest first: %+v", out.Tasks)
	}

	// Existing per-device listing must keep requiring exactly one serial.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/automation/tasks", nil))
	if w.Code != 400 {
		t.Fatalf("single-device API changed: %d %s", w.Code, w.Body.String())
	}
}

func newTestMux(svc *Service) *http.ServeMux {
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	return mux
}
