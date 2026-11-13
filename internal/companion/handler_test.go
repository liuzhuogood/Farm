package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesAreMoreSpecificThanDevicePrefixAndReportActualState(t *testing.T) {
	f := &fakeADB{version: "2"}
	s := newFakeService(f, validResponse())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/devices/", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "wrong handler", 599) })
	s.RegisterRoutes(mux, func(serial string) bool { return serial == "10.0.0.30:5555" })
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/devices/10.0.0.30:5555/companion", nil))
	if !strings.Contains(rec.Body.String(), `"update_available":false`) {
		t.Fatalf("missing explicit update flag: %s", rec.Body.String())
	}
	var status Status
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &status) != nil || status.Installed || status.Ready || status.Serial != "10.0.0.30:5555" {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/devices/10.0.0.30:5555/companion", strings.NewReader(`{"action":"enable"}`)))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), `"error":"companion_failed"`) {
		t.Fatalf("enable absent app: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/devices/unknown/companion", nil))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "device_unavailable") {
		t.Fatalf("offline: %d %s", rec.Code, rec.Body.String())
	}
}
