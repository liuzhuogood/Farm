package companion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"farm/internal/automation"
	"farm/internal/uixml"
)

type fakeADB struct {
	mu                sync.Mutex
	services          string
	installed         bool
	version           string
	master            string
	forwards, removes int
	ignoreWrites      bool
	installSuccess    bool
	installedFileMode os.FileMode
}

func (f *fakeADB) run(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := strings.Join(args[2:], " ")
	switch {
	case cmd == "forward tcp:0 localabstract:farm_companion":
		f.forwards++
		return []byte("12345\n"), nil
	case cmd == "forward --remove tcp:12345":
		f.removes++
		return nil, nil
	case cmd == "shell settings get secure enabled_accessibility_services":
		return []byte(f.services + "\n"), nil
	case cmd == "shell settings get secure accessibility_enabled":
		return []byte(f.master + "\n"), nil
	case strings.HasPrefix(cmd, "shell settings put secure enabled_accessibility_services "):
		if !f.ignoreWrites {
			f.services = strings.Trim(args[len(args)-1], "'")
		}
		return nil, nil
	case cmd == "shell settings put secure accessibility_enabled 1":
		if !f.ignoreWrites {
			f.master = "1"
		}
		return nil, nil
	case cmd == "shell pm list packages "+Package:
		if f.installed {
			return []byte("package:" + Package + "\n"), nil
		}
		return nil, nil
	case cmd == "shell dumpsys package "+Package:
		name := "1.0.0"
		if f.version == "2" {
			name = "1.0.1"
		}
		return []byte("versionCode=" + f.version + " minSdk=26\nversionName=" + name + "\n"), nil
	case strings.HasPrefix(cmd, "install -r "):
		stat, err := os.Stat(args[len(args)-1])
		if err != nil {
			return nil, err
		}
		f.installedFileMode = stat.Mode().Perm()
		if f.installSuccess {
			f.installed = true
			f.version = "2"
		}
		return []byte("Success\n"), nil
	default:
		return nil, fmt.Errorf("unexpected fake adb args %v", args)
	}
}
func newFakeService(f *fakeADB, response any) *Service {
	return newFakeServiceWithHandler(f, func(command string) any {
		if command == "ping" {
			return map[string]any{"version": 2, "command": "pong"}
		}
		return response
	})
}
func newFakeServiceWithHandler(f *fakeADB, handle func(string) any) *Service {
	s := New("unused", []byte("test-apk"))
	s.run = f.run
	s.dial = func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			var req struct {
				Version int    `json:"version"`
				Command string `json:"command"`
			}
			if json.NewDecoder(server).Decode(&req) != nil || req.Version != 2 {
				return
			}
			if response := handle(req.Command); response != nil {
				_ = json.NewEncoder(server).Encode(response)
			}
		}()
		return client, nil
	}
	return s
}
func validResponse() map[string]any {
	return map[string]any{"version": 2, "xml": `<hierarchy><node text="Settings" bounds="[0,0][100,100]"/></hierarchy>`, "width": 1080, "height": 2400, "captured_at_ms": time.Now().UnixMilli()}
}
func enabledFake() *fakeADB {
	return &fakeADB{services: "other.app/.Reader:" + Component, installed: true, version: "2", master: "1"}
}

func TestSnapshotValidatesPayloadAndAlwaysRemovesForward(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response any
		want     string
	}{
		{"success", validResponse(), ""},
		{"unsupported", map[string]any{"version": 1}, "unsupported companion protocol"},
		{"agent error", map[string]any{"version": 2, "error": "no active root"}, "no active root"},
		{"missing metadata", map[string]any{"version": 2, "xml": "<hierarchy/>"}, "invalid companion snapshot metadata"},
		{"invalid XML", map[string]any{"version": 2, "xml": "<node/>", "width": 1, "height": 1, "captured_at_ms": 1}, "invalid companion hierarchy root"},
		{"oversize", map[string]any{"version": 2, "xml": strings.Repeat("x", maxResponseBytes)}, "exceeds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := enabledFake()
			s := newFakeService(f, tc.response)
			got, err := s.FetchSnapshot(context.Background(), "serial")
			if tc.want == "" {
				if err != nil || got.Source != "companion" || got.DisplaySize.Width != 1080 || got.CapturedAt.IsZero() {
					t.Fatalf("snapshot: %#v %v", got, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v want %s", err, tc.want)
			}
			if f.forwards != 1 || f.removes != 1 {
				t.Fatalf("forward leak: %+v", f)
			}
		})
	}
}

