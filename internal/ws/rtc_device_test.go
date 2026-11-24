package ws

import (
	"context"
	"os"
	"testing"
	"time"

	"farm/internal/scrcpy"
)

// TestRTCRequestKeyFrame 验证 WebRTC 通道的关键帧请求链路：
// RESET_VIDEO 是否真的让设备重启编码，并在多短时间内送出新的 config 包与关键帧。
// 需要真机：SCRCPY_BENCH_SERIAL=<serial> go test ./internal/ws -run TestRTCRequestKeyFrame -v
func TestRTCRequestKeyFrame(t *testing.T) {
	serial := os.Getenv("SCRCPY_BENCH_SERIAL")
	if serial == "" {
		t.Skip("skipping real-device keyframe test: set SCRCPY_BENCH_SERIAL to run")
	}
	adbPath := os.Getenv("ADB")
	if adbPath == "" {
		adbPath = "/Users/liuzhuo/Library/Android/sdk/platform-tools/adb"
		if _, err := os.Stat(adbPath); err != nil {
			adbPath = "adb"
		}
	}
	jarPath := os.Getenv("SCRCPY_JAR")
	if jarPath == "" {
		jarPath = "../../assets/scrcpy-server"
	}
	if _, err := os.Stat(jarPath); err != nil {
		t.Fatalf("locate scrcpy-server jar: %v", err)
	}

	h := NewHub(adbPath, jarPath)
	// 用独立的端口段，避免与正在运行的 Farm 后端抢同一台设备的 adb forward。
	h.nextPort = 39183
	defer h.Close()

	session, err := h.OpenRTCSession(serial)
	if err != nil {
		t.Fatalf("OpenRTCSession: %v", err)
	}
	defer session.Close()

	// 先把当前 GOP 收完，确保请求落在稳定状态。
	waitForKey(t, session, 15*time.Second, 0)

	requested := time.Now()
	accepted, err := session.RequestKeyFrame(context.Background())
	if err != nil {
		t.Fatalf("RequestKeyFrame: %v", err)
	}
	if !accepted {
		t.Fatal("RequestKeyFrame 被限流合并，首次调用不应被跳过")
	}
	// 紧随其后的第二次请求必须被限流，避免连续重启编码造成叠加停顿。
	throttled, throttleErr := session.RequestKeyFrame(context.Background())
	if throttleErr != nil {
		t.Fatalf("第二次 RequestKeyFrame: %v", throttleErr)
	}
	if throttled {
		t.Fatal("第二次 RequestKeyFrame 未被限流")
	}

	// 等新的 config 包出现，再等它后面的关键帧。
	var configAfter, keyAfter time.Duration
	deadline := time.After(15 * time.Second)
	for configAfter == 0 || keyAfter == 0 {
		select {
		case <-deadline:
			t.Fatalf("reset 后未收到新的 config/key（config=%v key=%v）", configAfter, keyAfter)
		case frame, ok := <-session.Frames():
			if !ok {
				t.Fatal("会话在等待关键帧时结束")
			}
			switch scrcpy.FrameKind(frame.Kind) {
			case scrcpy.FrameConfig:
				if configAfter == 0 && !frame.Replayed && time.Since(requested) > 0 {
					configAfter = time.Since(requested)
				}
			case scrcpy.FrameKey:
				if configAfter > 0 && keyAfter == 0 && !frame.Replayed {
					keyAfter = time.Since(requested)
				}
			}
		}
	}

	t.Logf("[PERF] RESET_VIDEO 后 config=%v keyframe=%v（设备 %s）", configAfter.Round(time.Millisecond), keyAfter.Round(time.Millisecond), serial)
}

// waitForKey 丢弃帧直到出现关键帧；timeout 为 0 时使用默认值。
func waitForKey(t *testing.T, session *RTCSession, timeout time.Duration, _ int) {
	t.Helper()
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatal("等待关键帧超时")
		case frame, ok := <-session.Frames():
			if !ok {
				t.Fatal("会话在等待关键帧时结束")
			}
			if scrcpy.FrameKind(frame.Kind) == scrcpy.FrameKey {
				return
			}
		}
	}
}
