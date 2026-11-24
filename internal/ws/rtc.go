package ws

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"farm/internal/scrcpy"
)

// WebRTC 通道与浏览器 WS 通道共用同一份 scrcpy 会话与视频帧。
// 这里的适配层只做三件事：暴露带 GOP 追帧的订阅、把帧转成外部可用结构、
// 把关键帧请求翻译成 scrcpy 的 RESET_VIDEO 控制消息。

// rtcKeyFrameMinInterval 限制 RESET_VIDEO 频率：每次请求都会让设备重启编码，
// 连续请求会把画面停顿叠加起来，因此观看端丢包请求必须被合并。
const rtcKeyFrameMinInterval = 900 * time.Millisecond

// RTCVideoFrame 是一帧 scrcpy 视频数据，供非 WS 传输通道（WebRTC）消费。
type RTCVideoFrame struct {
	Kind     uint8  // 0=config 1=key 2=delta 3=session
	PTS      uint64 // 设备时间戳，微秒；config/session 包为 0
	Payload  []byte
	Width    uint32 // 仅 session 包填充
	Height   uint32 // 仅 session 包填充
	Replayed bool   // 订阅时补发的 config/key，用于追帧而非实时画面
}

// RTCMeta 是会话建立时向观看端公布的视频参数。
type RTCMeta struct {
	Serial    string
	SessionID string
	Codec     string
	Width     uint32
	Height    uint32
}

// RTCSession 是一个设备的 WebRTC 视频会话。
type RTCSession struct {
	meta    RTCMeta
	ms      *managedSession
	release func()
	cancel  func()

	frames   chan RTCVideoFrame
	done     chan struct{}
	closeOne sync.Once

	mu             sync.Mutex
	lastKeyRequest time.Time

	dropped atomic.Uint64
	closed  atomic.Bool
}

// OpenRTCSession 打开设备的 WebRTC 视频会话，与 WS 投屏共享同一个 scrcpy 会话。
// 调用方必须在结束时调用 Close，否则 scrcpy 会话引用计数不会释放。
func (h *Hub) OpenRTCSession(serial string) (*RTCSession, error) {
	ms, _, release, err := h.acquireSession(serial)
	if err != nil {
		return nil, err
	}
	meta, src, cancel := h.subscribeBrowserVideoWithMeta(ms)
	s := &RTCSession{
		meta: RTCMeta{
			Serial:    serial,
			SessionID: meta.SessionID,
			Codec:     meta.Codec,
			Width:     meta.Width,
			Height:    meta.Height,
		},
		ms:      ms,
		release: release,
		cancel:  cancel,
		frames:  make(chan RTCVideoFrame, 192),
		done:    make(chan struct{}),
	}
	go s.convert(src)
	log.Printf("[rtc] 会话已打开 serial=%s codec=%s %dx%d", serial, meta.Codec, meta.Width, meta.Height)
	return s, nil
}

// Meta 返回会话建立时公布给观看端的视频参数。
func (s *RTCSession) Meta() RTCMeta { return s.meta }

// Frames 返回广播通道；关闭表示 scrcpy 会话结束，观看端应断开。
func (s *RTCSession) Frames() <-chan RTCVideoFrame { return s.frames }

// DroppedFrames 返回因消费过慢被丢弃的帧数（直播语义下跳帧优于阻塞共享订阅）。
func (s *RTCSession) DroppedFrames() uint64 { return s.dropped.Load() }

// RequestKeyFrame 让设备重启视频采集与编码，从而立即产生关键帧。
// 两次请求间隔小于 rtcKeyFrameMinInterval 时直接跳过，返回 false 表示被合并。
func (s *RTCSession) RequestKeyFrame(ctx context.Context) (bool, error) {
	s.mu.Lock()
	if !s.lastKeyRequest.IsZero() && time.Since(s.lastKeyRequest) < rtcKeyFrameMinInterval {
		s.mu.Unlock()
		return false, nil
	}
	s.lastKeyRequest = time.Now()
	s.mu.Unlock()

	if s.closed.Load() {
		return false, fmt.Errorf("会话已关闭")
	}
	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.msControlWrite(writeCtx); err != nil {
		return false, err
	}
	log.Printf("[rtc] 已请求关键帧（RESET_VIDEO） serial=%s", s.meta.Serial)
	return true, nil
}

// Close 释放订阅与 scrcpy 会话引用，可重复调用。
func (s *RTCSession) Close() {
	s.closeOne.Do(func() {
		s.closed.Store(true)
		close(s.done)
		if s.cancel != nil {
			s.cancel()
		}
		if s.release != nil {
			s.release()
		}
		log.Printf("[rtc] 会话已关闭 serial=%s dropped=%d", s.meta.Serial, s.dropped.Load())
	})
}

// msControlWrite 复用 scrcpy 控制 socket；底层写入自带串行化与超时。
func (s *RTCSession) msControlWrite(ctx context.Context) error {
	if s.ms == nil || s.ms.sess == nil || s.ms.sess.conn == nil {
		return fmt.Errorf("控制通道不可用")
	}
	return s.ms.sess.conn.WriteControlContext(ctx, scrcpy.ResetVideoEvent())
}

// convert 把内部共享帧转成公开结构。队列满时丢弃最旧的一帧而不是阻塞生产者：
// 请求方是其它观看端共用的广播器，阻塞会连带拖慢所有订阅者。
func (s *RTCSession) convert(src <-chan sharedVideoFrame) {
	defer close(s.frames)
	for {
		select {
		case <-s.done:
			return
		case sf, ok := <-src:
			if !ok {
				return
			}
			frame, ok := decodeSharedFrame(sf)
			if !ok {
				continue
			}
		deliver:
			for {
				select {
				case s.frames <- frame:
					break deliver
				case <-s.done:
					return
				default:
					select {
					case <-s.frames:
						s.dropped.Add(1)
					default:
					}
				}
			}
		}
	}
}

// decodeSharedFrame 解析 WS 广播使用的帧格式：[1B kind][8B PTS BE][payload]。
func decodeSharedFrame(sf sharedVideoFrame) (RTCVideoFrame, bool) {
	if len(sf.data) < 9 {
		return RTCVideoFrame{}, false
	}
	f := RTCVideoFrame{
		Kind:     sf.data[0],
		PTS:      binary.BigEndian.Uint64(sf.data[1:9]),
		Replayed: sf.replayed,
	}
	if scrcpy.FrameKind(f.Kind) == scrcpy.FrameSession {
		if len(sf.data) < 17 {
			return RTCVideoFrame{}, false
		}
		f.Width = binary.BigEndian.Uint32(sf.data[9:13])
		f.Height = binary.BigEndian.Uint32(sf.data[13:17])
		return f, true
	}
	f.Payload = sf.data[9:]
	if len(f.Payload) == 0 {
		return RTCVideoFrame{}, false
	}
	return f, true
}
