package automation

import (
	"context"
	"farm/internal/uipage"
	"testing"
	"time"
)

type approvalPageProvider struct {
	serial  string
	timeout bool
}

func (p *approvalPageProvider) FetchPageInfo(ctx context.Context, serial string) (*uipage.PageInfo, error) {
	p.serial = serial
	if p.timeout {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &uipage.PageInfo{PackageName: "com.android.settings"}, nil
}
func TestApprovalAppUsesBoundDeviceAndExactMapping(t *testing.T) {
	p := &approvalPageProvider{}
	s := NewService(nil, nil, nil, nil, "")
	s.SetUIPageProvider(p)
	pkg, name, icon := s.ApprovalApp(t.Context(), "phone-B")
	if p.serial != "phone-B" || pkg != "com.android.settings" || name != "系统设置" || icon != "settings" {
		t.Fatalf("wrong identity: %s %s %s %s", p.serial, pkg, name, icon)
	}
}
func TestApprovalAppMetadataTimeoutIsBounded(t *testing.T) {
	s := NewService(nil, nil, nil, nil, "")
	s.SetUIPageProvider(&approvalPageProvider{timeout: true})
	start := time.Now()
	pkg, name, icon := s.ApprovalApp(t.Context(), "offline")
	if pkg != "" || name != "" || icon != "" {
		t.Fatal("failed metadata must be absent")
	}
	if time.Since(start) > time.Second {
		t.Fatal("metadata exceeded bounded timeout")
	}
}
