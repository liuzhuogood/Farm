package automation

import (
	"errors"
	"strings"
	"testing"

	"farm/internal/action"
	"farm/internal/uixml"
)

func clearTestService() (*Service, *actTestTransport) {
	svc, transport := newActTestService()
	svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: `<hierarchy><node class="android.widget.EditText" text="existing input" focused="true" enabled="true" bounds="[0,0][90,40]"/></hierarchy>`, Source: "uiautomator"}})
	return svc, transport
}

func TestActClearRequiresFocusAndDoesNotChangeClipboard(t *testing.T) {
	svc, transport := clearTestService()
	w := callAct(svc, "clear")
	if w.Code != 200 || len(transport.requests) != 2 || !strings.Contains(w.Body.String(), "（2/2）") {
		t.Fatalf("status=%d body=%s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
	a, b := transport.requests[0], transport.requests[1]
	if a.Action != "key" || a.Keycode != 29 || a.MetaState != 0x1000 || b.Action != "key" || b.Keycode != 67 || b.MetaState != 0 || a.Mode != action.ModeSDK || b.Mode != action.ModeSDK {
		t.Fatalf("wrong clear keys: %+v %+v", a, b)
	}
	svc, transport = clearTestService()
	svc.SetUIXMLProvider(&mockUIXMLProvider{snapshot: uixml.Snapshot{XML: `<hierarchy><node class="android.widget.TextView" text="not an input" focused="true" bounds="[0,0][90,40]"/></hierarchy>`}})
	if w = callAct(svc, "clear"); w.Code != 400 || len(transport.requests) != 0 {
		t.Fatalf("clear touched non-input: %d %s", w.Code, w.Body.String())
	}
	svc, transport = clearTestService()
	if w = callAct(svc, "clear unwanted"); w.Code != 400 || len(transport.requests) != 0 {
		t.Fatal("clear accepted params")
	}
}

func TestActClearFailureStopsWithoutRetry(t *testing.T) {
	svc, transport := clearTestService()
	transport.failAt = 2
	transport.failErr = errors.New("delete injection failed")
	w := callAct(svc, "clear\nkeyevent home")
	if w.Code != 502 || len(transport.requests) != 2 || !strings.Contains(w.Body.String(), "（1/2）") {
		t.Fatalf("status=%d body=%s requests=%+v", w.Code, w.Body.String(), transport.requests)
	}
}
