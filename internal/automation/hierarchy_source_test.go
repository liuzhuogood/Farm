package automation

import (
	"context"
	"errors"
	"farm/internal/uixml"
	"testing"
)

type fastHierarchyStub struct {
	fallback bool
	err      error
}

func (p fastHierarchyStub) FetchWithFallback(ctx context.Context, serial string, fallback func(context.Context, string) (uixml.Snapshot, error)) (uixml.Snapshot, error) {
	if p.fallback {
		return fallback(ctx, serial)
	}
	return uixml.Snapshot{Source: "companion", XML: "<hierarchy/>"}, p.err
}
func TestFastHierarchyRetainsConfiguredDumpAndPropagatesFailure(t *testing.T) {
	dumpErr := errors.New("dump failed")
	for _, tc := range []struct {
		name    string
		fast    fastHierarchyStub
		want    string
		wantErr bool
	}{
		{"fast", fastHierarchyStub{}, "companion", false},
		{"fallback", fastHierarchyStub{fallback: true}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{}
			svc.SetUIXMLProvider(&mockUIXMLProvider{err: dumpErr})
			svc.SetFastUIXMLProvider(tc.fast)
			result, err := svc.getUIXMLProvider().FetchSnapshot(context.Background(), "phone")
			if tc.wantErr {
				if !errors.Is(err, dumpErr) {
					t.Fatalf("lost fallback error: %v", err)
				}
				return
			}
			if err != nil || result.Source != tc.want {
				t.Fatalf("snapshot=%+v err=%v", result, err)
			}
		})
	}
}
