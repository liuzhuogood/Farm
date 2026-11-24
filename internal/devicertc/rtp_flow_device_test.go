package devicertc

import (
	"os"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"farm/internal/ws"
)

// TestRTCPacketFlow 用 pion 自己当观看端，验证桥接是否真的把 RTP 发出去了。
// 需要真机且该设备当前没有被其它进程占用 ADB forward：
//
//	SCRCPY_BENCH_SERIAL=<serial> go test ./internal/devicertc -run TestRTCPacketFlow -v
func TestRTCPacketFlow(t *testing.T) {
	serial := os.Getenv("SCRCPY_BENCH_SERIAL")
	if serial == "" {
		t.Skip("skipping real-device rtp test: set SCRCPY_BENCH_SERIAL to run")
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

	hub := ws.NewHub(adbPath, jarPath)
	defer hub.Close()

	b, offer, err := newBridge("test-"+serial, serial, hub)
	if err != nil {
		t.Fatalf("newBridge: %v", err)
	}
	defer b.close()

	remote := pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: offer}
	viewer, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("viewer pc: %v", err)
	}
	defer viewer.Close()

	got := make(chan struct{}, 1)
	viewer.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		buf := make([]byte, 1500)
		total := 0
		for {
			n, _, readErr := track.Read(buf)
			if readErr != nil {
				return
			}
			total += n
			if total > 0 {
				select {
				case got <- struct{}{}:
				default:
				}
			}
			if total > 20000 {
				return
			}
		}
	})
	if err = viewer.SetRemoteDescription(remote); err != nil {
		t.Fatalf("setRemoteDescription: %v", err)
	}
	answer, err := viewer.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("createAnswer: %v", err)
	}
	if err = viewer.SetLocalDescription(answer); err != nil {
		t.Fatalf("setLocalDescription: %v", err)
	}
	gatherDone := pion.GatheringCompletePromise(viewer)
	select {
	case <-gatherDone:
	case <-time.After(5 * time.Second):
		t.Fatal("viewer ICE 收集超时")
	}
	if err = b.setAnswer(viewer.LocalDescription().SDP); err != nil {
		t.Fatalf("setAnswer: %v", err)
	}

	select {
	case <-got:
		t.Logf("[PERF] 观看端已收到 RTP（设备 %s，发送帧 %d）", serial, b.framesSent.Load())
	case <-time.After(20 * time.Second):
		t.Fatalf("20 秒内未收到任何 RTP（发送帧 %d，丢弃帧 %d）", b.framesSent.Load(), b.framesDrop.Load())
	}
}
