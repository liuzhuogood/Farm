package automation

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActScreenshotPreservesLatestFrameBytes(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		t.Run(format, func(t *testing.T) {
			svc, fixture := artifactTestService(t)
			prepareArtifactDeviceDirectory(t, svc, "shots/O'Reilly")
			img := image.NewRGBA(image.Rect(0, 0, 3, 4))
			var encoded bytes.Buffer
			ext := ".png"
			if format == "jpeg" {
				ext = ".jpg"
				if err := jpeg.Encode(&encoded, img, nil); err != nil {
					t.Fatal(err)
				}
			} else if err := png.Encode(&encoded, img); err != nil {
				t.Fatal(err)
			}
			fetches := 0
			svc.SetFrameFetcher(func(ctx context.Context, serial string) (image.Image, []byte, error) {
				fetches++
				if serial != "phone" {
					t.Fatal(serial)
				}
				return img, encoded.Bytes(), nil
			})
			saved, err := svc.runActArtifact(context.Background(), "phone", "screenshot", "shots/O'Reilly", 0)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(artifactDeviceDirectory(fixture, saved))
			if err != nil || !bytes.Equal(actual, encoded.Bytes()) || path.Ext(saved) != ext || fetches != 1 {
				t.Fatalf("saved=%s bytes=%d fetches=%d err=%v", saved, len(actual), fetches, err)
			}
			commands, _ := os.ReadFile(filepath.Join(fixture, "commands"))
			if strings.Contains(string(commands), "screencap") || strings.Contains(string(commands), "head -c") || !strings.Contains(string(commands), "shell -T umask 077; cat >") {
				t.Fatalf("screenshot was recaptured or read back: %s", commands)
			}
			logs := svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone", Type: LogTypeAction, Limit: 1}).Logs
			if len(logs) != 1 {
				t.Fatalf("logs=%v", logs)
			}
			f := logs[0].Fields
			if f["frame_source"] != "latest_frame_service" || f["frame_format"] != "image/"+format || f["frame_width"] != 3 || f["frame_height"] != 4 || f["frame_bytes"] != encoded.Len() {
				t.Fatalf("frame metadata=%v", f)
			}
			for _, key := range []string{"frame_fetch_ms", "frame_upload_ms", "frame_publish_ms"} {
				if _, ok := f[key]; !ok {
					t.Fatalf("missing %s: %v", key, f)
				}
			}
		})
	}
}

func TestActScreenshotFallbackAndValidation(t *testing.T) {
	svc, fixture := artifactTestService(t)
	prepareArtifactDeviceDirectory(t, svc, "shots")
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
		return nil, nil, errors.New("stream unavailable")
	})
	saved, err := svc.runActArtifact(context.Background(), "phone", "screenshot", "shots", 0)
	if err != nil || path.Ext(saved) != ".png" {
		t.Fatalf("saved=%q err=%v", saved, err)
	}
	expected, _ := os.ReadFile(filepath.Join(fixture, "screen.png"))
	actual, _ := os.ReadFile(artifactDeviceDirectory(fixture, saved))
	if !bytes.Equal(expected, actual) {
		t.Fatal("fallback PNG changed")
	}
	fields := svc.ListAutomationLogs(AutomationLogQuery{Serial: "phone", Type: LogTypeAction, Limit: 1}).Logs[0].Fields
	if fields["frame_source"] != "adb_fallback" || fields["frame_fallback_reason"] != "stream unavailable" {
		t.Fatalf("fields=%v", fields)
	}
	for _, raw := range [][]byte{[]byte("invalid"), make([]byte, actScreenshotLimit+1), expected[:len(expected)/2]} {
		svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
			return image.NewRGBA(image.Rect(0, 0, 2, 3)), raw, nil
		})
		before, _ := os.ReadFile(filepath.Join(fixture, "commands"))
		if _, err := svc.runActArtifact(context.Background(), "phone", "screenshot", "shots", 0); err == nil {
			t.Fatal("invalid frame accepted")
		}
		after, _ := os.ReadFile(filepath.Join(fixture, "commands"))
		if !bytes.Equal(before, after) {
			t.Fatal("invalid frame reached device")
		}
	}
}

func TestActScreenshotUploadFailuresCleanOwnedStage(t *testing.T) {
	for _, mode := range []string{"upload_fail", "upload_short", "upload_cancel"} {
		t.Run(mode, func(t *testing.T) {
			svc, fixture := artifactTestService(t)
			prepareArtifactDeviceDirectory(t, svc, "shots")
			raw, _ := os.ReadFile(filepath.Join(fixture, "screen.png"))
			svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
				return image.NewRGBA(image.Rect(0, 0, 2, 3)), raw, nil
			})
			t.Setenv("FARM_ARTIFACT_MODE", mode)
			ctx := context.Background()
			if mode == "upload_cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 300*time.Millisecond)
				defer cancel()
			}
			_, err := svc.runActArtifact(ctx, "phone", "screenshot", "shots", 0)
			if err == nil {
				t.Fatal("incomplete upload published")
			}
			if mode == "upload_cancel" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err=%v", err)
			}
			files, _ := os.ReadDir(artifactDeviceDirectory(fixture, "shots"))
			if len(files) != 0 {
				t.Fatalf("failed upload leaked %v", files)
			}
		})
	}
}

func TestActScreenshotCanceledFrameDoesNotFallBack(t *testing.T) {
	svc, fixture := artifactTestService(t)
	prepareArtifactDeviceDirectory(t, svc, "shots")
	ctx, cancel := context.WithCancel(context.Background())
	svc.SetFrameFetcher(func(context.Context, string) (image.Image, []byte, error) {
		cancel()
		return nil, nil, context.Canceled
	})
	before, _ := os.ReadFile(filepath.Join(fixture, "commands"))
	if _, err := svc.runActArtifact(ctx, "phone", "screenshot", "shots", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	after, _ := os.ReadFile(filepath.Join(fixture, "commands"))
	if !bytes.Equal(before, after) {
		t.Fatal("canceled fetch reached device")
	}
}
