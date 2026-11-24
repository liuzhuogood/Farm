package devicertc

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"farm/internal/ws"
)

//go:embed poc.html
var pocHTML []byte

// sessionIdleTimeout 回收既没连上也不再被轮询的桥接，避免泄漏 scrcpy 会话引用。
const sessionIdleTimeout = 60 * time.Second

// Server 提供 WebRTC 视频通道。它与 WS 投屏通道并行存在：
// WS 仍是默认路径，WebRTC 只作为额外通道，供需要 UDP 低延迟的观看端使用。
type Server struct {
	hub *ws.Hub

	mu        sync.Mutex
	bridges   map[string]*trackedBridge
	closed    bool
	pocEnable bool
}

type trackedBridge struct {
	*bridge
	answered  bool
	createdAt time.Time
	lastSeen  time.Time
}

// NewServer 创建 WebRTC 视频通道服务。
// ICE 只使用 host 候选：媒体端点就是本服务本身，不需要 STUN/TURN，也不需要打洞。
func NewServer(hub *ws.Hub) (*Server, error) {
	server := &Server{
		hub:       hub,
		bridges:   make(map[string]*trackedBridge),
		pocEnable: os.Getenv("FARM_WEBRTC_POC") != "",
	}
	go server.reap()
	log.Printf("[webrtc] WebRTC 视频通道已启用（H.264 直通，ICE-Lite，无 STUN/TURN）")
	return server, nil
}

// RegisterRoutes 挂载 WebRTC 通道接口。WS 投屏通道不受影响。
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/webrtc/offer", s.handleOffer)
	mux.HandleFunc("/api/webrtc/answer", s.handleAnswer)
	mux.HandleFunc("/api/webrtc/session", s.handleSession)
	if s.pocEnable {
		mux.HandleFunc("/api/webrtc/poc", s.handlePoc)
	}
}

// handleOffer 为指定设备生成 offer。请求体：{"serial":"..."}
func (s *Server) handleOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求体格式错误: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Serial == "" {
		http.Error(w, "missing serial", http.StatusBadRequest)
		return
	}

	id := fmt.Sprintf("%s-%d", req.Serial, time.Now().UnixNano())
	b, sdp, err := newBridge(id, req.Serial, s.hub)
	if err != nil {
		log.Printf("[webrtc] 建立桥接失败 serial=%s err=%v", req.Serial, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	now := time.Now()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		b.close()
		http.Error(w, "服务正在退出", http.StatusServiceUnavailable)
		return
	}
	s.bridges[id] = &trackedBridge{bridge: b, createdAt: now, lastSeen: now}
	s.mu.Unlock()

	meta := b.session.Meta()
	log.Printf("[webrtc] 已生成 offer serial=%s id=%s %dx%d", req.Serial, id, meta.Width, meta.Height)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     id,
		"serial": req.Serial,
		"sdp":    sdp,
		"codec":  meta.Codec,
		"width":  meta.Width,
		"height": meta.Height,
	})
}

// handleAnswer 应用浏览器回答并开始推流。请求体：{"id":"...","sdp":"..."}
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID  string `json:"id"`
		SDP string `json:"sdp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求体格式错误: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	entry, ok := s.bridges[req.ID]
	if ok {
		entry.lastSeen = time.Now()
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, "会话不存在或已过期", http.StatusNotFound)
		return
	}
	if req.SDP == "" {
		http.Error(w, "missing sdp", http.StatusBadRequest)
		return
	}
	if err := entry.setAnswer(req.SDP); err != nil {
		log.Printf("[webrtc] 应用 answer 失败 id=%s err=%v", req.ID, err)
		s.drop(req.ID)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	entry.answered = true
	entry.lastSeen = time.Now()
	s.mu.Unlock()
	log.Printf("[webrtc] 已开始推流 id=%s", req.ID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// handleSession 关闭会话：DELETE /api/webrtc/session?id=...
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	s.drop(id)
	w.WriteHeader(http.StatusNoContent)
}

// handlePoc 只在 FARM_WEBRTC_POC 设置时挂载，用于本地验证通道，不进正式界面。
func (s *Server) handlePoc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pocHTML)
}

func (s *Server) drop(id string) {
	s.mu.Lock()
	entry, ok := s.bridges[id]
	if ok {
		delete(s.bridges, id)
	}
	s.mu.Unlock()
	if ok {
		entry.close()
	}
}

// Close 关闭全部桥接。
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	entries := make([]*trackedBridge, 0, len(s.bridges))
	for _, entry := range s.bridges {
		entries = append(entries, entry)
	}
	s.bridges = make(map[string]*trackedBridge)
	s.mu.Unlock()
	for _, entry := range entries {
		entry.close()
	}
}

// reap 回收过期桥接：未完成协商或长时间没有观看端活动的一律释放。
func (s *Server) reap() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		var expired []string
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		for id, entry := range s.bridges {
			idle := now.Sub(entry.lastSeen)
			if (!entry.answered && idle > sessionIdleTimeout) || (entry.answered && idle > 10*time.Minute) {
				expired = append(expired, id)
			}
		}
		s.mu.Unlock()
		for _, id := range expired {
			log.Printf("[webrtc] 回收空闲会话 id=%s", id)
			s.drop(id)
		}
	}
}
