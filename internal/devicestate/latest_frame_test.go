package devicestate

import (
	"bytes"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"

	"farm/internal/visionframes"
)

func TestLatestFrameImageMethods(t *testing.T) {
	frames := visionframes.NewStore()
	want := createTestJPEG(4, 6, color.White)
	if _, err := frames.Put("phone", want); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(NewService("", nil, nil, nil, nil, frames), nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, path := range []string{
			"/api/device-state/phone/screenshot",
			"/api/devices/phone/latest-frame",
			"/api/v1/vision/latest-frame?serial=phone",
		} {
			t.Run(method+" "+path, func(t *testing.T) {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
				if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/jpeg" {
					t.Fatalf("unexpected response: code=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
				}
				if !bytes.Equal(w.Body.Bytes(), want) {
					t.Fatal("response must contain original image bytes")
				}
				img, err := jpeg.Decode(w.Body)
				if err != nil || img.Bounds().Dx() != 4 || img.Bounds().Dy() != 6 {
					t.Fatalf("response is not a decodable image: image=%v err=%v", img, err)
				}
				if got := w.Header().Get("Cache-Control"); got != "no-cache, no-store, must-revalidate" {
					t.Fatalf("latest frame must not be cached: %s", got)
				}
			})
		}
	}
}

func TestLatestFrameMethodsErrors(t *testing.T) {
	h := NewHandler(NewService("", nil, nil, nil, nil, nil), nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, tc := range []struct {
			path string
			code int
		}{
			{"/api/v1/vision/latest-frame", http.StatusBadRequest},
			{"/api/v1/vision/latest-frame?serial=phone", http.StatusBadGateway},
		} {
			t.Run(method+" "+tc.path, func(t *testing.T) {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest(method, tc.path, nil))
				if w.Code != tc.code || w.Body.Len() == 0 {
					t.Fatalf("expected explicit failure %d, got %d: %s", tc.code, w.Code, w.Body.String())
				}
				if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
					t.Fatalf("failure must not be disguised as an image: %s", got)
				}
			})
		}
	}
}
