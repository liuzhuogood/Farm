package devicertc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	pion "github.com/pion/webrtc/v4"

	"farm/internal/scrcpy"
	"farm/internal/ws"
)

const (
	videoMTU = 1200
	// videoPayloadType 由服务端单方决定：SDP 里只有这一种视频编码，浏览器按它回答。
	videoPayloadType pion.PayloadType = 102
	// 90kHz 时钟 / 每秒 30 帧，用于 PTS 回退时保持 RTP 时间戳单调。
	timestampStep = 90000 / 30
	// waitConfigTimeout 在握手前等待首帧 config 包，用于生成与码流一致的 SDP。
	waitConfigTimeout = 4 * time.Second
	// maxPendingFrames 限制握手期缓存的帧数，超出后丢弃等待下一个关键帧。
	maxPendingFrames = 240
)

// bridge 把一个设备会话的视频推给一个 WebRTC 观看端。
type bridge struct {
	id      string
	serial  string
	session *ws.RTCSession

	pc     *pion.PeerConnection
	track  *pion.TrackLocalStaticRTP
	codec  pion.RTPCodecCapability
	payl   *codecs.H264Payloader
	ssrc   uint32
	seq    uint16
	closed chan struct{}
	once   sync.Once

	pending []ws.RTCVideoFrame

	mu         sync.Mutex
	sps        []byte
	pps        []byte
	lastTS     uint32
	pendingKey bool
	started    bool
	width      uint32
	height     uint32

	framesSent atomic.Uint64
	framesDrop atomic.Uint64
}

// newBridge 建立会话、取到首帧 config 并生成 offer，返回 bridge 与 SDP。
func newBridge(id, serial string, hub *ws.Hub) (*bridge, string, error) {
	session, err := hub.OpenRTCSession(serial)
	if err != nil {
		return nil, "", fmt.Errorf("打开设备会话失败: %w", err)
	}
	meta := session.Meta()
	if meta.Codec != "h264" {
		session.Close()
		return nil, "", fmt.Errorf("设备编码 %s 暂不支持 WebRTC 通道，仅支持 h264", meta.Codec)
	}

	var ssrcBytes [4]byte
	_, _ = rand.Read(ssrcBytes[:])
	b := &bridge{
		id:      id,
		serial:  serial,
		session: session,
		payl:    &codecs.H264Payloader{},
		ssrc:    binary.BigEndian.Uint32(ssrcBytes[:]),
		seq:     uint16(time.Now().UnixNano() & 0x7FFF),
		closed:  make(chan struct{}),
		width:   meta.Width,
		height:  meta.Height,
	}
	if err := b.collectStartupFrames(); err != nil {
		session.Close()
		return nil, "", err
	}

	if len(b.sps) >= 4 {
		log.Printf("[webrtc] SDP 使用设备 SPS serial=%s profile=%02x%02x%02x sps=%dB pps=%dB", b.serial, b.sps[1], b.sps[2], b.sps[3], len(b.sps), len(b.pps))
	} else {
		log.Printf("[webrtc] 未取到 SPS，SDP 使用默认 profile serial=%s", b.serial)
	}

	b.codec = pion.RTPCodecCapability{
		MimeType:    pion.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: b.fmtpLine(),
		RTCPFeedback: []pion.RTCPFeedback{
			{Type: "nack"},
			{Type: "nack", Parameter: "pli"},
		},
	}
	api, err := newAPI(b.codec.SDPFmtpLine)
	if err != nil {
		session.Close()
		return nil, "", err
	}
	if b.track, err = pion.NewTrackLocalStaticRTP(b.codec, "video", "farm-"+serial); err != nil {
		session.Close()
		return nil, "", fmt.Errorf("创建本地视频轨失败: %w", err)
	}
	if b.pc, err = api.NewPeerConnection(pion.Configuration{}); err != nil {
		session.Close()
		return nil, "", fmt.Errorf("创建 PeerConnection 失败: %w", err)
	}
	if _, err = b.pc.AddTrack(b.track); err != nil {
		b.close()
		return nil, "", fmt.Errorf("添加视频轨失败: %w", err)
	}
	b.pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		log.Printf("[webrtc] 连接状态 serial=%s id=%s state=%s", b.serial, b.id, state)
		switch state {
		case pion.PeerConnectionStateFailed, pion.PeerConnectionStateClosed, pion.PeerConnectionStateDisconnected:
			// Disconnected 可能是短暂抖动，交给上层超时回收；这里只记录。
		}
	})

	offer, err := b.pc.CreateOffer(nil)
	if err != nil {
		b.close()
		return nil, "", fmt.Errorf("生成 offer 失败: %w", err)
	}
	if err = b.pc.SetLocalDescription(offer); err != nil {
		b.close()
		return nil, "", fmt.Errorf("设置本地描述失败: %w", err)
	}
	select {
	case <-pion.GatheringCompletePromise(b.pc):
	case <-time.After(3 * time.Second):
		b.close()
		return nil, "", errors.New("ICE 候选收集超时")
	}
	local := b.pc.LocalDescription()
	if local == nil {
		b.close()
		return nil, "", errors.New("本地描述为空")
	}
	return b, local.SDP, nil
}

