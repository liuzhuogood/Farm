package safety

import (
	"testing"
	"time"
)

func TestApprovalKeepsCapturedAppIdentity(t *testing.T) {
	center := NewApprovalCenter(time.Minute)
	request := ApprovalRequest{Binding: ApprovalBinding{Device: "phone-B"}, Title: "审核", AppPackage: "com.android.settings", AppName: "系统设置", AppIcon: "settings"}
	info := center.Open(request)
	request.AppIcon = "wechat"
	list := center.List(false)
	if len(list) != 1 || list[0].AppIcon != "settings" || info.AppPackage != "com.android.settings" || info.AppName != "系统设置" {
		t.Fatalf("identity not captured: %+v", info)
	}
}
