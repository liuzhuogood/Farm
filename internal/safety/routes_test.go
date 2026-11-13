package safety

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafetyRoutesRejectInvalidInput(t *testing.T) {
	mux := http.NewServeMux()
	engine := NewEngine(PresetPayments)
	RegisterRoutes(mux, engine, NewApprovalCenter(0), nil)
	for _, tc := range []struct{ method, path, body string }{
		{"PUT", "/api/safety/policy", `{"preset":99}`},
		{"POST", "/api/safety/approvals/test/decide", `{"decision":"surprise"}`},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, body %s", tc.path, w.Code, w.Body.String())
		}
	}
	if engine.Preset() != PresetPayments {
		t.Fatal("invalid request changed policy")
	}
}

func TestSafetyHistoryDetailAndExportRoutes(t *testing.T) {
	dir := t.TempDir()
	history, err := OpenHistoryStore(filepath.Join(dir, "history.db"), filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	defer history.Close()
	_, err = history.db.ExecContext(context.Background(), `INSERT INTO tasks(id,title,started_at) VALUES('task-route','Route evidence',1)`)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewEngine(PresetPayments), NewApprovalCenter(0), history)
	for _, path := range []string{"/api/safety/history/tasks/task-route", "/api/safety/history/tasks/task-route/export"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Route evidence") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
