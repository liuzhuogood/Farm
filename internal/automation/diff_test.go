package automation

import (
	"strings"
	"testing"
)

func TestDiffFallbackScenarios(t *testing.T) {
	screenA := &Screen{
		Width:       1080,
		Height:      2400,
		AppName:     "Settings",
		PackageName: "com.android.settings",
		Orientation: "portrait",
		Elements: []Element{
			{Ref: 1, Role: "button", Label: "返回", CenterX: 60, CenterY: 120},
			{Ref: 2, Role: "switch", Label: "WLAN", CenterX: 980, CenterY: 420, Checked: false},
		},
	}

	// 1. Prev 为 nil，降级为全量
	{
		out, isDiff := Diff(nil, screenA, []string{"ref"})
		if isDiff {
			t.Errorf("expected isDiff=false for nil prev")
		}
		if !strings.Contains(out, `[1] button "返回" @60,120`) {
			t.Errorf("expected full render output when prev is nil, got: %s", out)
		}
	}

	// 2. 前台 App 切换，降级为全量
	{
		screenB := &Screen{
			Width:       1080,
			Height:      2400,
			AppName:     "WeChat",
			PackageName: "com.tencent.mm",
			Orientation: "portrait",
			Elements: []Element{
				{Ref: 1, Role: "text", Label: "微信", CenterX: 540, CenterY: 120},
			},
		}

		out, isDiff := Diff(screenA, screenB, []string{"ref"})
		if isDiff {
			t.Errorf("expected isDiff=false when package changes")
		}
		if !strings.Contains(out, "app: WeChat (com.tencent.mm)") {
			t.Errorf("expected full render of WeChat, got: %s", out)
		}
	}

	// 3. 屏幕旋转横屏，降级为全量
	{
		screenRotated := &Screen{
			Width:       2400,
			Height:      1080,
			AppName:     "Settings",
			PackageName: "com.android.settings",
			Orientation: "landscape",
			Elements:    screenA.Elements,
		}

		out, isDiff := Diff(screenA, screenRotated, []string{"ref"})
		if isDiff {
			t.Errorf("expected isDiff=false on screen rotation")
		}
		if !strings.Contains(out, "landscape") {
			t.Errorf("expected full render with landscape, got: %s", out)
		}
	}

	// 4. 变更行数超过全屏容量，降级为全量
	{
		screenDrastic := &Screen{
			Width:       1080,
			Height:      2400,
			AppName:     "Settings",
			PackageName: "com.android.settings",
			Orientation: "portrait",
			Elements: []Element{
				{Ref: 1, Role: "text", Label: "全然不同A", CenterX: 100, CenterY: 200},
				{Ref: 2, Role: "text", Label: "全然不同B", CenterX: 100, CenterY: 400},
			},
		}

		out, isDiff := Diff(screenA, screenDrastic, []string{"ref"})
		if isDiff {
			t.Errorf("expected isDiff=false when total changes >= capacity")
		}
		if strings.HasPrefix(out, "+ ") || strings.HasPrefix(out, "- ") {
			t.Errorf("expected full render, got diff lines: %s", out)
		}
	}
}

