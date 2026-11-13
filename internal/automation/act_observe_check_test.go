package automation

import (
	"context"
	"errors"
	"image"
	"strings"
	"sync/atomic"
	"testing"

	"farm/internal/device"
	"farm/internal/uixml"
)

type actSnapshotFunc func(context.Context, string) (uixml.Snapshot, error)

func (fn actSnapshotFunc) FetchSnapshot(ctx context.Context, serial string) (uixml.Snapshot, error) {
	return fn(ctx, serial)
}

func TestActCheckMatchesObserveVisibleText(t *testing.T) {
	for _, tc := range []struct {
		name, xml, source, keyword, wantSource string
		matched                                bool
	}{
		{"tree text", `<hierarchy><node text="已完成下班打卡" bounds="[1,1][90,30]"/></hierarchy>`, "companion", "下班打卡", "tree:companion", true},
		{"tree description", `<hierarchy><node content-desc="打卡已完成" bounds="[1,1][90,30]"/></hierarchy>`, "uiautomator", "打卡已完成", "tree:uiautomator", true},
		{"unknown source", `<hierarchy><node text="已完成" bounds="[1,1][90,30]"/></hierarchy>`, "other-provider", "已完成", "tree", true},
		{"offscreen", `<hierarchy><node text="已完成" bounds="[100,1][190,30]"/></hierarchy>`, "companion", "已完成", "", false},
		{"metadata package", `<hierarchy><node package="com.android.settings" text="已完成" bounds="[1,1][90,30]"/></hierarchy>`, "companion", "com.android.settings", "", false},
		{"header app", `<hierarchy/>`, "uiautomator", "Settings", "", false},
		{"role", `<hierarchy><node class="android.widget.Button" text="已完成" bounds="[1,1][90,30]"/></hierarchy>`, "companion", "button", "", false},
		{"coordinates", `<hierarchy><node text="已完成" bounds="[1,1][90,30]"/></hierarchy>`, "companion", "45,15", "", false},
		// Resource labels printed by Observe are intentionally within scope.
		{"observe resource label", `<hierarchy><node resource-id="app:id/save" class="android.widget.Button" bounds="[1,1][90,30]"/></hierarchy>`, "uiautomator", "save", "tree:uiautomator", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newActOCRTestService(t, func(int64) string { return `{"ocr_result":"","ocr_boxes":[]}` })
			svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: tc.xml, Source: tc.source}})
			old := &Screen{Elements: []Element{{Label: "previous screen"}}}
			svc.screenCache.Store("phone", old)
			got, err := svc.checkActText(t.Context(), "phone", tc.keyword)
			if err != nil || got.Matched != tc.matched || got.Source != tc.wantSource {
				t.Fatalf("got %+v err=%v; want matched=%v source=%s", got, err, tc.matched, tc.wantSource)
			}
			if cached, _ := svc.screenCache.Load("phone"); cached != old {
				t.Fatal("check consumed the previous Observe diff cache")
			}
		})
	}
}

func TestActCheckUsesObserveOCRBoxes(t *testing.T) {
	svc, _ := newActOCRTestService(t, func(int64) string {
		return `{"ocr_result":"not-rendered text","ocr_boxes":[{"text":"OCR 可见文本","confidence":0.99,"x":0.1,"y":0.1,"w":0.5,"h":0.1}]}`
	})
	got, err := svc.checkActText(t.Context(), "phone", "可见文本")
	if err != nil || !got.Matched || got.Source != "ocr" {
		t.Fatalf("OCR visible box not matched: %+v %v", got, err)
	}
	got, err = svc.checkActText(t.Context(), "phone", "not-rendered text")
	if err != nil || got.Matched {
		t.Fatalf("non-rendered OCR aggregate must not match: %+v %v", got, err)
	}
}

