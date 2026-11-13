package automation

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"image"
	"image/png"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"farm/internal/device"
)

func artifactTestMux(svc *Service) *http.ServeMux {
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	return mux
}

func artifactTestRequest(mux *http.ServeMux, endpoint string, query url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, endpoint+"?"+query.Encode(), nil))
	return w
}

// The fake ADB executes device shell checks against a virtual Android root.
// Local files below this fixture stand in for device files, never HTTP targets.
func artifactHTTPFixture(t *testing.T, directory string) (*Service, string) {
	t.Helper()
	svc, fixture := artifactTestService(t)
	// Android Toybox find supports this confirmed -printf contract. The host
	// fixture supplies the same byte protocol because macOS find lacks it.
	findShim := `#!/usr/bin/env python3
import os,sys,stat
if sys.argv[1:] != ['.', '-mindepth', '1', '-name', '*.partial', '-prune', '-o', '-type', 'f', '-printf', r'%P\0%s\0']:
    print('unexpected find contract',file=sys.stderr);sys.exit(8)
def fail(error): raise error
try:
    for root,dirs,files in os.walk('.',followlinks=False,onerror=fail):
        dirs[:]=[name for name in dirs if not name.endswith('.partial') and not os.path.islink(os.path.join(root,name))]
        for name in files:
            if name.endswith('.partial'): continue
            filename=os.path.join(root,name); info=os.lstat(filename)
            if stat.S_ISREG(info.st_mode):
                sys.stdout.buffer.write(os.fsencode(os.path.relpath(filename,'.'))+b'\0'+str(info.st_size).encode()+b'\0')
except OSError as error:
    print(str(error),file=sys.stderr);sys.exit(9)
`
	if err := os.WriteFile(filepath.Join(fixture, "bin", "find"), []byte(findShim), 0700); err != nil {
		t.Fatal(err)
	}
	svc.SetDeviceProvider(func() []device.Device {
		return []device.Device{{Serial: "phone", State: "device"}, {Serial: "offline", State: "offline"}}
	})
	root := filepath.Join(fixture, "device", "automation", filepath.FromSlash(directory))
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	return svc, root
}

func TestArtifactsDeviceDirectoryLinksAndDownload(t *testing.T) {
	directory := "产物 <script> & files"
	svc, fixtureRoot := artifactHTTPFixture(t, directory)
	if err := os.MkdirAll(filepath.Join(fixtureRoot, "子目录"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"截图 <img>& #?.png": "image bytes", "子目录/screen ' 录屏.mp4": "video bytes", ".notes": "normal hidden file"}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(fixtureRoot, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, ".farm-shot-unfinished.partial"), []byte("incomplete"), 0600); err != nil {
		t.Fatal(err)
	}
	mux := artifactTestMux(svc)
	req := httptest.NewRequest(http.MethodGet, "http://farm.test:1989/api/automation/artifacts?"+url.Values{"serial": {"phone"}, "directory": {directory}}.Encode(), nil)
	req.Header.Set("X-Forwarded-Host", "untrusted.test")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("unexpected directory response: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "<script>") || strings.Contains(body, "<img>") || strings.Contains(body, ".partial") || strings.Contains(body, "untrusted.test") || !strings.Contains(body, deviceArtifactRoot) || !strings.Contains(body, "phone") {
		t.Fatalf("unsafe or incorrect device directory: %s", body)
	}
	links := regexp.MustCompile(`<a href="([^"]+)" download>`).FindAllStringSubmatch(body, -1)
	if len(links) != len(files) {
		t.Fatalf("want %d files, got %d: %s", len(files), len(links), body)
	}
	seen := map[string]bool{}
	for _, match := range links {
		link, err := url.Parse(html.UnescapeString(match[1]))
		if err != nil {
			t.Fatal(err)
		}
		if link.Scheme != "http" || link.Host != "farm.test:1989" || link.Path != "/api/automation/artifacts/download" || link.Query().Get("directory") != directory || link.Query().Get("serial") != "phone" {
			t.Fatalf("wrong absolute device URL: %s", link)
		}
		name := link.Query().Get("file")
		if _, ok := files[name]; !ok || seen[name] {
			t.Fatalf("wrong or duplicate filename: %q", name)
		}
		seen[name] = true
		download := httptest.NewRecorder()
		mux.ServeHTTP(download, httptest.NewRequest(http.MethodGet, link.String(), nil))
		if download.Code != http.StatusOK || download.Body.String() != files[name] {
			t.Fatalf("device download did not match: %d %s", download.Code, download.Body.String())
		}
		mediaType, params, err := mime.ParseMediaType(download.Header().Get("Content-Disposition"))
		if err != nil || mediaType != "attachment" || params["filename"] != filepath.Base(name) {
			t.Fatalf("invalid attachment disposition: %s (%v)", download.Header().Get("Content-Disposition"), err)
		}
	}
}

