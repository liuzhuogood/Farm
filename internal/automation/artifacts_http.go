package automation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"farm/internal/dependencies"
	"github.com/Eyevinn/mp4ff/mp4"
)

const (
	artifactDirectoryLimit        = 1000
	artifactHTTPConcurrency       = 4
	artifactDownloadLimit   int64 = 32 << 20
	artifactHTTPTimeout           = 30 * time.Second
)

type artifactLink struct {
	Name, URL string
	Size      int64
}
type artifactDirectoryPage struct {
	Serial, Directory, Root string
	Files                   []artifactLink
	Truncated               bool
}

var artifactDirectoryTemplate = template.Must(template.New("artifacts").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Farm 设备产物下载</title><style>body{max-width:1000px;margin:24px auto;padding:0 16px;font:14px/1.6 system-ui,sans-serif;color:#202124}li{margin:14px 0}a{color:#1677ff;overflow-wrap:anywhere}small{color:#666}code{overflow-wrap:anywhere}</style></head>
<body><h1>设备产物下载</h1><p>设备：<code>{{.Serial}}</code><br>设备根目录：<code>{{.Root}}</code><br>相对目录：<code>{{.Directory}}</code></p>{{if .Truncated}}<p role="status">仅展示前 1000 个文件，列表未完整展示；请指定更小的目录。</p>{{end}}
<ul>{{range .Files}}<li><strong>{{.Name}}</strong> <small>{{.Size}} 字节</small><br><a href="{{.URL}}" download>{{.URL}}</a></li>{{else}}<li>没有可下载的文件。</li>{{end}}</ul></body></html>`))

// Forwarded headers are intentionally ignored without a trusted proxy setting.
func artifactDirectoryURL(r *http.Request, endpoint string, params url.Values) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: r.Host, Path: endpoint, RawQuery: params.Encode()}).String()
}

type artifactRequestError struct {
	status int
	err    error
}

func (e *artifactRequestError) Error() string { return e.err.Error() }
func (e *artifactRequestError) Unwrap() error { return e.err }

func artifactHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadGateway
	var requestErr *artifactRequestError
	switch {
	case errors.As(err, &requestErr):
		status = requestErr.status
	case errors.Is(err, fs.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(err, fs.ErrPermission):
		status = http.StatusForbidden
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	log.Printf("[Automation] 设备产物访问失败 path=%s serial=%q directory=%q: %v", r.URL.Path, r.URL.Query().Get("serial"), r.URL.Query().Get("directory"), err)
	http.Error(w, err.Error(), status)
}

func publicDeviceArtifactName(name string) error {
	if _, err := deviceArtifactRelative(name); err != nil {
		return err
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasSuffix(part, ".partial") {
			return fs.ErrNotExist
		}
	}
	return nil
}

// Validate every requested path before consulting the online registry or ADB.
func artifactRequest(r *http.Request, download bool) (serial, directory, file string, err error) {
	bad := func(err error) (string, string, string, error) {
		return "", "", "", &artifactRequestError{http.StatusBadRequest, err}
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return bad(err)
	}
	for _, key := range []string{"serial", "directory"} {
		if len(q[key]) != 1 || q[key][0] == "" {
			return bad(fmt.Errorf("%s must occur exactly once and be nonempty", key))
		}
	}
	serial = q.Get("serial")
	directory, err = deviceArtifactDirectory(q.Get("directory"))
	if err != nil {
		return bad(err)
	}
	if directory != "." {
		if err := publicDeviceArtifactName(directory); err != nil {
			return bad(err)
		}
	}
	if values, present := q["preview"]; present && (!download || len(values) != 1 || values[0] != "1") {
		return bad(fmt.Errorf("preview only supports the value 1 on file downloads"))
	}
	if download {
		if len(q["file"]) != 1 {
			return bad(fmt.Errorf("file must occur exactly once"))
		}
		file = q.Get("file")
		if err := publicDeviceArtifactName(file); err != nil {
			return bad(err)
		}
	}
	return serial, directory, file, nil
}

func (s *Service) acquireArtifactHTTP() bool {
	s.artifactHTTPOnce.Do(func() { s.artifactHTTPSlots = make(chan struct{}, artifactHTTPConcurrency) })
	select {
	case s.artifactHTTPSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

// deviceArtifactListingOutput retains at most 1001 NUL-delimited name/size
// records. Once truncation is known it cancels the remote producer immediately.
type deviceArtifactListingOutput struct {
	data               bytes.Buffer
	fields, fieldBytes int
	truncated          bool
	err                error
	cancel             context.CancelFunc
}

func (o *deviceArtifactListingOutput) Write(p []byte) (int, error) {
	if o.truncated || o.err != nil {
		return len(p), nil
	}
	for i, b := range p {
		o.fieldBytes++
		if o.fieldBytes > 8192 || o.data.Len()+i+1 > 8<<20 {
			o.err = fmt.Errorf("device artifact listing exceeds bounded record size")
			o.cancel()
			return len(p), nil
		}
		if b == 0 {
			o.fields++
			o.fieldBytes = 0
			if o.fields == 2*(artifactDirectoryLimit+1) {
				o.data.Write(p[:i+1])
				o.truncated = true
				o.cancel()
				return len(p), nil
			}
		}
	}
	o.data.Write(p)
	return len(p), nil
}

func (s *Service) listDeviceArtifacts(ctx context.Context, serial, directory string) ([]artifactLink, bool, error) {
	prefix, err := deviceArtifactDirectoryScript(directory, false)
	if err != nil {
		return nil, false, err
	}
	if s.adbPath == "" {
		return nil, false, fmt.Errorf("ADB executable is unavailable")
	}
	commandCtx, cancel := context.WithTimeout(ctx, artifactHTTPTimeout)
	defer cancel()
	// Default find does not follow child symlinks; partial temporary entries
	// are pruned before traversal, and only regular files are emitted.
	script := prefix + `find . -mindepth 1 -name '*.partial' -prune -o -type f -printf '%P\0%s\0'`
	cmd := dependencies.CommandContext(commandCtx, s.adbPath, "-s", serial, "shell", "-T", script)
	out := &deviceArtifactListingOutput{cancel: cancel}
	var stderr strings.Builder
	cmd.Stdout = out
	cmd.Stderr = &actLimitedWriter{w: &stderr, remaining: 8192}
	cmd.WaitDelay = time.Second
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if out.err != nil {
		return nil, false, out.err
	}
	if !out.truncated {
		if commandCtx.Err() != nil {
			return nil, false, commandCtx.Err()
		}
		if runErr != nil {
			return nil, false, deviceArtifactCommandError(fmt.Errorf("list device artifacts: %w: %s", runErr, strings.TrimSpace(stderr.String())))
		}
	}
	data := out.data.Bytes()
	if len(data) > 0 && data[len(data)-1] != 0 {
		return nil, false, fmt.Errorf("device artifact listing returned an incomplete record")
	}
	parts := bytes.Split(data, []byte{0})
	if len(parts)%2 != 1 {
		return nil, false, fmt.Errorf("device artifact listing returned invalid name/size records")
	}
	files := make([]artifactLink, 0, min(len(parts)/2, artifactDirectoryLimit))
	for i := 0; i+1 < len(parts) && len(files) < artifactDirectoryLimit; i += 2 {
		name := string(parts[i])
		if err := publicDeviceArtifactName(name); err != nil {
			return nil, false, fmt.Errorf("invalid device artifact name: %w", err)
		}
		size, err := strconv.ParseInt(string(parts[i+1]), 10, 64)
		if err != nil || size < 0 {
			return nil, false, fmt.Errorf("device artifact listing returned invalid file size")
		}
		files = append(files, artifactLink{Name: name, Size: size})
	}
	return files, out.truncated, nil
}

func (s *Service) handleArtifactsDirectory(w http.ResponseWriter, r *http.Request) {
	serial, directory, _, err := artifactRequest(r, false)
	if err != nil {
		artifactHTTPError(w, r, err)
		return
	}
	if _, err := s.resolveActDevice(serial); err != nil {
		artifactHTTPError(w, r, &artifactRequestError{http.StatusNotFound, err})
		return
	}
	if !s.acquireArtifactHTTP() {
		artifactHTTPError(w, r, &artifactRequestError{http.StatusTooManyRequests, fmt.Errorf("artifact download capacity is full; retry later")})
		return
	}
	defer func() { <-s.artifactHTTPSlots }()
	files, truncated, err := s.listDeviceArtifacts(r.Context(), serial, directory)
	if err != nil {
		artifactHTTPError(w, r, err)
		return
	}
	page := artifactDirectoryPage{Serial: serial, Directory: directory, Root: deviceArtifactRoot, Files: files, Truncated: truncated}
	for i := range page.Files {
		page.Files[i].URL = artifactDirectoryURL(r, "/api/automation/artifacts/download", url.Values{"serial": {serial}, "directory": {directory}, "file": {page.Files[i].Name}})
	}
	sort.Slice(page.Files, func(i, j int) bool { return page.Files[i].Name < page.Files[j].Name })
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := artifactDirectoryTemplate.Execute(w, page); err != nil {
		log.Printf("[Automation] 设备产物目录响应失败 serial=%q directory=%q: %v", serial, directory, err)
	}
}

func (s *Service) handleArtifactDownload(w http.ResponseWriter, r *http.Request) {
	serial, directory, name, err := artifactRequest(r, true)
	if err != nil {
		artifactHTTPError(w, r, err)
		return
	}
	if _, err := s.resolveActDevice(serial); err != nil {
		artifactHTTPError(w, r, &artifactRequestError{http.StatusNotFound, err})
		return
	}
	if !s.acquireArtifactHTTP() {
		artifactHTTPError(w, r, &artifactRequestError{http.StatusTooManyRequests, fmt.Errorf("artifact download capacity is full; retry later")})
		return
	}
	defer func() { <-s.artifactHTTPSlots }()
	ctx, cancel := context.WithTimeout(r.Context(), artifactHTTPTimeout)
	defer cancel()
	data, err := s.readDeviceArtifact(ctx, serial, directory, name, artifactDownloadLimit)
	if err != nil {
		artifactHTTPError(w, r, err)
		return
	}
	if err := ctx.Err(); err != nil {
		artifactHTTPError(w, r, err)
		return
	}
	disposition, contentType := "attachment", "application/octet-stream"
	if r.URL.Query().Get("preview") == "1" {
		if detected := artifactPreviewType(data); detected != "" {
			disposition, contentType = "inline", detected
		}
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": path.Base(name)}))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

// Only inert media types are previewed. HTML, SVG and unknown files retain the
// normal attachment contract even if the caller requests preview=1.
func artifactPreviewType(data []byte) string {
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return contentType
	}
	// Valid MP4 brands also include cmfc/iso6, which net/http does not label
	// video/mp4. Require the actual ftyp box and validate the video metadata.
	if len(data) >= 8 && string(data[4:8]) == "ftyp" {
		movie, err := mp4.DecodeFile(bytes.NewReader(data), mp4.WithDecodeMode(mp4.DecModeLazyMdat))
		if err == nil && movie.Ftyp != nil && movie.Moov != nil {
			for _, track := range movie.Moov.Traks {
				if actVideoHasDuration(track) {
					return "video/mp4"
				}
			}
		}
	}
	return ""
}
