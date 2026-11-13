package automation

import (
	"context"
	"farm/internal/uixml"
)

// FastUIXMLProvider owns the fast service's lifecycle and coordinates a dump
// fallback with its service enable/disable operations on the same device.
type FastUIXMLProvider interface {
	FetchWithFallback(context.Context, string, func(context.Context, string) (uixml.Snapshot, error)) (uixml.Snapshot, error)
}
type preferredUIXMLProvider struct {
	fast     FastUIXMLProvider
	fallback UIXMLProvider
}

func (p preferredUIXMLProvider) FetchSnapshot(ctx context.Context, serial string) (uixml.Snapshot, error) {
	return p.fast.FetchWithFallback(ctx, serial, p.fallback.FetchSnapshot)
}

// SetFastUIXMLProvider preserves the configured UI dump provider as fallback.
// Configure once before serving requests; the inspector remains on raw UI dump.
func (s *Service) SetFastUIXMLProvider(provider FastUIXMLProvider) {
	s.uiXML = preferredUIXMLProvider{fast: provider, fallback: s.getUIXMLProvider()}
}
