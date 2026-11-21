package idlescreen

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingValidationAndPersistenceFailure(t *testing.T) {
	m, _, _, _ := fixture()
	mux := http.NewServeMux()
	m.RegisterRoutes(mux)
	for _, body := range []string{`{}`, `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":true,"timeout":1}`, `{"enabled":true,"timeout_minutes":2}`, `{"enabled":true}{}`} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("PUT", "/api/system/idle-screen", strings.NewReader(body)))
		if w.Code != 400 || m.Config().Enabled {
			t.Fatalf("invalid body accepted: %s code=%d", body, w.Code)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("PUT", "/api/system/idle-screen", strings.NewReader(`{"enabled":true,"timeout_minutes":5}`)))
	if w.Code != 200 || !m.Config().Enabled || m.Config().TimeoutMinutes != 5 {
		t.Fatalf("valid config did not save: code=%d config=%+v", w.Code, m.Config())
	}
}
