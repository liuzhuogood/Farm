package automation

import (
	"testing"
)

func TestElementLineFormattingAndParsing(t *testing.T) {
	tests := []struct {
		name     string
		elem     Element
		wantLine string
	}{
		{
			name: "Button simple",
			elem: Element{
				Ref:     1,
				Role:    "button",
				Label:   "返回",
				CenterX: 60,
				CenterY: 120,
			},
			wantLine: `[1] button "返回" @60,120`,
		},
		{
			name: "Text simple",
			elem: Element{
				Ref:     2,
				Role:    "text",
				Label:   "设置",
				CenterX: 540,
				CenterY: 120,
			},
			wantLine: `[2] text "设置" @540,120`,
		},
		{
			name: "Input focused editable",
			elem: Element{
				Ref:      3,
				Role:     "input",
				Label:    "搜索设置",
				CenterX:  540,
				CenterY:  240,
				Focused:  true,
				Editable: true,
			},
			wantLine: `[3] input "搜索设置" @540,240 focused editable`,
		},
		{
			name: "Switch checked",
			elem: Element{
				Ref:     4,
				Role:    "switch",
				Label:   "WLAN",
				CenterX: 980,
				CenterY: 420,
				Checked: true,
			},
			wantLine: `[4] switch "WLAN" @980,420 checked`,
		},
		{
			name: "Checkbox disabled and selected",
			elem: Element{
				Ref:      5,
				Role:     "checkbox",
				Label:    "同意用户协议",
				CenterX:  150,
				CenterY:  800,
				Disabled: true,
				Selected: true,
			},
			wantLine: `[5] checkbox "同意用户协议" @150,800 disabled selected`,
		},
		{
			name: "Password input with scrollable",
			elem: Element{
				Ref:        6,
				Role:       "input",
				Label:      "密码",
				CenterX:    300,
				CenterY:    600,
				Editable:   true,
				Password:   true,
				Scrollable: true,
			},
			wantLine: `[6] input "密码" @300,600 editable scrollable password`,
		},
		{
			name: "Special characters: quotes, backslash, newlines",
			elem: Element{
				Ref:     7,
				Role:    "button",
				Label:   "点击\"测试\" \\\n换行\t制表",
				CenterX: 100,
				CenterY: 200,
			},
			wantLine: `[7] button "点击\"测试\" \\\n换行\t制表" @100,200`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotLine := tc.elem.Line()
			if gotLine != tc.wantLine {
				t.Fatalf("Line() = %q, want %q", gotLine, tc.wantLine)
			}

			// 反序列化校验
			parsed, err := ParseElementLine(gotLine)
			if err != nil {
				t.Fatalf("ParseElementLine(%q) error: %v", gotLine, err)
			}

			if parsed.Ref != tc.elem.Ref {
				t.Errorf("Ref: got %d, want %d", parsed.Ref, tc.elem.Ref)
			}
			if parsed.Role != tc.elem.Role {
				t.Errorf("Role: got %q, want %q", parsed.Role, tc.elem.Role)
			}
			if parsed.Label != tc.elem.Label {
				t.Errorf("Label: got %q, want %q", parsed.Label, tc.elem.Label)
			}
			if parsed.CenterX != tc.elem.CenterX || parsed.CenterY != tc.elem.CenterY {
				t.Errorf("Center: got @%d,%d, want @%d,%d", parsed.CenterX, parsed.CenterY, tc.elem.CenterX, tc.elem.CenterY)
			}
			if parsed.Focused != tc.elem.Focused ||
				parsed.Editable != tc.elem.Editable ||
				parsed.Checked != tc.elem.Checked ||
				parsed.Disabled != tc.elem.Disabled ||
				parsed.Selected != tc.elem.Selected ||
				parsed.Scrollable != tc.elem.Scrollable ||
				parsed.Password != tc.elem.Password {
				t.Errorf("States mismatch between original and parsed")
			}
		})
	}
}

func TestOCRLineFormattingAndParsing(t *testing.T) {
	tests := []struct {
		name     string
		item     OCRItem
		wantLine string
	}{
		{
			name: "Normal OCR text",
			item: OCRItem{
				Text:    "已连接到 5G_WiFi",
				CenterX: 320,
				CenterY: 480,
			},
			wantLine: `ocr "已连接到 5G_WiFi" @320,480`,
		},
		{
			name: "OCR text with quotes and escapes",
			item: OCRItem{
				Text:    "包含 \"引号\" 和 \\ 反斜杠",
				CenterX: 850,
				CenterY: 920,
			},
			wantLine: `ocr "包含 \"引号\" 和 \\ 反斜杠" @850,920`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotLine := tc.item.Line()
			if gotLine != tc.wantLine {
				t.Fatalf("Line() = %q, want %q", gotLine, tc.wantLine)
			}

			parsed, err := ParseOCRLine(gotLine)
			if err != nil {
				t.Fatalf("ParseOCRLine(%q) error: %v", gotLine, err)
			}

			if parsed.Text != tc.item.Text {
				t.Errorf("Text: got %q, want %q", parsed.Text, tc.item.Text)
			}
			if parsed.CenterX != tc.item.CenterX || parsed.CenterY != tc.item.CenterY {
				t.Errorf("Center: got @%d,%d, want @%d,%d", parsed.CenterX, parsed.CenterY, tc.item.CenterX, tc.item.CenterY)
			}
		})
	}
}