func TestSnapshotDisabledDoesNotInstallEnableOrForward(t *testing.T) {
	f := enabledFake()
	f.services = "other.app/.Reader"
	s := newFakeService(f, validResponse())
	_, err := s.FetchSnapshot(context.Background(), "serial")
	if err == nil || !strings.Contains(err.Error(), "not enabled") || f.forwards != 0 || f.services != "other.app/.Reader" {
		t.Fatalf("disabled observation mutated state: %+v %v", f, err)
	}
}

func TestSnapshotDeadlineAndGateCancellation(t *testing.T) {
	f := enabledFake()
	s := newFakeService(f, nil)
	s.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() { defer server.Close(); _, _ = io.Copy(io.Discard, server) }()
		return client, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := s.FetchSnapshot(ctx, "serial")
	if err == nil || f.removes != 1 {
		t.Fatalf("timeout failed or forward leaked: %v %+v", err, f)
	}
	_, release, err := s.acquire(context.Background(), "serial", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	_, _, err = s.acquire(ctx2, "serial", false)
	release()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("gate was not cancellable: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.gates) != 0 {
		t.Fatalf("device gate leak: %v", s.gates)
	}
}

func TestEnableDisablePreserveOtherServicesAndVerifyReadiness(t *testing.T) {
	f := enabledFake()
	f.services = "other.app/.Reader"
	f.master = "0"
	s := newFakeService(f, validResponse())
	status, err := s.Enable(context.Background(), "serial")
	if err != nil || !status.Ready || !status.Enabled || f.services != "other.app/.Reader:"+Component {
		t.Fatalf("enable: %+v %v services=%s", status, err, f.services)
	}
	status, err = s.Disable(context.Background(), "serial")
	if err != nil || status.Ready || status.Enabled || f.services != "other.app/.Reader" {
		t.Fatalf("disable: %+v %v services=%s", status, err, f.services)
	}
	f.ignoreWrites = true
	_, err = s.Enable(context.Background(), "serial")
	if err == nil || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("false settings success accepted: %v", err)
	}
}

func TestInstallVerifiesPackageAndDoesNotUninstallOnFailure(t *testing.T) {
	f := &fakeADB{version: "2"}
	s := newFakeService(f, validResponse())
	_, err := s.Install(context.Background(), "serial")
	if err == nil || !strings.Contains(err.Error(), "package is absent") {
		t.Fatalf("false install success accepted: %v", err)
	}
	if runtime.GOOS != "windows" && f.installedFileMode != 0600 {
		t.Fatalf("APK permissions %o", f.installedFileMode)
	}
	f.installSuccess = true
	status, err := s.Install(context.Background(), "serial")
	if err != nil || !status.Installed || status.Ready {
		t.Fatalf("install: %+v %v", status, err)
	}
	f.version = "1"
	_, err = s.Status(context.Background(), "serial")
	if err == nil || !strings.Contains(err.Error(), "unsupported installed") {
		t.Fatalf("unsupported version accepted: %v", err)
	}
}

func TestFallbackTemporarilyDisablesOnlyCompanionAndRestores(t *testing.T) {
	f := enabledFake()
	s := newFakeService(f, map[string]any{"version": 2, "error": "root unavailable"})
	before := f.services
	snapshot, err := s.FetchWithFallback(context.Background(), "serial", func(context.Context, string) (uixml.Snapshot, error) {
		if f.services != "other.app/.Reader" {
			t.Fatalf("fallback did not preserve peer service: %s", f.services)
		}
		return uixml.Snapshot{XML: "<hierarchy/>"}, nil
	})
	if err != nil || snapshot.Source != "uiautomator" || f.services != before {
		t.Fatalf("fallback: %#v %v services=%s", snapshot, err, f.services)
	}
}

func TestFallbackDoesNotOverwriteExternalSettingsChange(t *testing.T) {
	f := enabledFake()
	s := newFakeService(f, map[string]any{"version": 2, "error": "root unavailable"})
	_, err := s.FetchWithFallback(context.Background(), "serial", func(context.Context, string) (uixml.Snapshot, error) {
		f.services = "other.app/.Reader:new.app/.Reader"
		return uixml.Snapshot{}, errors.New("dump failed")
	})
	if err == nil || f.services != "other.app/.Reader:new.app/.Reader" {
		t.Fatalf("external settings overwritten: %s %v", f.services, err)
	}
}

