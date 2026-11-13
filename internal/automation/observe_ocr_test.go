package automation

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"farm/internal/ocr"
	"farm/internal/uipage"
	"farm/internal/uixml"
)

type forbiddenXMLProvider struct{ t *testing.T }

func (p forbiddenXMLProvider) FetchSnapshot(context.Context, string) (uixml.Snapshot, error) {
	p.t.Error("OCR-only observation must not call uiautomator")
	return uixml.Snapshot{}, errors.New("dump unavailable")
}

func TestObserveOCROnlySkipsDumpAndKeepsAllOCR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ocr_result":"设置","ocr_boxes":[{"text":"设置","confidence":0.99,"x":10,"y":20,"w":40,"h":20}]}`))
	}))
	defer server.Close()
	t.Setenv("INDRAOCR_URL", server.URL)
	t.Setenv("OCR_ENGINE", "indraocr")
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 100, 200)))
	svc := &Service{ocrService: ocr.NewService(nil, nil)}
	svc.SetUIXMLProvider(forbiddenXMLProvider{t})
	svc.SetUIPageProvider(&mockUIPageProvider{pageInfo: &uipage.PageInfo{
		PackageName: "com.android.settings", Display: &uipage.DisplayInfo{Width: 1000, Height: 2000},
	}})
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) { return nil, buf.Bytes(), nil })
	// A previous ref snapshot must not suppress matching text from OCR-only output.
	svc.screenCache.Store("phone", &Screen{Width: 100, Height: 200, Elements: []Element{{Label: "设置", Bounds: [4]int{0, 0, 100, 200}}}})
	diff := false
	out, err := svc.Observe(context.Background(), ObserveRequest{Serial: "phone", Formats: []string{"ocr"}, Diff: &diff})
	if err != nil || !strings.Contains(out, `ocr "设置" @300,300`) || strings.Contains(out, "[1]") {
		t.Fatalf("OCR-only output=%q error=%v", out, err)
	}
	cached, ok := svc.screenCache.Load("phone")
	if !ok || len(cached.(*Screen).Elements) != 0 || cached.(*Screen).RawXML != "" {
		t.Fatal("OCR-only observation retained stale ref/XML data")
	}
}

func TestObserveOCROnlyReportsUnavailableSources(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*Service)
		want      string
	}{
		{"service", func(*Service) {}, "OCR service is unavailable"},
		{"frame", func(s *Service) { s.ocrService = ocr.NewService(nil, nil) }, "no screen frame"},
		{"frame-error", func(s *Service) {
			s.ocrService = ocr.NewService(nil, nil)
			s.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
				return nil, nil, errors.New("stream disconnected")
			})
		}, "stream disconnected"},
		{"page", func(s *Service) { s.SetUIPageProvider(&mockUIPageProvider{err: errors.New("page unavailable")}) }, "page unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{}
			svc.SetUIXMLProvider(forbiddenXMLProvider{t})
			svc.SetUIPageProvider(&mockUIPageProvider{pageInfo: &uipage.PageInfo{Display: &uipage.DisplayInfo{Width: 100, Height: 200}}})
			tc.configure(svc)
			_, err := svc.Observe(context.Background(), ObserveRequest{Serial: "phone", Formats: []string{"ocr"}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %s", err, tc.want)
			}
			if _, ok := svc.screenCache.Load("phone"); ok {
				t.Fatal("failed observation cached as successful")
			}
		})
	}
}

func TestObserveRefStillReportsDumpFailure(t *testing.T) {
	svc := &Service{}
	svc.SetUIXMLProvider(&mockUIXMLProvider{err: errors.New("dump unavailable")})
	_, err := svc.Observe(context.Background(), ObserveRequest{Serial: "phone", Formats: []string{"ref", "ocr"}})
	if err == nil || !strings.Contains(err.Error(), "dump unavailable") {
		t.Fatalf("error=%v", err)
	}
}
