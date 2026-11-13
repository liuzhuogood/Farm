package automation

import (
	"context"
	"farm/internal/device"
	"farm/internal/uipage"
	"log"
	"time"
)

// Limit optional lookups even if a provider is waiting on its device lock.
var approvalAppSlots = make(chan struct{}, 2)

// ApprovalApp captures the foreground application on the approval's bound device.
// Metadata is optional and bounded; a failed observation must not prevent review.
func (s *Service) ApprovalApp(ctx context.Context, serial string) (pkg, name, icon string) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	select {
	case approvalAppSlots <- struct{}{}:
	default:
		log.Printf("[Approval] app metadata lookup at capacity serial=%s", serial)
		return "", "", ""
	}
	type result struct {
		page *uipage.PageInfo
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-approvalAppSlots }()
		page, err := s.getUIPageProvider().FetchPageInfo(ctx, serial)
		done <- result{page, err}
	}()
	select {
	case value := <-done:
		if value.err != nil {
			log.Printf("[Approval] app metadata unavailable serial=%s: %v", serial, value.err)
			return "", "", ""
		}
		if value.page == nil || value.page.PackageName == "" {
			return "", "", ""
		}
		name, icon = device.ApprovalAppVisual(value.page.PackageName)
		return value.page.PackageName, name, icon
	case <-ctx.Done():
		log.Printf("[Approval] app metadata lookup timed out serial=%s: %v", serial, ctx.Err())
		return "", "", ""
	}
}
