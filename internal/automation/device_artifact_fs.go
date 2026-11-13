package automation

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"
)

const deviceArtifactRoot = "/sdcard/Farm/automation"

func deviceArtifactDirectory(raw string) (string, error) {
	if raw == "" || path.IsAbs(raw) || strings.ContainsAny(raw, "\x00\r\n") {
		return "", fmt.Errorf("directory must be relative to %s", deviceArtifactRoot)
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", fmt.Errorf("directory may not contain ..")
		}
	}
	return path.Clean(raw), nil
}
func deviceArtifactPath(relative string) string { return path.Join(deviceArtifactRoot, relative) }
func quoteDeviceArtifact(raw string) string {
	return "'" + strings.ReplaceAll(raw, "'", "'\"'\"'") + "'"
}
func deviceArtifactRelative(raw string) (string, error) {
	if !fs.ValidPath(raw) || raw == "." || strings.ContainsAny(raw, "\x00\r\n") {
		return "", fmt.Errorf("file must be a relative path inside the device directory")
	}
	return raw, nil
}

// Android's /sdcard alias may be a symlink; all descendants must be directories.
func deviceArtifactDirectoryScript(directory string, create bool) (string, error) {
	directory, err := deviceArtifactDirectory(directory)
	if err != nil {
		return "", err
	}
	parts := []string{"/sdcard/Farm", deviceArtifactRoot}
	if directory != "." {
		p := deviceArtifactRoot
		for _, part := range strings.Split(directory, "/") {
			p = path.Join(p, part)
			parts = append(parts, p)
		}
	}
	var out strings.Builder
	for _, p := range parts {
		q := quoteDeviceArtifact(p)
		out.WriteString("if [ -L " + q + " ]; then echo FARM_ARTIFACT_FORBIDDEN >&2; exit 43; fi; ")
		if create {
			out.WriteString("mkdir -p -- " + q + " || exit $?; ")
		}
		out.WriteString("if [ ! -d " + q + " ]; then echo FARM_ARTIFACT_NOT_FOUND >&2; exit 44; fi; ")
	}
	out.WriteString("cd " + quoteDeviceArtifact(deviceArtifactPath(directory)) + " || exit $?; ")
	return out.String(), nil
}
func deviceArtifactCommandError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "FARM_ARTIFACT_NOT_FOUND") {
		return fmt.Errorf("%w: device artifact path does not exist", fs.ErrNotExist)
	}
	if strings.Contains(err.Error(), "FARM_ARTIFACT_FORBIDDEN") {
		return fmt.Errorf("%w: device artifact symlinks or special files are forbidden", fs.ErrPermission)
	}
	return err
}
func (s *Service) readDeviceArtifact(ctx context.Context, serial, directory, relative string, max int64) ([]byte, error) {
	if max <= 0 || max > actRecordingLimit {
		return nil, fmt.Errorf("invalid device artifact read limit")
	}
	relative, err := deviceArtifactRelative(relative)
	if err != nil {
		return nil, err
	}
	prefix, err := deviceArtifactDirectoryScript(directory, false)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(relative, "/")
	for i := range parts {
		q := quoteDeviceArtifact(strings.Join(parts[:i+1], "/"))
		prefix += "if [ -L " + q + " ]; then echo FARM_ARTIFACT_FORBIDDEN >&2; exit 43; fi; "
		if i < len(parts)-1 {
			prefix += "if [ ! -d " + q + " ]; then echo FARM_ARTIFACT_NOT_FOUND >&2; exit 44; fi; "
		}
	}
	q := quoteDeviceArtifact(relative)
	prefix += "if [ ! -e " + q + " ]; then echo FARM_ARTIFACT_NOT_FOUND >&2; exit 44; fi; if [ ! -f " + q + " ]; then echo FARM_ARTIFACT_FORBIDDEN >&2; exit 43; fi; "
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var output bytes.Buffer
	// shell v2 without a PTY keeps binary stdout intact while propagating the
	// remote exit status and separate stderr. exec-out would hide failures and
	// could turn a missing-file sentinel into a successful binary download.
	err = s.writeArtifactADB(readCtx, serial, &output, max, "shell", "-T", prefix+fmt.Sprintf("head -c %d -- %s", max+1, q))
	if err != nil {
		return nil, deviceArtifactCommandError(err)
	}
	return output.Bytes(), nil
}
