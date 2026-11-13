package companion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (s *Service) RegisterRoutes(mux *http.ServeMux, isOnline func(string) bool) {
	mux.HandleFunc("/api/devices/{serial}/companion", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fail := func(status int, code, message string) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			fail(http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are supported")
			return
		}
		serial := r.PathValue("serial")
		if isOnline != nil && !isOnline(serial) {
			fail(http.StatusBadGateway, "device_unavailable", "device is not online")
			return
		}
		if r.Method == http.MethodGet {
			status, _ := s.Status(r.Context(), serial)
			_ = json.NewEncoder(w).Encode(status)
			return
		}
		var request struct {
			Action string `json:"action"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			fail(http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			fail(http.StatusBadRequest, "invalid_request", "request must contain exactly one JSON object")
			return
		}
		if request.Action != "install" && request.Action != "enable" && request.Action != "disable" {
			fail(http.StatusBadRequest, "invalid_action", fmt.Sprintf("unsupported action %q", request.Action))
			return
		}
		status, err := s.action(r.Context(), serial, request.Action)
		if err != nil {
			fail(http.StatusBadGateway, "companion_failed", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(status)
	})
}
