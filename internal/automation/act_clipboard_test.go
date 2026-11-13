package automation

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"farm/internal/action"
	"farm/internal/inputconfig"
	"farm/internal/scrcpy"
)

func TestActCopyPasteKeysUseSDK(t *testing.T) {
	svc, transport := newActTestService()
	svc.inputConfig = inputconfig.New(nil)
	if err := svc.inputConfig.Set(action.ModeADB); err != nil {
		t.Fatal(err)
	}
	w := callAct(svc, "copy\npaste")
	if w.Code != 200 || len(transport.requests) != 2 {
		t.Fatalf("clipboard keys failed: %d %s", w.Code, w.Body.String())
	}
	for i, code := range []uint32{278, 279} {
		req := transport.requests[i]
		if req.Action != "key" || req.Keycode != code || req.Mode != action.ModeSDK {
			t.Fatalf("wrong clipboard request: %+v", req)
		}
	}
	for _, body := range []string{"copy text", "paste text", "past"} {
		w = callAct(svc, body)
		if w.Code != 400 || len(transport.requests) != 2 {
			t.Fatalf("invalid clipboard command reached device: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestActSetCopyOnlySetsClipboardAndPreservesRawText(t *testing.T) {
	svc, transport := newActTestService()
	var previous uint64
	for _, text := range []string{"中文🙂", "  大小写 AbZ é  ", "空格\t保留", " "} {
		w := callAct(svc, "set-copy "+text)
		if w.Code != 200 {
			t.Fatalf("set-copy failed: %d %s", w.Code, w.Body.String())
		}
		req := transport.requests[len(transport.requests)-1]
		raw := req.Raw
		if req.Action != "raw" || req.Mode != action.ModeSDK || len(raw) < 14 || raw[0] != scrcpy.TypeSetClipboard || raw[9] != 0 || binary.BigEndian.Uint32(raw[10:14]) != uint32(len([]byte(text))) || string(raw[14:]) != text || !utf8.Valid(raw[14:]) {
			t.Fatalf("set-copy must set Unicode clipboard without pasting: %+v bytes=%v", req, raw)
		}
		sequence := binary.BigEndian.Uint64(raw[1:9])
		if sequence <= previous {
			t.Fatalf("clipboard sequence did not advance: %d -> %d", previous, sequence)
		}
		previous = sequence
	}
	before := len(transport.requests)
	w := callAct(svc, "set-copy")
	if w.Code != 400 || len(transport.requests) != before {
		t.Fatalf("empty set-copy accepted: %d %s", w.Code, w.Body.String())
	}
	for _, entry := range svc.ListAutomationLogs(AutomationLogQuery{}).Logs {
		if strings.Contains(entry.Message, "大小写") || strings.Contains(formatAutomationFields(entry.Fields), "大小写") {
			t.Fatalf("clipboard text leaked to logs: %+v", entry)
		}
	}
}

func TestActClipboardFailuresAreNotRetried(t *testing.T) {
	for _, command := range []string{"copy", "paste", "set-copy 中文🙂"} {
		t.Run(command, func(t *testing.T) {
			svc, transport := newActTestService()
			transport.failAt, transport.failErr = 1, errors.New("transport lost after injection")
			w := callAct(svc, command+"\nkeyevent back")
			if w.Code != 502 || len(transport.requests) != 1 || !strings.Contains(w.Body.String(), "完成 0/2") || !strings.Contains(w.Body.String(), "失败行 1") {
				t.Fatalf("clipboard command retried or next command executed: %d %s requests=%d", w.Code, w.Body.String(), len(transport.requests))
			}
		})
	}
}