func TestActCheckPreservesObserveFailureAndDegradation(t *testing.T) {
	svc, transport := newActOCRTestService(t, func(int64) string {
		return `{"ocr_result":"目标","ocr_boxes":[{"text":"目标","confidence":0.99,"x":0.1,"y":0.1,"w":0.5,"h":0.1}]}`
	})
	svc.SetUIXMLProvider(&mockUIXMLProvider{err: errors.New("tree unavailable")})
	w := callAct(svc, "check 目标\nkeyevent back")
	if w.Code != 502 || !strings.Contains(w.Body.String(), "observation_failed") || !strings.Contains(w.Body.String(), "tree unavailable") || len(transport.requests) != 0 {
		t.Fatalf("tree failure must follow Observe even when OCR could match: %d %s", w.Code, w.Body.String())
	}
	svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: `<hierarchy><node text="目标" bounds="[1,1][90,30]"/></hierarchy>`, Source: "companion"}})
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
		return nil, nil, errors.New("frame unavailable")
	})
	w = callAct(svc, "check 目标")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "匹配来源 tree:companion") {
		t.Fatalf("OCR frame degradation must preserve tree match: %d %s", w.Code, w.Body.String())
	}
	w = callAct(svc, "check 未出现")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "check_not_matched") {
		t.Fatalf("OCR frame degradation must preserve Observe tree-only scope: %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, entry := range svc.ListAutomationLogs(AutomationLogQuery{}).Logs {
		if entry.Message == "屏幕帧获取为空，OCR 将被跳过" {
			found = true
		}
	}
	if !found {
		t.Fatal("Observe degradation log was lost")
	}
	svc.SetUIXMLProvider(&mockUIXMLProvider{err: errors.New("both sources unavailable")})
	if _, err := svc.checkActText(t.Context(), "phone", "目标"); err == nil {
		t.Fatal("source failure became an empty match result")
	}
}

func TestActWaitReobservesTreeText(t *testing.T) {
	svc, transport := newActTestService()
	var reads atomic.Int64
	svc.SetUIXMLProvider(actSnapshotFunc(func(context.Context, string) (uixml.Snapshot, error) {
		xml := `<hierarchy/>`
		if reads.Add(1) > 1 {
			xml = `<hierarchy><node text="页面加载完成" bounds="[1,1][90,30]"/></hierarchy>`
		}
		return uixml.Snapshot{XML: xml, Source: "uiautomator"}, nil
	}))
	w := callAct(svc, "wait 2000 加载完成\nkeyevent back")
	if w.Code != 200 || reads.Load() < 2 || len(transport.requests) != 1 || !strings.Contains(w.Body.String(), "匹配来源 tree:uiautomator") {
		t.Fatalf("wait did not collect fresh Observe tree: %d %s", w.Code, w.Body.String())
	}
}

func TestActCheckCancellationUsesCurrentInvocation(t *testing.T) {
	svc, _ := newActTestService()
	ctx, cancel := context.WithCancel(t.Context())
	svc.SetUIXMLProvider(actSnapshotFunc(func(ctx context.Context, _ string) (uixml.Snapshot, error) {
		cancel()
		return uixml.Snapshot{}, ctx.Err()
	}))
	if _, err := svc.checkActText(ctx, "phone", "目标"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestActRemovedOCRRejectedBeforeDeviceInteraction(t *testing.T) {
	svc, transport := newActTestService()
	calls := 0
	svc.SetDeviceProvider(func() []device.Device { calls++; return nil })
	w := callAct(svc, "ocr\nkeyevent back")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_params") || calls != 0 || len(transport.requests) != 0 {
		t.Fatalf("removed OCR command reached device: %d %s calls=%d", w.Code, w.Body.String(), calls)
	}
}

func TestActCheckDoesNotUseAnotherObservationsCachedScreen(t *testing.T) {
	svc, _ := newActOCRTestService(t, func(int64) string { return `{"ocr_result":"","ocr_boxes":[]}` })
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
		// A simultaneous Observe can commit its own screen while this check is
		// collecting. This check must retain its directly returned snapshot.
		svc.screenCache.Store("phone", &Screen{Elements: []Element{{Label: "other observation target"}}})
		return nil, nil, nil
	})
	got, err := svc.checkActText(t.Context(), "phone", "other observation target")
	if err != nil || got.Matched {
		t.Fatalf("check used another invocation's screen: %+v %v", got, err)
	}
}
