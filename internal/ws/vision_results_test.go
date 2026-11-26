package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"farm/internal/vision"
	"github.com/gorilla/websocket"
)

// 浏览器关闭投屏页后 handler 必须返回：否则订阅与设备活动租约（空闲熄屏计时）会一直泄漏。
func TestServeVisionResultsReturnsOnClientClose(t *testing.T) {
	h := NewHub("adb", "server.jar")
	returned := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeVisionResults(w, r)
		close(returned)
	}))
	defer server.Close()

	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/vision/results?serial=phone", nil)
	if err != nil {
		t.Fatal(err)
	}
	// 不发送任何消息，直接断开；服务端必须能感知，而不是只在有下发消息时才发现。
	_ = c.Close()

	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("客户端断开后 ServeVisionResults 未返回")
	}
}

func TestResultSubscriberReceivesCurrentSessionState(t *testing.T) {
	h := NewHub("adb", "server.jar")
	empty := vision.Message{Type: "detection.result", SessionID: "session-1", Objects: []vision.Object{}}
	h.publishResult("device-1", empty)

	matching, unsubscribeMatching := h.subscribeResults("device-1", "session-1")
	defer unsubscribeMatching()
	select {
	case got := <-matching:
		if got.SessionID != empty.SessionID || got.Objects == nil || len(got.Objects) != 0 {
			t.Fatalf("unexpected replayed result: %#v", got)
		}
	default:
		t.Fatal("current session did not receive latest empty result")
	}

	otherSession, unsubscribeOther := h.subscribeResults("device-1", "session-2")
	defer unsubscribeOther()
	select {
	case got := <-otherSession:
		t.Fatalf("different session received stale result: %#v", got)
	default:
	}
}

func TestResultSubscriberIgnoresOtherSessionUpdates(t *testing.T) {
	h := NewHub("adb", "server.jar")
	current, unsubscribe := h.subscribeResults("device-1", "session-1")
	defer unsubscribe()
	h.publishResult("device-1", vision.Message{Type: "detection.result", SessionID: "session-2"})
	select {
	case got := <-current:
		t.Fatalf("subscriber received another session result: %#v", got)
	default:
	}
}

func TestResultSubscriberReceivesDeviceLevelBroadcast(t *testing.T) {
	h := NewHub("adb", "server.jar")
	// Subscriber with active session ID
	sub, unsubscribe := h.subscribeResults("device-1", "browser-session-123")
	defer unsubscribe()

	// Device-level broadcast (msg.SessionID == "")
	deviceMsg := vision.Message{
		Type:     "detection.result",
		DeviceID: "device-1",
		Objects: []vision.Object{
			{Label: "login_button", Confidence: 0.95, X: 0.1, Y: 0.2, W: 0.3, H: 0.4},
		},
	}
	h.publishResult("device-1", deviceMsg)

	select {
	case got := <-sub:
		if len(got.Objects) != 1 || got.Objects[0].Label != "login_button" {
			t.Fatalf("subscriber did not receive expected device-level broadcast: %#v", got)
		}
	default:
		t.Fatal("subscriber with session_id failed to receive device-level broadcast")
	}

	// Late-joining subscriber with session_id should receive the replayed device-level lastResult
	lateSub, lateUnsub := h.subscribeResults("device-1", "browser-session-456")
	defer lateUnsub()

	select {
	case got := <-lateSub:
		if len(got.Objects) != 1 || got.Objects[0].Label != "login_button" {
			t.Fatalf("late subscriber did not receive replayed device-level broadcast: %#v", got)
		}
	default:
		t.Fatal("late subscriber with session_id failed to receive replayed device-level broadcast")
	}
}
