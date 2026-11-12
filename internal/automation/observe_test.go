package automation

import (
	"bytes"
	"context"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"farm/internal/ocr"
	"farm/internal/uipage"
	"farm/internal/uixml"
)

type mockUIXMLProvider struct {
	snapshot uixml.Snapshot
	err      error
}

func (m *mockUIXMLProvider) FetchSnapshot(ctx context.Context, serial string) (uixml.Snapshot, error) {
	if m.err != nil {
		return uixml.Snapshot{}, m.err
	}
	return m.snapshot, nil
}

type mockUIPageProvider struct {
	pageInfo *uipage.PageInfo
	err      error
}

func (m *mockUIPageProvider) FetchPageInfo(ctx context.Context, serial string) (*uipage.PageInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pageInfo, nil
}

func TestObserveCompactFormattingAndDeduplication(t *testing.T) {
	xmlContent := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="com.android.settings:id/back" class="android.widget.ImageButton" package="com.android.settings" content-desc="返回" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[16,84][104,172]" />
    <node index="1" text="设置" resource-id="com.android.settings:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[120,84][960,172]" />
    <node index="2" text="WLAN" resource-id="com.android.settings:id/switch_widget" class="android.widget.Switch" package="com.android.settings" content-desc="" checkable="true" checked="true" clickable="true" enabled="true" focusable="true" focused="true" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[880,400][1040,480]" />
  </node>
</hierarchy>`

	svc := &Service{}
	svc.SetUIXMLProvider(&mockUIXMLProvider{
		snapshot: uixml.Snapshot{
			XML: xmlContent,
			DisplaySize: &uixml.DisplaySize{
				Width:  1080,
				Height: 2400,
			},
		},
	})
	svc.SetUIPageProvider(&mockUIPageProvider{
		pageInfo: &uipage.PageInfo{
			PackageName: "com.android.settings",
			Activity:    ".SettingsActivity",
			Display: &uipage.DisplayInfo{
				Width:    1080,
				Height:   2400,
				Rotation: 0,
			},
		},
	})

	// 模拟 FrameFetcher 和 OCR
	svc.SetFrameFetcher(func(ctx context.Context, serial string) (image.Image, []byte, error) {
		return nil, []byte("fake-image"), nil
	})

	// 创建可被调用的 OCR 服务
	svc.ocrService = ocr.NewService(nil, nil)
	// mock screen capture
	svc.ocrService.SetScreenCapture(func(ctx context.Context, serial string) ([]byte, error) {
		return []byte("fake-image"), nil
	})

	// 1. 全量感知测试 (diff=false)
	diffFalse := false
	out, err := svc.Observe(context.Background(), ObserveRequest{
		Serial:  "device-test-1",
		Formats: []string{"ref"},
		Diff:    &diffFalse,
	})
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}

	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d:\n%s", len(lines), out)
	}

	// 验证首行元数据
	if lines[0] != "screen 1080x2400 · app: Settings (com.android.settings) · portrait · keyboard hidden · focus [3]" {
		t.Errorf("metadata mismatch:\nGot:  %s\nWant: screen 1080x2400 · app: Settings (com.android.settings) · portrait · keyboard hidden · focus [3]", lines[0])
	}

	// 验证元素行
	if !strings.Contains(out, `[1] button "返回" @60,128`) {
		t.Errorf("missing button 返回 in output")
	}
	if !strings.Contains(out, `[2] text "设置" @540,128`) {
		t.Errorf("missing text 设置 in output")
	}
	if !strings.Contains(out, `[3] switch "WLAN" @960,440 focused checked`) {
		t.Errorf("missing switch WLAN in output: %s", out)
	}
}

func TestObserveDiffStreaming(t *testing.T) {
	xml1 := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" class="android.widget.FrameLayout" package="com.android.settings" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="com.android.settings:id/back" class="android.widget.ImageButton" package="com.android.settings" content-desc="返回" clickable="true" bounds="[16,84][104,172]" />
    <node index="1" text="WLAN" resource-id="com.android.settings:id/switch_widget" class="android.widget.Switch" package="com.android.settings" checkable="true" checked="false" clickable="true" bounds="[880,400][1040,480]" />
    <node index="2" text="蓝牙" resource-id="com.android.settings:id/bluetooth" class="android.widget.TextView" package="com.android.settings" clickable="true" bounds="[40,500][1040,600]" />
  </node>
</hierarchy>`

	xml2 := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" class="android.widget.FrameLayout" package="com.android.settings" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="com.android.settings:id/back" class="android.widget.ImageButton" package="com.android.settings" content-desc="返回" clickable="true" bounds="[16,84][104,172]" />
    <node index="1" text="WLAN" resource-id="com.android.settings:id/switch_widget" class="android.widget.Switch" package="com.android.settings" checkable="true" checked="true" clickable="true" bounds="[880,400][1040,480]" />
    <node index="2" text="蓝牙" resource-id="com.android.settings:id/bluetooth" class="android.widget.TextView" package="com.android.settings" clickable="true" bounds="[40,500][1040,600]" />
  </node>
</hierarchy>`

	mockXML := &mockUIXMLProvider{
		snapshot: uixml.Snapshot{
			XML: xml1,
			DisplaySize: &uixml.DisplaySize{
				Width:  1080,
				Height: 2400,
			},
		},
	}

	svc := &Service{}
	svc.SetUIXMLProvider(mockXML)
	svc.SetUIPageProvider(&mockUIPageProvider{
		pageInfo: &uipage.PageInfo{
			PackageName: "com.android.settings",
			Display: &uipage.DisplayInfo{
				Width:  1080,
				Height: 2400,
			},
		},
	})

	diffTrue := true

	// 第一次调用：此前无缓存，应输出全量
	out1, err := svc.Observe(context.Background(), ObserveRequest{
		Serial:  "dev-diff-test",
		Formats: []string{"ref"},
		Diff:    &diffTrue,
	})
	if err != nil {
		t.Fatalf("first Observe failed: %v", err)
	}
	if !strings.Contains(out1, `[2] switch "WLAN" @960,440`) {
		t.Fatalf("first observe should contain WLAN switch")
	}

	// 模拟 WLAN 开关变为 checked=true
	mockXML.snapshot.XML = xml2

	// 第二次调用：应当仅输出 diff 增量行
	out2, err := svc.Observe(context.Background(), ObserveRequest{
		Serial:  "dev-diff-test",
		Formats: []string{"ref"},
		Diff:    &diffTrue,
	})
	if err != nil {
		t.Fatalf("second Observe failed: %v", err)
	}

	lines2 := strings.Split(out2, "\n")
	if len(lines2) != 2 {
		t.Fatalf("expected exactly 2 lines in diff output (metadata + diff), got %d:\n%s", len(lines2), out2)
	}

	// 确认包含状态修改标记 ~
	if lines2[1] != `~ [2] switch "WLAN" @960,440 checked` {
		t.Errorf("expected modified WLAN switch line, got: %q", lines2[1])
	}
}

func TestObserveFormatDebugXML(t *testing.T) {
	rawXML := `<hierarchy rotation="0"><node text="debug-node" bounds="[0,0][100,100]"/></hierarchy>`
	svc := &Service{}
	svc.SetUIXMLProvider(&mockUIXMLProvider{
		snapshot: uixml.Snapshot{
			XML: rawXML,
		},
	})
	svc.SetUIPageProvider(&mockUIPageProvider{
		pageInfo: &uipage.PageInfo{PackageName: "com.debug"},
	})

	out, err := svc.Observe(context.Background(), ObserveRequest{
		Serial:  "dev-xml",
		Formats: []string{"xml"},
	})
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}

	if out != rawXML {
		t.Errorf("expected raw XML %q, got %q", rawXML, out)
	}
}

func TestObserveHTTPHandler(t *testing.T) {
	xmlContent := `<hierarchy rotation="0"><node text="测试" class="android.widget.TextView" clickable="true" bounds="[10,10][100,100]"/></hierarchy>`
	svc := &Service{}
	svc.SetUIXMLProvider(&mockUIXMLProvider{
		snapshot: uixml.Snapshot{XML: xmlContent},
	})
	svc.SetUIPageProvider(&mockUIPageProvider{
		pageInfo: &uipage.PageInfo{PackageName: "com.test.app"},
	})

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	// 1. POST 请求成功
	{
		reqBody := `{"serial": "dev-http-test", "formats": ["ref"], "settle_ms": 10}`
		req := httptest.NewRequest("POST", "/api/automation/observe", bytes.NewBufferString(reqBody))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("POST expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/plain") {
			t.Errorf("expected text/plain Content-Type, got %s", rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), `[1] button "测试" @55,55`) {
			t.Errorf("missing element in body: %s", rec.Body.String())
		}
	}

	// 2. GET 请求成功
	{
		req := httptest.NewRequest("GET", "/api/automation/observe?serial=dev-http-test&formats=ref", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 3. 缺少 serial 返回 400
	{
		req := httptest.NewRequest("POST", "/api/automation/observe", bytes.NewBufferString(`{}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing serial, got %d", rec.Code)
		}
	}

	// 4. 请求 xml 格式返回 Content-Type text/xml
	{
		reqBody := `{"serial": "dev-http-test", "formats": ["xml"]}`
		req := httptest.NewRequest("POST", "/api/automation/observe", bytes.NewBufferString(reqBody))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("POST xml expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/xml") {
			t.Errorf("expected text/xml Content-Type, got %s", rec.Header().Get("Content-Type"))
		}
	}
}
