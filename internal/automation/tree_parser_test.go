package automation

import (
	"testing"
)

const sampleAndroidDumpXML = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="com.android.settings:id/action_bar" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,72][1080,216]">
      <node index="0" text="" resource-id="com.android.settings:id/back" class="android.widget.ImageButton" package="com.android.settings" content-desc="返回" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[16,84][104,172]" />
      <node index="1" text="设置" resource-id="com.android.settings:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[120,84][960,172]" />
    </node>
    <node index="1" text="" resource-id="com.android.settings:id/search_box" class="android.widget.EditText" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="true" scrollable="false" long-clickable="true" password="false" selected="false" bounds="[40,240][1040,360]">
      <node index="0" text="搜索设置" resource-id="" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[60,260][500,340]" />
    </node>
    <node index="2" text="WLAN" resource-id="com.android.settings:id/switch_widget" class="android.widget.Switch" package="com.android.settings" content-desc="" checkable="true" checked="true" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[880,400][1040,480]" />
  </node>
</hierarchy>`

func TestParseHierarchyXML(t *testing.T) {
	elements, focusRef, err := ParseHierarchyXML([]byte(sampleAndroidDumpXML), 1080, 2400)
	if err != nil {
		t.Fatalf("ParseHierarchyXML failed: %v", err)
	}

	if len(elements) < 4 {
		t.Fatalf("expected at least 4 elements, got %d", len(elements))
	}

	// 1. back button
	btn := elements[0]
	if btn.Role != "button" || btn.Label != "返回" {
		t.Errorf("elem 0: got role=%s label=%s, want button / 返回", btn.Role, btn.Label)
	}
	if btn.CenterX != (16+104)/2 || btn.CenterY != (84+172)/2 {
		t.Errorf("elem 0 center: got @%d,%d, want @60,128", btn.CenterX, btn.CenterY)
	}

	// 2. title text
	txt := elements[1]
	if txt.Role != "text" || txt.Label != "设置" {
		t.Errorf("elem 1: got role=%s label=%s, want text / 设置", txt.Role, txt.Label)
	}

	// 3. search input (focused)
	input := elements[2]
	if input.Role != "input" || !input.Focused || !input.Editable {
		t.Errorf("elem 2: got role=%s focused=%v editable=%v", input.Role, input.Focused, input.Editable)
	}
	if focusRef != input.Ref {
		t.Errorf("focusRef: got %d, want %d", focusRef, input.Ref)
	}

	// 4. switch checked
	sw := elements[3]
	if sw.Role != "switch" || !sw.Checked || sw.Label != "WLAN" {
		t.Errorf("elem 3: got role=%s checked=%v label=%s", sw.Role, sw.Checked, sw.Label)
	}
}

func TestCompanionUsesActualEditableStateForCustomViews(t *testing.T) {
	data := []byte(`<hierarchy><node class="example.CustomView" editable="true" enabled="true" bounds="[0,0][100,50]"/><node class="android.widget.EditText" editable="false" enabled="true" bounds="[0,60][100,100]"/></hierarchy>`)
	elements, _, err := ParseHierarchyXML(data, 100, 200)
	if err != nil || len(elements) != 2 || elements[0].Role != "input" || !elements[0].Editable || elements[1].Editable {
		t.Fatalf("elements=%+v err=%v", elements, err)
	}
}
