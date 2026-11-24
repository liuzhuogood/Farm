package ws

import "time"

// keyFrameGate 控制 RESET_VIDEO 的触发频率。
//
// 只看时间窗口不够：实测一次 RESET_VIDEO 到新关键帧需要 0.8~1.9 秒，固定短窗口会在
// 设备还没送出关键帧时再次重启编码，画面被反复打断。因此以"已请求且尚未收到新关键帧"
// 作为主要闸门，另设兜底窗口处理关键帧丢失的情况。
type keyFrameGate struct {
	window    time.Duration
	requested bool
	at        time.Time
}

// newKeyFrameGate 创建闸门；window<=0 时使用默认兜底窗口。
func newKeyFrameGate(window time.Duration) *keyFrameGate {
	if window <= 0 {
		window = defaultKeyFrameFallback
	}
	return &keyFrameGate{window: window}
}

// defaultKeyFrameFallback 是"已请求但始终没等到关键帧"后的重试间隔。
const defaultKeyFrameFallback = 2 * time.Second

// allow 判断此刻是否可以再次请求关键帧。
func (g *keyFrameGate) allow(now time.Time) bool {
	if !g.requested {
		return true
	}
	return now.Sub(g.at) >= g.window
}

// markRequested 记录一次已生效的请求。
func (g *keyFrameGate) markRequested(now time.Time) {
	g.requested = true
	g.at = now
}

// onKeyFrame 在收到新的关键帧后解除闸门。
func (g *keyFrameGate) onKeyFrame() {
	g.requested = false
	g.at = time.Time{}
}