func TestArtifactsDeviceRejectInvalidAndSpecialFiles(t *testing.T) {
	svc, root := artifactHTTPFixture(t, "exports")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unfinished.partial"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "linked-file")); err != nil {
		t.Fatal(err)
	}
	mux := artifactTestMux(svc)
	for _, file := range []string{"", ".", "..", "../secret", "/etc/passwd", "subdir/../../secret", "linked/secret", "linked-file", "unfinished.partial", "subdir"} {
		t.Run(file, func(t *testing.T) {
			w := artifactTestRequest(mux, "/api/automation/artifacts/download", url.Values{"serial": {"phone"}, "directory": {"exports"}, "file": {file}})
			if w.Code < 400 || strings.Contains(w.Body.String(), "secret payload") || w.Header().Get("Content-Disposition") != "" {
				t.Fatalf("invalid device file accepted: %q: %d %s", file, w.Code, w.Body.String())
			}
		})
	}
	w := artifactTestRequest(mux, "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"exports"}})
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "linked") || strings.Contains(w.Body.String(), "unfinished") {
		t.Fatalf("symlink or partial file listed: %d %s", w.Code, w.Body.String())
	}
	// A requested directory that traverses a link is rejected too.
	w = artifactTestRequest(mux, "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"exports/linked"}})
	if w.Code != http.StatusForbidden {
		t.Fatalf("linked directory exposed device root: %d %s", w.Code, w.Body.String())
	}
}

func TestArtifactsDevicePipeFailsWithoutBlocking(t *testing.T) {
	svc, root := artifactHTTPFixture(t, "pipes")
	if err := exec.Command("mkfifo", filepath.Join(root, "pipe")).Run(); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/automation/artifacts/download?serial=phone&directory=pipes&file=pipe", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	artifactTestMux(svc).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("device pipe should fail before opening: %d %s", w.Code, w.Body.String())
	}
}

func TestArtifactsValidateSerialAndRelativeDirectoryBeforeADB(t *testing.T) {
	svc, _ := newActTestService()
	calls := 0
	svc.SetDeviceProvider(func() []device.Device { calls++; return nil })
	for _, q := range []url.Values{
		{"directory": {"exports"}}, {"serial": {"phone"}}, {"serial": {"phone", "other"}, "directory": {"exports"}},
		{"serial": {"phone"}, "directory": {"/sdcard/export"}}, {"serial": {"phone"}, "directory": {"../export"}}, {"serial": {"phone"}, "directory": {"a/../export"}},
	} {
		w := artifactTestRequest(artifactTestMux(svc), "/api/automation/artifacts", q)
		if w.Code != http.StatusBadRequest || calls != 0 {
			t.Fatalf("invalid path consulted device: %d %s calls=%d", w.Code, w.Body.String(), calls)
		}
	}
	svc.SetDeviceProvider(func() []device.Device { return []device.Device{{Serial: "offline", State: "offline"}} })
	for _, serial := range []string{"offline", "missing"} {
		w := artifactTestRequest(artifactTestMux(svc), "/api/automation/artifacts", url.Values{"serial": {serial}, "directory": {"."}})
		if w.Code != http.StatusNotFound {
			t.Fatalf("offline device reached ADB: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestArtifactsDeviceDirectoryErrorsAndHostFilesNotExposed(t *testing.T) {
	svc, _ := artifactHTTPFixture(t, "exports")
	mux := artifactTestMux(svc)
	w := artifactTestRequest(mux, "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"missing"}})
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing device directory: %d %s", w.Code, w.Body.String())
	}
	w = artifactTestRequest(mux, "/api/automation/artifacts/download", url.Values{"serial": {"phone"}, "directory": {"exports"}, "file": {"missing"}})
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing device file: %d %s", w.Code, w.Body.String())
	}
	hostDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostDir, "host-secret"), []byte("host-only bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	w = artifactTestRequest(mux, "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {hostDir}})
	if w.Code != 400 || strings.Contains(w.Body.String(), "host-only bytes") {
		t.Fatalf("host directory exposed: %d %s", w.Code, w.Body.String())
	}
}