// newAPI 按本次码流的 fmtp 注册唯一视频编码：SDP 里的 profile-level-id 必须与设备
// 实际码流一致，否则浏览器会用错误的解码配置（Chrome 软件解码只接受 constrained
// baseline），表现为连接成功但画面一直不出来。
// ICE 只用 host 候选并启用 ICE-Lite：媒体端点就是本服务，不需要 STUN/TURN。
func newAPI(fmtp string) (*pion.API, error) {
	settingEngine := pion.SettingEngine{}
	settingEngine.SetLite(true)
	settingEngine.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)

	mediaEngine := &pion.MediaEngine{}
	codec := pion.RTPCodecCapability{
		MimeType:     pion.MimeTypeH264,
		ClockRate:    90000,
		SDPFmtpLine:  fmtp,
		RTCPFeedback: []pion.RTCPFeedback{{Type: "nack"}, {Type: "nack", Parameter: "pli"}},
	}
	if err := mediaEngine.RegisterCodec(pion.RTPCodecParameters{RTPCodecCapability: codec, PayloadType: videoPayloadType}, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("注册 H.264 编码失败: %w", err)
	}
	loggerFactory := logging.NewDefaultLoggerFactory()
	loggerFactory.DefaultLogLevel = logging.LogLevelWarn
	settingEngine.LoggerFactory = loggerFactory
	return pion.NewAPI(pion.WithMediaEngine(mediaEngine), pion.WithSettingEngine(settingEngine)), nil
}

// collectStartupFrames 取握手期需要的信息：首帧 config 提供 SPS/PPS，
// 用于把 SDP 的 profile-level-id 与设备实际码流对齐（不一致时 Safari 会直接黑屏）。
func (b *bridge) collectStartupFrames() error {
	deadline := time.After(waitConfigTimeout)
	for {
		select {
		case <-deadline:
			log.Printf("[webrtc] 未在超时内取到 config 包，SDP 使用默认 profile serial=%s", b.serial)
			return nil
		case frame, ok := <-b.session.Frames():
			if !ok {
				return errors.New("设备视频流在握手前结束")
			}
			b.absorb(frame)
			switch scrcpy.FrameKind(frame.Kind) {
			case scrcpy.FrameConfig:
				return nil
			case scrcpy.FrameSession:
				b.width, b.height = frame.Width, frame.Height
			default:
				if len(b.pending) < maxPendingFrames {
					b.pending = append(b.pending, frame)
				}
			}
		}
	}
}

// absorb 记录 SPS/PPS 与分辨率，供 SDP 与后续关键帧使用。
func (b *bridge) absorb(frame ws.RTCVideoFrame) {
	switch scrcpy.FrameKind(frame.Kind) {
	case scrcpy.FrameConfig:
		sps, pps := splitParameterSets(frame.Payload)
		b.mu.Lock()
		if sps != nil {
			b.sps = sps
		}
		if pps != nil {
			b.pps = pps
		}
		b.pendingKey = true
		b.mu.Unlock()
	case scrcpy.FrameSession:
		b.mu.Lock()
		b.width, b.height = frame.Width, frame.Height
		b.mu.Unlock()
	}
}