func TestDiffIncrementalChanges(t *testing.T) {
	prev := &Screen{
		Width:       1080,
		Height:      2400,
		AppName:     "Settings",
		PackageName: "com.android.settings",
		Orientation: "portrait",
		Elements: []Element{
			{Ref: 1, Role: "button", Label: "返回", CenterX: 60, CenterY: 120, ResourceID: "com.android.settings:id/back"},
			{Ref: 2, Role: "input", Label: "搜索设置", CenterX: 540, CenterY: 240, ResourceID: "com.android.settings:id/search", Focused: false, Editable: true},
			{Ref: 3, Role: "switch", Label: "WLAN", CenterX: 980, CenterY: 420, ResourceID: "com.android.settings:id/wlan", Checked: false},
			{Ref: 4, Role: "text", Label: "旧提示", CenterX: 540, CenterY: 800, ResourceID: "com.android.settings:id/tip"},
			{Ref: 5, Role: "button", Label: "底部操作", CenterX: 540, CenterY: 2200, ResourceID: "com.android.settings:id/btn_action"},
			{Ref: 6, Role: "button", Label: "辅助操作", CenterX: 540, CenterY: 2300, ResourceID: "com.android.settings:id/btn_aux"},
		},
		OCR: []OCRItem{
			{Text: "已保存", CenterX: 540, CenterY: 1000},
		},
	}

	next := &Screen{
		Width:       1080,
		Height:      2400,
		AppName:     "Settings",
		PackageName: "com.android.settings",
		Orientation: "portrait",
		Elements: []Element{
			// Ref 1 未变
			{Ref: 1, Role: "button", Label: "返回", CenterX: 60, CenterY: 120, ResourceID: "com.android.settings:id/back"},
			// Ref 2 获得焦点 (状态变化 ~)
			{Ref: 2, Role: "input", Label: "搜索设置", CenterX: 540, CenterY: 240, ResourceID: "com.android.settings:id/search", Focused: true, Editable: true},
			// Ref 3 开启了开关 (状态变化 ~)
			{Ref: 3, Role: "switch", Label: "WLAN", CenterX: 980, CenterY: 420, ResourceID: "com.android.settings:id/wlan", Checked: true},
			// Ref 4 "旧提示" 消失了 (-)
			// 新增 Ref 4 "确定" 弹窗浮层 (+)
			{Ref: 4, Role: "button", Label: "确定", CenterX: 540, CenterY: 1200, ResourceID: "com.android.settings:id/ok"},
			// 保留底部操作
			{Ref: 5, Role: "button", Label: "底部操作", CenterX: 540, CenterY: 2200, ResourceID: "com.android.settings:id/btn_action"},
			{Ref: 6, Role: "button", Label: "辅助操作", CenterX: 540, CenterY: 2300, ResourceID: "com.android.settings:id/btn_aux"},
		},
		OCR: []OCRItem{
			// "已保存" 消失 (-)
			// 新增 "加载中..." (+)
			{Text: "加载中...", CenterX: 540, CenterY: 1300},
		},
	}

	out, isDiff := Diff(prev, next, []string{"ref", "ocr"})
	if !isDiff {
		t.Fatalf("expected isDiff=true for incremental changes, got:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("too few lines in diff output: %d", len(lines))
	}

	// 首行必须为元数据
	if !strings.HasPrefix(lines[0], "screen 1080x2400 · app: Settings (com.android.settings)") {
		t.Errorf("first line should be metadata, got: %s", lines[0])
	}

	// 验证包含期望的 diff 标记
	hasRemovedTip := false
	hasAddedOK := false
	hasModifiedInput := false
	hasModifiedWLAN := false
	hasRemovedSavedOCR := false
	hasAddedLoadingOCR := false

	for _, line := range lines[1:] {
		switch {
		case line == `- [4] text "旧提示" @540,800`:
			hasRemovedTip = true
		case line == `+ [4] button "确定" @540,1200`:
			hasAddedOK = true
		case line == `~ [2] input "搜索设置" @540,240 focused editable`:
			hasModifiedInput = true
		case line == `~ [3] switch "WLAN" @980,420 checked`:
			hasModifiedWLAN = true
		case line == `- ocr "已保存" @540,1000`:
			hasRemovedSavedOCR = true
		case line == `+ ocr "加载中..." @540,1300`:
			hasAddedLoadingOCR = true
		case strings.Contains(line, "返回"):
			t.Errorf("unchanged element '返回' should not appear in diff, got: %s", line)
		}
	}

	if !hasRemovedTip {
		t.Errorf("missing - [4] 旧提示 in diff")
	}
	if !hasAddedOK {
		t.Errorf("missing + [4] 确定 in diff")
	}
	if !hasModifiedInput {
		t.Errorf("missing ~ [2] input 搜索设置 in diff")
	}
	if !hasModifiedWLAN {
		t.Errorf("missing ~ [3] switch WLAN in diff")
	}
	if !hasRemovedSavedOCR {
		t.Errorf("missing - ocr 已保存 in diff")
	}
	if !hasAddedLoadingOCR {
		t.Errorf("missing + ocr 加载中... in diff")
	}
}
