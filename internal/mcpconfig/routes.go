package mcpconfig

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
)

func RegisterRoutes(mux *http.ServeMux, m *Manager) {
	respond := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	local := func(r *http.Request) bool {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			return false
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			return false
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return false
			}
			if u.Host == r.Host {
				return true
			}
			backend, _, err := net.SplitHostPort(r.Host)
			loopback := func(host string) bool { return host == "localhost" || net.ParseIP(host).IsLoopback() }
			// Vite's configured 5173 proxy rewrites Host to the backend.
			return err == nil && loopback(backend) && u.Scheme == "http" && u.Port() == "5173" && loopback(u.Hostname())
		}
		return true
	}
	mux.HandleFunc("GET /api/ai-integrations/mcp", func(w http.ResponseWriter, r *http.Request) {
		if !local(r) {
			respond(w, 403, map[string]string{"error": "MCP 配置仅允许本机访问"})
			return
		}
		respond(w, 200, map[string]any{"endpoint": m.endpoint, "clients": m.List()})
	})
	mutate := func(w http.ResponseWriter, r *http.Request) {
		if !local(r) {
			respond(w, 403, map[string]string{"error": "MCP 配置仅允许本机同源操作"})
			return
		}
		result, err := m.Mutate(r.Context(), r.PathValue("client"), r.Method == http.MethodPost)
		if err != nil {
			respond(w, 409, map[string]string{"error": err.Error()})
			return
		}
		respond(w, 200, result)
	}
	mux.HandleFunc("POST /api/ai-integrations/mcp/{client}", mutate)
	mux.HandleFunc("DELETE /api/ai-integrations/mcp/{client}", mutate)
}