func TestArtifactsDeviceDirectoryBounded(t *testing.T) {
	svc, root := artifactHTTPFixture(t, "many")
	for i := 0; i < artifactDirectoryLimit+2; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%04d.txt", i)), []byte("a"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w := artifactTestRequest(artifactTestMux(svc), "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"many"}})
	if w.Code != http.StatusOK || strings.Count(w.Body.String(), "<a href=") != artifactDirectoryLimit || !strings.Contains(w.Body.String(), "列表未完整展示") {
		t.Fatalf("device listing cap missing: code=%d links=%d body=%.200s", w.Code, strings.Count(w.Body.String(), "<a href="), w.Body.String())
	}
}

func TestArtifactsDeviceDownloadCapacityAndCancellation(t *testing.T) {
	svc, _ := artifactHTTPFixture(t, "exports")
	for i := 0; i < artifactHTTPConcurrency; i++ {
		if !svc.acquireArtifactHTTP() {
			t.Fatal("capacity too small")
		}
	}
	w := artifactTestRequest(artifactTestMux(svc), "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"exports"}})
	if w.Code != http.StatusTooManyRequests || len(svc.actSlots) != 0 {
		t.Fatalf("artifact capacity did not isolate act: %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < artifactHTTPConcurrency; i++ {
		<-svc.artifactHTTPSlots
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/automation/artifacts?serial=phone&directory=exports", nil).WithContext(ctx)
	w = httptest.NewRecorder()
	artifactTestMux(svc).ServeHTTP(w, req)
	if w.Code != http.StatusGatewayTimeout || len(svc.artifactHTTPSlots) != 0 {
		t.Fatalf("cancelled list leaked capacity: %d %s", w.Code, w.Body.String())
	}
}

func TestDeviceArtifactListingOutputBoundedRecords(t *testing.T) {
	cancelled := false
	out := &deviceArtifactListingOutput{cancel: func() { cancelled = true }}
	for i := 0; i < artifactDirectoryLimit+2; i++ {
		_, _ = out.Write([]byte(fmt.Sprintf("%d.txt\x001\x00", i)))
	}
	if !cancelled || !out.truncated || out.fields != 2*(artifactDirectoryLimit+1) {
		t.Fatalf("unbounded device output: %+v", out)
	}
	bad := &deviceArtifactListingOutput{cancel: func() {}}
	_, _ = bad.Write([]byte(strings.Repeat("x", 8193)))
	if bad.err == nil {
		t.Fatal("unbounded single device filename accepted")
	}
}

func TestArtifactURLsHTTPS(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://farm.test/api/automation/artifacts", nil)
	r.TLS = &tls.ConnectionState{}
	got := artifactDirectoryURL(r, "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"结果"}})
	u, err := url.Parse(got)
	if err != nil || u.Scheme != "https" || u.Host != "farm.test" || u.Query().Get("directory") != "结果" || u.Query().Get("serial") != "phone" {
		t.Fatalf("unexpected URL %q: %v", got, err)
	}
}

func TestArtifactsDeviceDownloadSizeLimit(t *testing.T) {
	svc, root := artifactHTTPFixture(t, "large")
	file, err := os.Create(filepath.Join(root, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(artifactDownloadLimit + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	w := artifactTestRequest(artifactTestMux(svc), "/api/automation/artifacts/download", url.Values{"serial": {"phone"}, "directory": {"large"}, "file": {"large.bin"}})
	if w.Code < 400 || !strings.Contains(w.Body.String(), "exceeds size limit") || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("oversize device file was downloaded: %d %.200s", w.Code, w.Body.String())
	}
	if len(svc.artifactHTTPSlots) != 0 {
		t.Fatal("oversize download leaked its capacity")
	}
}

func TestArtifactsMalformedDeviceListingFailsExplicitly(t *testing.T) {
	svc, _ := artifactHTTPFixture(t, "exports")
	find := filepath.Join(filepath.Dir(svc.adbPath), "find")
	if err := os.WriteFile(find, []byte("#!/bin/sh\nprintf 'incomplete record'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	w := artifactTestRequest(artifactTestMux(svc), "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"exports"}})
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "没有可下载") {
		t.Fatalf("invalid device response became an empty listing: %d %s", w.Code, w.Body.String())
	}
}

func TestArtifactsDeviceListUsesShellV2AndNULFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX executable")
	}
	svc, _ := newActTestService()
	adb := filepath.Join(t.TempDir(), "adb")
	// shell -T keeps binary stdout separate from stderr and preserves remote
	// exit status. exec-out cannot provide that failure contract on the device.
	fixture := `#!/usr/bin/env python3
import sys
args=sys.argv[1:]
if len(args)!=5 or args[:4]!=['-s','phone','shell','-T']:
    print('listing must use shell v2 without a PTY',file=sys.stderr);sys.exit(7)
script=args[4]
expected="-printf '%P"+chr(92)+"0%s"+chr(92)+"0'"
if expected not in script or chr(92)*2+'0' in script:
    print('listing must use a single-backslash Toybox NUL escape',file=sys.stderr);sys.exit(8)
sys.stdout.buffer.write(b'probe.txt'+bytes([0])+b'4'+bytes([0]))
`
	if err := os.WriteFile(adb, []byte(fixture), 0700); err != nil {
		t.Fatal(err)
	}
	svc.adbPath = adb
	w := artifactTestRequest(artifactTestMux(svc), "/api/automation/artifacts", url.Values{"serial": {"phone"}, "directory": {"."}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "probe.txt") || !strings.Contains(w.Body.String(), "4 字节") {
		t.Fatalf("shell v2 listing protocol failed: %d %s", w.Code, w.Body.String())
	}
}

func TestArtifactsMediaPreviewAndRange(t *testing.T) {
	svc, root := artifactHTTPFixture(t, "media")
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	videoData, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(svc.adbPath)), "movie.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"screen.png": imageData.Bytes(), "record.mp4": videoData, "unsafe.html": []byte("<html><script>alert(1)</script></html>")}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	mux := artifactTestMux(svc)
	for _, tc := range []struct{ file, contentType, disposition string }{
		{"screen.png", "image/png", "inline"}, {"record.mp4", "video/mp4", "inline"}, {"unsafe.html", "application/octet-stream", "attachment"},
	} {
		q := url.Values{"serial": {"phone"}, "directory": {"media"}, "file": {tc.file}, "preview": {"1"}}
		w := artifactTestRequest(mux, "/api/automation/artifacts/download", q)
		disposition, _, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
		if w.Code != 200 || w.Header().Get("Content-Type") != tc.contentType || disposition != tc.disposition || err != nil || !bytes.Equal(w.Body.Bytes(), files[tc.file]) {
			t.Fatalf("bad media preview %s: %d %v", tc.file, w.Code, w.Header())
		}
		req := httptest.NewRequest(http.MethodGet, "/api/automation/artifacts/download?"+q.Encode(), nil)
		req.Header.Set("Range", "bytes=0-7")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusPartialContent || !bytes.Equal(w.Body.Bytes(), files[tc.file][:8]) {
			t.Fatalf("preview range failed %s: %d %v", tc.file, w.Code, w.Header())
		}
		q.Del("preview")
		w = artifactTestRequest(mux, "/api/automation/artifacts/download", q)
		disposition, _, _ = mime.ParseMediaType(w.Header().Get("Content-Disposition"))
		if w.Code != 200 || disposition != "attachment" || w.Header().Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("default download changed %s: %d %v", tc.file, w.Code, w.Header())
		}
	}
	for _, values := range [][]string{{"true"}, {"0"}, {"1", "1"}} {
		w := artifactTestRequest(mux, "/api/automation/artifacts/download", url.Values{"serial": {"phone"}, "directory": {"media"}, "file": {"screen.png"}, "preview": values})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid preview flag accepted: %d", w.Code)
		}
	}
}