func TestEventLineFormattingAndParsing(t *testing.T) {
	tests := []struct {
		name     string
		ev       Event
		wantLine string
	}{
		{
			name: "Toast message",
			ev: Event{
				Type:    "toast",
				Message: "已保存到相册",
			},
			wantLine: `toast: "已保存到相册"`,
		},
		{
			name: "Notification with sender",
			ev: Event{
				Type:    "notification",
				Sender:  "WeChat",
				Message: "收到一条新消息",
			},
			wantLine: `notification: WeChat · "收到一条新消息"`,
		},
		{
			name: "Notification without sender",
			ev: Event{
				Type:    "notification",
				Message: "系统更新已就绪",
			},
			wantLine: `notification: "系统更新已就绪"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotLine := tc.ev.Line()
			if gotLine != tc.wantLine {
				t.Fatalf("Line() = %q, want %q", gotLine, tc.wantLine)
			}

			parsed, err := ParseEventLine(gotLine)
			if err != nil {
				t.Fatalf("ParseEventLine(%q) error: %v", gotLine, err)
			}
			if parsed.Type != tc.ev.Type {
				t.Errorf("Type: got %q, want %q", parsed.Type, tc.ev.Type)
			}
			if parsed.Sender != tc.ev.Sender {
				t.Errorf("Sender: got %q, want %q", parsed.Sender, tc.ev.Sender)
			}
			if parsed.Message != tc.ev.Message {
				t.Errorf("Message: got %q, want %q", parsed.Message, tc.ev.Message)
			}
		})
	}
}

func TestMetadataLineAndScreenRender(t *testing.T) {
	screen := &Screen{
		Width:           1080,
		Height:          2400,
		AppName:         "Settings",
		PackageName:     "com.android.settings",
		Orientation:     "portrait",
		KeyboardVisible: false,
		FocusRef:        3,
		Events: []Event{
			{Type: "toast", Message: "已保存到相册"},
		},
		Elements: []Element{
			{Ref: 1, Role: "button", Label: "返回", CenterX: 60, CenterY: 120},
			{Ref: 2, Role: "text", Label: "设置", CenterX: 540, CenterY: 120},
			{Ref: 3, Role: "input", Label: "搜索设置", CenterX: 540, CenterY: 240, Focused: true, Editable: true},
			{Ref: 4, Role: "switch", Label: "WLAN", CenterX: 980, CenterY: 420, Checked: true},
		},
		OCR: []OCRItem{
			{Text: "已连接到 5G_WiFi", CenterX: 320, CenterY: 480},
		},
	}

	wantMeta := "screen 1080x2400 · app: Settings (com.android.settings) · portrait · keyboard hidden · focus [3]"
	gotMeta := screen.MetadataLine()
	if gotMeta != wantMeta {
		t.Fatalf("MetadataLine() = %q, want %q", gotMeta, wantMeta)
	}

	w, h, appName, pkg, orient, kb, focus, err := ParseMetadataLine(gotMeta)
	if err != nil {
		t.Fatalf("ParseMetadataLine error: %v", err)
	}
	if w != 1080 || h != 2400 || appName != "Settings" || pkg != "com.android.settings" || orient != "portrait" || kb != false || focus != 3 {
		t.Errorf("ParseMetadataLine mismatch: w=%d h=%d app=%s pkg=%s orient=%s kb=%v focus=%d", w, h, appName, pkg, orient, kb, focus)
	}

	rendered := screen.Render([]string{"ref", "ocr"})
	expectedText := `screen 1080x2400 · app: Settings (com.android.settings) · portrait · keyboard hidden · focus [3]
toast: "已保存到相册"
[1] button "返回" @60,120
[2] text "设置" @540,120
[3] input "搜索设置" @540,240 focused editable
[4] switch "WLAN" @980,420 checked
ocr "已连接到 5G_WiFi" @320,480`

	if rendered != expectedText {
		t.Errorf("Render() mismatch:\nGot:\n%s\nWant:\n%s", rendered, expectedText)
	}

	// formats 过滤测试：仅 ref
	refOnly := screen.Render([]string{"ref"})
	if refOnly == rendered || len(refOnly) >= len(rendered) {
		t.Errorf("Render ref only should exclude OCR lines")
	}

	// formats 过滤测试：仅 ocr
	ocrOnly := screen.Render([]string{"ocr"})
	if ocrOnly == rendered {
		t.Errorf("Render ocr only should exclude element lines")
	}
}
