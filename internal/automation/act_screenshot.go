package automation

import (
	"bytes"
	"context"
	"farm/internal/dependencies"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"time"
)

type actScreenshotFrame struct {
	data   []byte
	ext    string
	fields map[string]interface{}
}

// Use the same injected latest-frame service and fallback ordering as
// CaptureFrame. This service can itself fall back to ADB; do not label every
// successful fetch as a cache hit. Observe retains its existing capture path.
func (s *Service) captureActScreenshot(ctx context.Context, serial string) (frame actScreenshotFrame, err error) {
	started := time.Now()
	frame.fields = AutomationLogFields("frame_source", "latest_frame_service")
	defer func() {
		if _, recorded := frame.fields["frame_fetch_ms"]; !recorded {
			frame.fields["frame_fetch_ms"] = time.Since(started).Milliseconds()
		}
	}()
	if serial == "" {
		return frame, fmt.Errorf("device serial is required")
	}
	if err = ctx.Err(); err != nil {
		return frame, err
	}
	if s.frameFetcher != nil {
		img, raw, fetchErr := s.frameFetcher(ctx, serial)
		if fetchErr == nil && img != nil && len(raw) > 0 {
			frame.data = raw
		} else if fetchErr != nil {
			frame.fields["frame_fallback_reason"] = fetchErr.Error()
		}
	}
	if err = ctx.Err(); err != nil {
		return frame, err
	}
	if len(frame.data) == 0 {
		frame.fields["frame_source"] = "adb_fallback"
		var raw bytes.Buffer
		// Unlike exec-out shell scripts, the native screencap executable produces
		// binary stdout directly. Bound the fallback before retaining its output.
		err = s.writeArtifactADB(ctx, serial, &raw, actScreenshotLimit, "exec-out", "screencap", "-p")
		if err != nil {
			return frame, fmt.Errorf("capture fallback: %w", err)
		}
		frame.data = raw.Bytes()
	}
	frame.fields["frame_fetch_ms"] = time.Since(started).Milliseconds()
	validationStarted := time.Now()
	var format string
	var width, height int
	format, width, height, err = validateActScreenshot(frame.data)
	frame.fields["frame_validate_ms"] = time.Since(validationStarted).Milliseconds()
	if err != nil {
		return frame, err
	}
	frame.ext = ".png"
	if format == "jpeg" {
		frame.ext = ".jpg"
	}
	frame.fields["frame_format"] = "image/" + format
	frame.fields["frame_width"] = width
	frame.fields["frame_height"] = height
	frame.fields["frame_bytes"] = len(frame.data)
	return frame, ctx.Err()
}

func validateActScreenshot(data []byte) (format string, width, height int, err error) {
	if len(data) == 0 || int64(len(data)) > actScreenshotLimit {
		return "", 0, 0, fmt.Errorf("screenshot must contain 1..%d bytes", actScreenshotLimit)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 16384 || cfg.Height > 16384 || int64(cfg.Width)*int64(cfg.Height) > 64<<20 {
		return "", 0, 0, fmt.Errorf("latest frame is not a valid bounded JPEG or PNG: %v", err)
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return "", 0, 0, fmt.Errorf("decode screenshot: %w", err)
	}
	return format, cfg.Width, cfg.Height, nil
}

// The frame is validated in host memory and streamed once into an exclusively
// owned staging directory. Shell v2 preserves errors and avoids PTY byte
// rewriting. A complete cat and exact file size are required before publishing.
func (s *Service) uploadActScreenshot(ctx context.Context, serial, devicePath string, data []byte) error {
	if len(data) == 0 || int64(len(data)) > actScreenshotLimit {
		return fmt.Errorf("screenshot exceeds upload limit")
	}
	q := quoteDeviceArtifact(devicePath)
	script := "umask 077; cat > " + q + " && test -f " + q + " && test ! -L " + q + " && test \"$(wc -c < " + q + ")\" -eq " + fmt.Sprint(len(data))
	uploadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := dependencies.CommandContext(uploadCtx, s.adbPath, "-s", serial, "shell", "-T", script)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = &actLimitedWriter{w: io.Discard, remaining: 8192}
	var stderr strings.Builder
	cmd.Stderr = &actLimitedWriter{w: &stderr, remaining: 8192}
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if uploadCtx.Err() != nil {
			return uploadCtx.Err()
		}
		return fmt.Errorf("upload latest frame: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return uploadCtx.Err()
}
