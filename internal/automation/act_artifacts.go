package automation

import (
	"bytes"
	"context"
	"farm/internal/dependencies"
	"fmt"
	"github.com/Eyevinn/mp4ff/mp4"
	"image/png"
	"io"
	"path"
	"strings"
	"time"
)

const (
	actScreenshotLimit int64 = 20 << 20
	actRecordingLimit  int64 = 32 << 20
)

type actLimitedWriter struct {
	w         io.Writer
	remaining int64
}

func (w *actLimitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("artifact exceeds size limit")
	}
	n, e := w.w.Write(p)
	w.remaining -= int64(n)
	return n, e
}

// Artifacts remain on Android; only bounded validation buffers reach the host.
func (s *Service) runActArtifact(ctx context.Context, serial, action, directory string, seconds int64) (saved string, err error) {
	started := time.Now()
	defer func() {
		kind := LogTypeAction
		fields := AutomationLogFields("action", action, "directory", directory, "path", saved)
		if err != nil {
			kind = LogTypeError
			fields["error"] = err.Error()
		}
		s.recordAutomationLogContext(ctx, AutomationLog{Type: kind, Serial: serial, Message: "自动化设备文件操作完成", ElapsedMS: time.Since(started).Milliseconds(), Fields: fields})
	}()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	directory, err = deviceArtifactDirectory(directory)
	if err != nil {
		return "", err
	}
	if action != "mkdir" && action != "screenshot" && action != "record" && action != "rm" {
		return "", fmt.Errorf("unsupported artifact action %q", action)
	}
	if action == "record" && (seconds < 1 || seconds > 30) {
		return "", fmt.Errorf("record seconds must be 1..30")
	}
	if s.adbPath == "" {
		return "", fmt.Errorf("ADB executable is unavailable")
	}
	if action == "rm" {
		if directory == "." {
			return "", fmt.Errorf("rm may not remove the device artifact root")
		}
		prefix, buildErr := deviceArtifactDirectoryScript(path.Dir(directory), false)
		if buildErr != nil {
			return "", buildErr
		}
		q := quoteDeviceArtifact(path.Base(directory))
		script := prefix + "if [ -L " + q + " ]; then echo FARM_ARTIFACT_FORBIDDEN >&2; exit 43; fi; if [ ! -e " + q + " ]; then echo FARM_ARTIFACT_NOT_FOUND >&2; exit 44; fi; if [ ! -f " + q + " ] && [ ! -d " + q + " ]; then echo FARM_ARTIFACT_FORBIDDEN >&2; exit 43; fi; rm -rf -- " + q + " && test ! -e " + q + " && test ! -L " + q
		if err = s.writeArtifactADB(ctx, serial, io.Discard, 8192, "shell", script); err != nil {
			return "", deviceArtifactCommandError(err)
		}
		s.recordTaskRemoval(ctx, directory)
		return directory, ctx.Err()
	}
	prefix, err := deviceArtifactDirectoryScript(directory, action == "mkdir")
	if err != nil {
		return "", err
	}
	if action == "mkdir" {
		err = s.writeArtifactADB(ctx, serial, io.Discard, 8192, "shell", prefix+"test -d .")
		if err != nil {
			return "", deviceArtifactCommandError(err)
		}
		s.recordTaskDirectory(ctx, directory)
		return directory, ctx.Err()
	}
	id := newActionID()
	private := path.Join(directory, ".farm-"+id+".partial")
	privateName := path.Base(private)
	// Nonrecursive mkdir proves staging ownership before installing cleanup.
	if err = s.writeArtifactADB(ctx, serial, io.Discard, 8192, "shell", prefix+"mkdir -- "+quoteDeviceArtifact(privateName)); err != nil {
		return "", fmt.Errorf("创建设备暂存目录: %w", deviceArtifactCommandError(err))
	}
	ext := ".png"
	if action == "record" {
		ext = ".mp4"
	}
	partial := path.Join(private, "capture"+ext)
	final := path.Join(directory, "farm-"+action+"-"+id+ext)
	pidfile := path.Join(private, "record.pid")
	attempt := path.Join(private, "publication.attempt")
	keepFinal := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		pid := quoteDeviceArtifact(deviceArtifactPath(pidfile))
		cleanup := "cleanup_rc=0; if [ -f " + pid + " ] && [ ! -L " + pid + " ]; then read p < " + pid + "; case $p in ''|*[!0-9]*) ;; *) kill -INT \"$p\" 2>/dev/null ;; esac; fi; "
		if !keepFinal {
			// A no-clobber collision leaves our partial in place. Only a
			// publication attempt whose source disappeared can own the final.
			cleanup += "if [ -f " + quoteDeviceArtifact(deviceArtifactPath(attempt)) + " ] && [ ! -e " + quoteDeviceArtifact(deviceArtifactPath(partial)) + " ]; then rm -f -- " + quoteDeviceArtifact(deviceArtifactPath(final)) + " || cleanup_rc=$?; fi; "
		}
		cleanup += "rm -f -- " + pid + " " + quoteDeviceArtifact(deviceArtifactPath(partial)) + " " + quoteDeviceArtifact(deviceArtifactPath(attempt)) + " || cleanup_rc=$?; rmdir -- " + quoteDeviceArtifact(deviceArtifactPath(private)) + " || cleanup_rc=$?; exit \"$cleanup_rc\""
		if cleanupErr := s.writeArtifactADB(cleanupCtx, serial, io.Discard, 8192, "shell", cleanup); cleanupErr != nil {
			s.recordAutomationLogContext(ctx, AutomationLog{Type: LogTypeError, Serial: serial, Message: "设备产物临时文件清理失败", Fields: AutomationLogFields("directory", private, "error", cleanupErr.Error())})
		}
	}()
	if action == "screenshot" {
		err = s.writeArtifactADB(ctx, serial, io.Discard, 8192, "shell", "screencap -p "+quoteDeviceArtifact(deviceArtifactPath(partial)))
	} else {
		script := fmt.Sprintf("screenrecord --bit-rate 2000000 --time-limit %d %s & p=$!; echo $p > %s; wait $p; rc=$?; rm -f -- %s; exit $rc", seconds, quoteDeviceArtifact(deviceArtifactPath(partial)), quoteDeviceArtifact(deviceArtifactPath(pidfile)), quoteDeviceArtifact(deviceArtifactPath(pidfile)))
		err = s.writeArtifactADB(ctx, serial, io.Discard, 8192, "shell", script)
	}
	if err != nil {
		return "", fmt.Errorf("设备%s采集: %w", action, err)
	}
	limit := actScreenshotLimit
	if action == "record" {
		limit = actRecordingLimit
	}
	data, err := s.readDeviceArtifact(ctx, serial, directory, path.Join(privateName, "capture"+ext), limit)
	if err != nil {
		return "", fmt.Errorf("读回设备产物以校验: %w", err)
	}
	if err = validateActArtifact(action, data); err != nil {
		return "", fmt.Errorf("校验设备产物内容: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return "", fmt.Errorf("发布设备产物前已取消: %w", err)
	}
	// A skipped no-clobber move leaves its source and must not report success.
	pub := prefix + "test -f " + quoteDeviceArtifact(path.Join(privateName, "capture"+ext)) + " && test ! -L " + quoteDeviceArtifact(path.Join(privateName, "capture"+ext)) + " && printf '1' > " + quoteDeviceArtifact(path.Join(privateName, "publication.attempt")) + " && mv -nT -- " + quoteDeviceArtifact(path.Join(privateName, "capture"+ext)) + " " + quoteDeviceArtifact(path.Base(final)) + " && test ! -e " + quoteDeviceArtifact(path.Join(privateName, "capture"+ext)) + " && test -f " + quoteDeviceArtifact(path.Base(final)) + " && test ! -L " + quoteDeviceArtifact(path.Base(final))
	if err = s.writeArtifactADB(ctx, serial, io.Discard, 8192, "shell", pub); err != nil {
		return "", fmt.Errorf("发布设备产物: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return "", fmt.Errorf("发布设备产物后已取消: %w", err)
	}
	keepFinal = true
	kind := "image"
	if action == "record" {
		kind = "video"
	}
	s.recordTaskArtifact(ctx, serial, final, kind, int64(len(data)))
	return final, nil
}

func validateActArtifact(action string, data []byte) error {
	if action == "screenshot" {
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 16384 || cfg.Height > 16384 || int64(cfg.Width)*int64(cfg.Height) > 64<<20 {
			return fmt.Errorf("ADB did not return a valid bounded PNG: %v", err)
		}
		if _, err = png.Decode(bytes.NewReader(data)); err != nil {
			return fmt.Errorf("decode screenshot: %w", err)
		}
		return nil
	}
	movie, err := mp4.DecodeFile(bytes.NewReader(data), mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		return fmt.Errorf("decode recording: %w", err)
	}
	if movie.Ftyp == nil || movie.Moov == nil || movie.Mdat == nil || movie.Mdat.Size() <= 8 || len(movie.Moov.Traks) == 0 {
		return fmt.Errorf("ADB recording has no finalized MP4 video")
	}
	for _, track := range movie.Moov.Traks {
		if actVideoHasDuration(track) {
			return nil
		}
	}
	return fmt.Errorf("ADB recording has no video samples with positive duration; confirm the device screen is awake")
}
func actVideoHasDuration(track *mp4.TrakBox) bool {
	if track.Mdia == nil || track.Mdia.Hdlr == nil || track.Mdia.Hdlr.HandlerType != "vide" || track.Mdia.Mdhd == nil || track.Mdia.Mdhd.Timescale == 0 || track.Mdia.Minf == nil || track.Mdia.Minf.Stbl == nil {
		return false
	}
	table := track.Mdia.Minf.Stbl
	if table.Stsz == nil || table.Stsz.SampleNumber == 0 || table.Stts == nil || len(table.Stts.SampleCount) != len(table.Stts.SampleTimeDelta) {
		return false
	}
	for i, count := range table.Stts.SampleCount {
		if count > 0 && table.Stts.SampleTimeDelta[i] > 0 {
			return true
		}
	}
	return false
}
func (s *Service) writeArtifactADB(ctx context.Context, serial string, output io.Writer, limit int64, args ...string) error {
	cmd := dependencies.CommandContext(ctx, s.adbPath, append([]string{"-s", serial}, args...)...)
	cmd.Stdout = &actLimitedWriter{w: output, remaining: limit}
	var stderr strings.Builder
	cmd.Stderr = &actLimitedWriter{w: &stderr, remaining: 8192}
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ADB %s failed: %w: %s", strings.Join(args[:1], " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
