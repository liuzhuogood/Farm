package ws

import (
	"testing"
	"time"
)

func TestKeyFrameGateAllowsFirstRequest(t *testing.T) {
	g := newKeyFrameGate(2 * time.Second)
	if !g.allow(time.Now()) {
		t.Fatal("首次请求应放行")
	}
}

func TestKeyFrameGateBlocksUntilKeyFrame(t *testing.T) {
	g := newKeyFrameGate(2 * time.Second)
	now := time.Now()
	g.markRequested(now)

	// 设备还没送出关键帧：即使超过固定短窗口（900ms）也必须继续挡住，
	// 否则会在新关键帧到达前重启编码，画面被反复打断。
	if g.allow(now.Add(900 * time.Millisecond)) {
		t.Fatal("等待关键帧期间不应放行")
	}
	if g.allow(now.Add(1900 * time.Millisecond)) {
		t.Fatal("等待关键帧期间不应放行（1.9s 实测最大值内）")
	}
}

func TestKeyFrameGateReleasesOnKeyFrame(t *testing.T) {
	g := newKeyFrameGate(2 * time.Second)
	now := time.Now()
	g.markRequested(now)
	g.onKeyFrame()

	if !g.allow(now.Add(10 * time.Millisecond)) {
		t.Fatal("收到新关键帧后应立即放行")
	}
}

func TestKeyFrameGateFallbackWhenKeyFrameNeverArrives(t *testing.T) {
	g := newKeyFrameGate(2 * time.Second)
	now := time.Now()
	g.markRequested(now)

	if !g.allow(now.Add(2 * time.Second)) {
		t.Fatal("兜底窗口到期后应放行，避免永久卡死")
	}
}
