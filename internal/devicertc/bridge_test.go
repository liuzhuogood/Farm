package devicertc

import (
	"encoding/base64"
	"strings"
	"testing"

	pion "github.com/pion/webrtc/v4"
)

// spsWith 造一个最小 SPS：NAL header + profile_idc + 约束位 + level_idc，够 profile 映射用。
func spsWith(profile, constraints, level byte) []byte {
	return []byte{0x67, profile, constraints, level, 0x00}
}

func TestSDPProfileLevelID(t *testing.T) {
	cases := []struct {
		name     string
		sps      []byte
		want     string
		fallback bool
	}{
		{"High level1.0 回落", spsWith(0x64, 0x00, 0x0a), "42e01f", true},
		{"High level3.1 回落", spsWith(0x64, 0x00, 0x1f), "42e01f", true},
		{"High 约束位 0x0c 回落", spsWith(0x64, 0x0c, 0x1f), "42e01f", true},
		{"Baseline 原样 profile", spsWith(0x42, 0x00, 0x1f), "42001f", false},
		{"Constrained baseline 保留约束位", spsWith(0x42, 0xe0, 0x1f), "42e01f", false},
		{"Main 原样 profile", spsWith(0x4d, 0x00, 0x1f), "4d001f", false},
		{"Main 带约束位", spsWith(0x4d, 0x40, 0x1f), "4d401f", false},
		{"Extended 回落", spsWith(0x58, 0x00, 0x1f), "42e01f", true},
		{"level 统一 3.1", spsWith(0x42, 0x00, 0x0a), "42001f", false},
		{"无 SPS", nil, "42e01f", false},
		{"SPS 太短", []byte{0x67, 0x64}, "42e01f", false},
	}
	for _, c := range cases {
		got, fallback := sdpProfileLevelID(c.sps)
		if got != c.want || fallback != c.fallback {
			t.Errorf("%s: got (%s,%v)，want (%s,%v)", c.name, got, fallback, c.want, c.fallback)
		}
	}
}

// TestFmtpLineKeepsDeviceParameterSets 声明的 profile 可以回落，但 sprop 必须是设备真实参数集，
// 否则解码器拿不到实际码流的 SPS/PPS。
func TestFmtpLineKeepsDeviceParameterSets(t *testing.T) {
	sps := spsWith(0x64, 0x00, 0x0a)
	pps := []byte{0x68, 0xeb, 0xe3, 0xcb, 0x22, 0xc0}
	b := &bridge{serial: "test", sps: sps, pps: pps}

	fmtp := b.fmtpLine()
	if !strings.Contains(fmtp, "profile-level-id=42e01f") {
		t.Fatalf("High profile 未回落: %s", fmtp)
	}
	wantSprop := base64.StdEncoding.EncodeToString(sps) + "," + base64.StdEncoding.EncodeToString(pps)
	if !strings.Contains(fmtp, "sprop-parameter-sets="+wantSprop) {
		t.Fatalf("sprop 不是设备真实参数集: %s", fmtp)
	}
}

func TestFmtpLineWithoutParameterSets(t *testing.T) {
	b := &bridge{serial: "test"}
	if got := b.fmtpLine(); got != "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f" {
		t.Fatalf("无 SPS 时应退默认 fmtp，got: %s", got)
	}
}

// TestOfferSDPUsesBrowserCompatibleProfile 走一遍真实路径：设备 High profile 时，
// 生成的 offer 必须声明浏览器能接受的 profile，并带上设备真实 sprop。
func TestOfferSDPUsesBrowserCompatibleProfile(t *testing.T) {
	sps := spsWith(0x64, 0x00, 0x0a)
	pps := []byte{0x68, 0xeb, 0xe3, 0xcb, 0x22, 0xc0}
	b := &bridge{serial: "test", sps: sps, pps: pps}
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
		t.Fatalf("newAPI: %v", err)
	}
	track, err := pion.NewTrackLocalStaticRTP(b.codec, "video", "farm-test")
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	pc, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("pc: %v", err)
	}
	defer pc.Close()
	if _, err := pc.AddTrack(track); err != nil {
		t.Fatalf("addTrack: %v", err)
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("createOffer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("setLocalDescription: %v", err)
	}
	sdp := pc.LocalDescription().SDP
	if !strings.Contains(sdp, "profile-level-id=42e01f") {
		t.Fatalf("offer 未声明浏览器可接受的 profile:\n%s", sdp)
	}
	if !strings.Contains(sdp, "sprop-parameter-sets="+base64.StdEncoding.EncodeToString(sps)) {
		t.Fatalf("offer 丢了设备真实参数集:\n%s", sdp)
	}
}