// fmtpLine 用真实 SPS 生成 fmtp；取不到时退回 WebRTC 通用的 baseline 参数。
func (b *bridge) fmtpLine() string {
	b.mu.Lock()
	sps, pps := b.sps, b.pps
	b.mu.Unlock()
	if len(sps) < 4 {
		return "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
	}
	profile := fmt.Sprintf("%02x%02x%02x", sps[1], sps[2], sps[3])
	fmtp := "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + profile
	if pps != nil {
		fmtp += ";sprop-parameter-sets=" + base64.StdEncoding.EncodeToString(sps) + "," + base64.StdEncoding.EncodeToString(pps)
	}
	return fmtp
}

// setAnswer 应用浏览器回答并启动推流。
func (b *bridge) setAnswer(sdp string) error {
	log.Printf("[webrtc] 协商完成 serial=%s 视频编码=%s", b.serial, answerVideoSummary(sdp))
	if err := b.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: sdp}); err != nil {
		return fmt.Errorf("应用 answer 失败: %w", err)
	}
	go b.pump()
	go b.watchKeyFrameRequests()
	return nil
}

// pump 按序把帧打包成 RTP。config/session 包不产生 RTP；
// 关键帧先补发 SPS/PPS（payloader 会自动打包成 STAP-A），丢包后从关键帧恢复。
func (b *bridge) pump() {
	defer b.close()
	send := func(frame ws.RTCVideoFrame) {
		if err := b.writeFrame(frame); err != nil {
			log.Printf("[webrtc] 推流失败 serial=%s id=%s err=%v", b.serial, b.id, err)
		}
	}
	for _, frame := range b.pending {
		send(frame)
	}
	b.pending = nil
	for {
		select {
		case <-b.closed:
			return
		case frame, ok := <-b.session.Frames():
			if !ok {
				log.Printf("[webrtc] 设备视频流结束 serial=%s id=%s", b.serial, b.id)
				return
			}
			send(frame)
		}
	}
}

// writeFrame 写一帧到视频轨。时间戳来自设备 PTS（微秒 → 90kHz）。
func (b *bridge) writeFrame(frame ws.RTCVideoFrame) error {
	switch scrcpy.FrameKind(frame.Kind) {
	case scrcpy.FrameConfig:
		b.absorb(frame)
		return nil
	case scrcpy.FrameSession:
		b.absorb(frame)
		return nil
	}

	b.mu.Lock()
	if !b.started {
		if scrcpy.FrameKind(frame.Kind) != scrcpy.FrameKey {
			first := b.framesDrop.Load() == 0
			b.framesDrop.Add(1)
			b.mu.Unlock()
			if first {
				log.Printf("[webrtc] 等待首个关键帧，暂丢弃 delta serial=%s id=%s", b.serial, b.id)
			}
			return nil
		}
		b.started = true
		b.pendingKey = false
		log.Printf("[webrtc] 开始推流 serial=%s id=%s 首帧 pts=%dus", b.serial, b.id, frame.PTS)
	}
	payload := frame.Payload
	// 关键帧前补 SPS/PPS：设备只在 config 包和编码会话开头给出参数集，
	// 观看端中途加入或丢包重建时必须随关键帧一起收到。
	if scrcpy.FrameKind(frame.Kind) == scrcpy.FrameKey && b.sps != nil && b.pps != nil && !bytes.Contains(payload, []byte{0, 0, 1, 0x67}) {
		merged := make([]byte, 0, len(b.sps)+len(b.pps)+len(payload)+12)
		merged = appendStartCode(merged, b.sps)
		merged = appendStartCode(merged, b.pps)
		payload = append(merged, payload...)
	}
	ts := uint32(frame.PTS * 9 / 100)
	if ts <= b.lastTS {
		ts = b.lastTS + timestampStep
	}
	b.lastTS = ts
	b.framesSent.Add(1)
	b.mu.Unlock()

	payloads := b.payl.Payload(videoMTU, payload)
	if len(payloads) == 0 {
		return nil
	}
	for i, p := range payloads {
		marker := i == len(payloads)-1
		b.seq++
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    uint8(videoPayloadType),
				SequenceNumber: b.seq,
				Timestamp:      ts,
				SSRC:           b.ssrc,
				Marker:         marker,
			},
			Payload: p,
		}
		if err := b.track.WriteRTP(pkt); err != nil {
			return err
		}
	}
	return nil
}