func TestFallbackDoesNotRestoreOverQueuedDisable(t *testing.T) {
	f := enabledFake()
	s := newFakeService(f, map[string]any{"version": 2, "error": "root unavailable"})
	done := make(chan error, 1)
	_, err := s.FetchWithFallback(context.Background(), "serial", func(context.Context, string) (uixml.Snapshot, error) {
		s.mu.Lock()
		g := s.gates["serial"]
		rev := g.revision
		s.mu.Unlock()
		go func() { _, err := s.Disable(context.Background(), "serial"); done <- err }()
		deadline := time.Now().Add(time.Second)
		for s.revision(g) == rev && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if s.revision(g) == rev {
			t.Fatal("disable did not queue")
		}
		return uixml.Snapshot{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if containsService(f.services) {
		t.Fatalf("queued user disable overwritten: %s", f.services)
	}
}

func TestFallbackRestoresAccessibilityMasterAndSurfacesRestoreFailure(t *testing.T) {
	for _, failRestore := range []bool{false, true} {
		f := enabledFake()
		s := newFakeService(f, map[string]any{"version": 2, "error": "root unavailable"})
		_, err := s.FetchWithFallback(context.Background(), "serial", func(context.Context, string) (uixml.Snapshot, error) {
			f.master = "0" // Android/dump can turn off the master flag during fallback.
			f.ignoreWrites = failRestore
			return uixml.Snapshot{XML: "<hierarchy/>"}, nil
		})
		if failRestore {
			if err == nil || !strings.Contains(err.Error(), "restoration failed") {
				t.Fatalf("restore failure swallowed: %v", err)
			}
		} else if err != nil || f.master != "1" || !containsService(f.services) {
			t.Fatalf("master not restored: %v master=%s services=%s", err, f.master, f.services)
		}
	}
}

func TestPingRejectsOldOrMalformedProtocolAndAlwaysCleansForward(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response any
		want     string
	}{
		{"valid", map[string]any{"version": 2, "command": "pong"}, ""},
		{"old", map[string]any{"version": 1, "command": "pong"}, "update the Farm Companion APK"},
		{"missing command", map[string]any{"version": 2}, "unexpected companion ping response"},
		{"snapshot is not pong", validResponse(), "unexpected companion ping response"},
		{"agent failure", map[string]any{"version": 2, "error": "service_stopped"}, "service_stopped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := enabledFake()
			s := newFakeServiceWithHandler(f, func(command string) any {
				if command != "ping" {
					t.Errorf("ping requested %s", command)
				}
				return tc.response
			})
			err := s.pingLocked(context.Background(), "serial")
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ping error %v want %s", err, tc.want)
			}
			if f.forwards != 1 || f.removes != 1 {
				t.Fatalf("ping forward leak: %+v", f)
			}
		})
	}
}

func TestStatusReadinessDoesNotReadTreeAndReportsInstalledUpdate(t *testing.T) {
	f := enabledFake()
	var treeReads atomic.Int32
	s := newFakeServiceWithHandler(f, func(command string) any {
		if command == "snapshot" {
			treeReads.Add(1)
			return map[string]any{"version": 2, "error": "snapshot_timeout"}
		}
		return map[string]any{"version": 2, "command": "pong"}
	})
	status, err := s.Status(context.Background(), "serial")
	if err != nil || !status.Ready || !status.Enabled || status.UpdateAvailable || treeReads.Load() != 0 {
		t.Fatalf("status read tree or reported wrong state: %+v %v reads=%d", status, err, treeReads.Load())
	}
	f.version = "1"
	status, err = s.Status(context.Background(), "serial")
	if err == nil || !status.UpdateAvailable || !status.Installed || !status.Enabled || status.Ready || status.Version != "1.0.0" || !strings.Contains(status.Error, "update the Farm Companion APK") {
		t.Fatalf("old version: %+v %v", status, err)
	}
	before := f.services
	status, err = s.Enable(context.Background(), "serial")
	if err == nil || !status.UpdateAvailable || f.services != before {
		t.Fatalf("old version enabled: %+v %v", status, err)
	}
	status, err = s.Disable(context.Background(), "serial")
	if err != nil || status.Enabled || !status.UpdateAvailable || containsService(f.services) {
		t.Fatalf("old APK could not be disabled safely: %+v %v", status, err)
	}
	f.installed = false
	status, err = s.Status(context.Background(), "serial")
	raw, _ := json.Marshal(status)
	if err != nil || status.UpdateAvailable || !strings.Contains(string(raw), `"update_available":false`) {
		t.Fatalf("uninstalled version: %+v %v JSON=%s", status, err, raw)
	}
	f.installed = true
	f.installSuccess = true
	status, err = s.Install(context.Background(), "serial")
	if err != nil || status.UpdateAvailable || status.Version != "1.0.1" {
		t.Fatalf("explicit update: %+v %v", status, err)
	}
}

