package idlescreen

import (
	"encoding/json"
	"io"
	"net/http"
)

func (m *Manager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/system/idle-screen", func(w http.ResponseWriter, r *http.Request) { m.writeConfig(w) })
	mux.HandleFunc("PUT /api/system/idle-screen", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled        *bool `json:"enabled"`
			TimeoutMinutes *int  `json:"timeout_minutes"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.Enabled == nil {
			http.Error(w, "enabled 必须为布尔值", http.StatusBadRequest)
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			http.Error(w, "请求必须为单个 JSON 对象", http.StatusBadRequest)
			return
		}
		timeout := m.Config().TimeoutMinutes
		if body.TimeoutMinutes != nil {
			timeout = *body.TimeoutMinutes
		}
		if !allowedTimeoutMinutes[timeout] {
			http.Error(w, "熄屏时间必须为 1、5、10 或 30 分钟", http.StatusBadRequest)
			return
		}
		if err := m.Set(Config{Enabled: *body.Enabled, TimeoutMinutes: timeout}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m.writeConfig(w)
	})
}

func (m *Manager) writeConfig(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m.Config())
}