// watchKeyFrameRequests 把观看端的 PLI/FIR 翻译成设备端的 RESET_VIDEO。
// 设备编码器无法被外部强制 IDR，只能重启编码会话换取一个新关键帧。
func (b *bridge) watchKeyFrameRequests() {
	sender, ok := b.sender()
	if !ok {
		return
	}
	for {
		select {
		case <-b.closed:
			return
		default:
		}
		packets, _, err := sender.ReadRTCP()
		if err != nil {
			select {
			case <-b.closed:
				return
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}
		for _, pkt := range packets {
			switch pkt.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				accepted, err := b.session.RequestKeyFrame(context.Background())
				if err != nil {
					log.Printf("[webrtc] 请求关键帧失败 serial=%s err=%v", b.serial, err)
				} else if accepted {
					b.mu.Lock()
					b.pendingKey = true
					b.mu.Unlock()
				}
			}
		}
	}
}

// sender 取回视频轨的 RTPSender，用于读取 RTCP 反馈。
func (b *bridge) sender() (*pion.RTPSender, bool) {
	for _, s := range b.pc.GetSenders() {
		if s.Track() != nil && s.Track().ID() == b.track.ID() {
			return s, true
		}
	}
	return nil, false
}

func (b *bridge) close() {
	b.once.Do(func() {
		close(b.closed)
		if b.pc != nil {
			_ = b.pc.Close()
		}
		b.session.Close()
		log.Printf("[webrtc] 桥接结束 serial=%s id=%s 发送=%d 丢弃=%d", b.serial, b.id, b.framesSent.Load(), b.framesDrop.Load())
	})
}

// answerVideoSummary 提取回答里视频 m 行与 H.264 rtpmap，便于确认协商结果。
func answerVideoSummary(sdp string) string {
	var parts []string
	for _, line := range strings.Split(sdp, "\r\n") {
		switch {
		case strings.HasPrefix(line, "m=video"):
			parts = append(parts, line)
		case strings.HasPrefix(line, "a=rtpmap:") && strings.Contains(line, "H264"):
			parts = append(parts, line)
		}
	}
	if len(parts) == 0 {
		return "无视频编码"
	}
	return strings.Join(parts, " ")
}

// appendStartCode 以 Annex-B 起始码追加一个 NAL。
func appendStartCode(dst, nalu []byte) []byte {
	dst = append(dst, 0, 0, 0, 1)
	return append(dst, nalu...)
}

// splitParameterSets 从 scrcpy 的 config 包（Annex-B）里取出 SPS 与 PPS。
func splitParameterSets(payload []byte) (sps, pps []byte) {
	for _, nalu := range annexBNALUs(payload) {
		if len(nalu) == 0 {
			continue
		}
		switch nalu[0] & 0x1F {
		case 7:
			sps = nalu
		case 8:
			pps = nalu
		}
	}
	return sps, pps
}

// annexBNALUs 按 3/4 字节起始码切分 Annex-B 码流。
func annexBNALUs(buf []byte) [][]byte {
	var out [][]byte
	i := 0
	for i+3 <= len(buf) {
		if buf[i] == 0 && buf[i+1] == 0 && buf[i+2] == 1 {
			start := i + 3
			end := len(buf)
			for j := start; j+3 <= len(buf); j++ {
				if buf[j] == 0 && buf[j+1] == 0 && buf[j+2] == 1 {
					end = j
					break
				}
				if j+4 <= len(buf) && buf[j] == 0 && buf[j+1] == 0 && buf[j+2] == 0 && buf[j+3] == 1 {
					end = j
					break
				}
			}
			out = append(out, buf[start:end])
			i = end
			continue
		}
		i++
	}
	return out
}