func TestSnapshotTimeoutFallbackSucceedsWithPingOnlyRestoration(t *testing.T) {
	f := enabledFake()
	s := newFakeService(f, validResponse())
	var snapshots, pings atomic.Int32
	s.dial = func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			var request struct {
				Version int    `json:"version"`
				Command string `json:"command"`
			}
			if json.NewDecoder(server).Decode(&request) != nil {
				return
			}
			if request.Command == "snapshot" {
				snapshots.Add(1)
				_, _ = io.Copy(io.Discard, server)
				return
			}
			pings.Add(1)
			_ = json.NewEncoder(server).Encode(map[string]any{"version": 2, "command": "pong"})
		}()
		return client, nil
	}
	before := f.services
	auto := &automation.Service{}
	auto.SetUIXMLProvider(testDumpProvider{fetch: func(context.Context, string) (uixml.Snapshot, error) {
		f.master = "0"
		return uixml.Snapshot{XML: "<hierarchy/>"}, nil
	}})
	auto.SetFastUIXMLProvider(s)
	mux := http.NewServeMux()
	auto.RegisterRoutes(mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest("POST", "/api/automation/observe", strings.NewReader(`{"serial":"serial","formats":["xml"]}`)))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "<hierarchy/>" || snapshots.Load() != 1 || pings.Load() != 1 || f.master != "1" || f.services != before {
		t.Fatalf("successful dump lost to repeat tree: HTTP=%d body=%s reads=%d pings=%d master=%s services=%s", recorder.Code, recorder.Body.String(), snapshots.Load(), pings.Load(), f.master, f.services)
	}
	if f.forwards != f.removes {
		t.Fatalf("timeout forward leak: %+v", f)
	}
}

func TestEnableWaitsForPingRestartAndRemainsBounded(t *testing.T) {
	f := enabledFake()
	f.services = "other.app/.Reader"
	f.master = "0"
	var attempts atomic.Int32
	s := newFakeServiceWithHandler(f, func(command string) any {
		if command != "ping" {
			return map[string]any{"version": 2, "error": "tree must not be read"}
		}
		if attempts.Add(1) < 3 {
			return nil
		}
		return map[string]any{"version": 2, "command": "pong"}
	})
	status, err := s.Enable(context.Background(), "serial")
	if err != nil || !status.Ready || attempts.Load() != 3 {
		t.Fatalf("restart wait: %+v %v attempts=%d", status, err, attempts.Load())
	}
	s = newFakeServiceWithHandler(f, func(string) any { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	status, err = s.Enable(ctx, "serial")
	if err == nil || status.Ready || !status.Enabled || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("enable was not bounded: %+v %v elapsed=%v", status, err, time.Since(started))
	}
}

func TestRestoreRetriesPingAndDoesNotHideReadbackFailure(t *testing.T) {
	f := enabledFake()
	before := f.services
	f.services = "other.app/.Reader"
	f.master = "0"
	var attempts atomic.Int32
	s := newFakeServiceWithHandler(f, func(command string) any {
		if command != "ping" {
			t.Error("restore read snapshot")
		}
		if attempts.Add(1) < 2 {
			return nil
		}
		return map[string]any{"version": 2, "command": "pong"}
	})
	g, release, err := s.acquire(context.Background(), "serial", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	err = s.restoreLocked(ctx, "serial", g, s.revision(g), before, f.services, "1")
	cancel()
	release()
	if err != nil || attempts.Load() != 2 || f.services != before || f.master != "1" {
		t.Fatalf("restore restart: %v attempts=%d", err, attempts.Load())
	}
	s = newFakeServiceWithHandler(f, func(string) any { return nil })
	g, release, err = s.acquire(context.Background(), "serial", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
	err = s.restoreLocked(ctx, "serial", g, s.revision(g), before, before, "1")
	cancel()
	release()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("restore ping wait was not bounded: %v", err)
	}
	s = newFakeService(f, validResponse())
	rawRunner := s.run
	masterReads := 0
	s.run = func(ctx context.Context, args ...string) ([]byte, error) {
		if strings.Join(args[2:], " ") == "shell settings get secure accessibility_enabled" {
			masterReads++
			return nil, errors.New("master readback failed")
		}
		return rawRunner(ctx, args...)
	}
	g, release, err = s.acquire(context.Background(), "serial", false)
	if err != nil {
		t.Fatal(err)
	}
	err = s.restoreLocked(context.Background(), "serial", g, s.revision(g), before, before, "1")
	release()
	if err == nil || !strings.Contains(err.Error(), "master readback failed") || masterReads != 1 {
		t.Fatalf("readback failure hidden: %v", err)
	}
}

type testDumpProvider struct {
	fetch func(context.Context, string) (uixml.Snapshot, error)
}

func (p testDumpProvider) FetchSnapshot(ctx context.Context, serial string) (uixml.Snapshot, error) {
	return p.fetch(ctx, serial)
}
