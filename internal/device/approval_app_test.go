package device

import "testing"

func TestApprovalAppVisualDoesNotInventUnknownIdentity(t *testing.T) {
	for _, pkg := range []string{"", "unregistered.package", "com.tencent.mm.fake"} {
		name, icon := ApprovalAppVisual(pkg)
		if name != "" || icon != "" {
			t.Fatalf("invented identity for %q: %s %s", pkg, name, icon)
		}
	}
	name, icon := ApprovalAppVisual("com.tencent.mm")
	if name != "微信" || icon != "wechat" {
		t.Fatalf("known mapping lost: %s %s", name, icon)
	}
}
