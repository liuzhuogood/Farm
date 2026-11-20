package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeChannel(t *testing.T) {
	for _, tt := range []struct {
		value   string
		want    string
		wantErr bool
	}{
		{"", ChannelStable, false},
		{"stable", ChannelStable, false},
		{" Beta ", ChannelBeta, false},
		{"nightly", "", true},
	} {
		got, err := NormalizeChannel(tt.value)
		if (err != nil) != tt.wantErr {
			t.Fatalf("NormalizeChannel(%q) err = %v, wantErr %v", tt.value, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Fatalf("NormalizeChannel(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestPickReleaseSkipsDraftAndForeignPrerelease(t *testing.T) {
	releases := []ReleaseInfo{
		{TagName: "v2026.11.30", Draft: true},
		{TagName: "v2026.11.29-rc1", Prerelease: true},
		{TagName: "v2026.11.28-beta.2", Prerelease: true},
		{TagName: "v2026.11.19"},
	}
	if got := pickRelease(releases, true); got == nil || got.TagName != "v2026.11.28-beta.2" {
		t.Fatalf("beta channel picked %+v", got)
	}
	if got := pickRelease(releases, false); got == nil || got.TagName != "v2026.11.19" {
		t.Fatalf("stable channel picked %+v", got)
	}
}

func newChannelChecker(current string, handler http.HandlerFunc) (*Checker, *httptest.Server) {
	server := httptest.NewServer(handler)
	checker := &Checker{repo: "mock/repo", currentVersion: current, httpClient: server.Client()}
	checker.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		request, err := http.NewRequest(r.Method, server.URL+r.URL.RequestURI(), r.Body)
		if err != nil {
			return nil, err
		}
		return http.DefaultTransport.RoundTrip(request)
	})
	return checker, server
}

func githubStub(t *testing.T, latest ReleaseInfo, releases []ReleaseInfo) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/releases/latest") {
			_ = json.NewEncoder(w).Encode(latest)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/releases") {
			_ = json.NewEncoder(w).Encode(releases)
			return
		}
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	}
}

func TestCheckUpdateChannel(t *testing.T) {
	asset := ReleaseAsset{Name: "farm-linux-amd64-cli", Size: 10, BrowserDownloadURL: "https://github.com/mock/repo/releases/download/v2026.11.20-beta.1/farm-linux-amd64-cli", Digest: "sha256:abc"}
	latest := ReleaseInfo{TagName: "v2026.11.20", PublishedAt: time.Now()}
	releases := []ReleaseInfo{
		{TagName: "v2026.11.20", PublishedAt: time.Now()},
		{TagName: "v2026.11.20-beta.1", Prerelease: true, PublishedAt: time.Now(), Assets: []ReleaseAsset{asset}},
		{TagName: "v2026.11.20-beta.0", Prerelease: true, PublishedAt: time.Now()},
		{TagName: "v2026.11.21", Draft: true},
	}
	checker, server := newChannelChecker("v2026.11.20-beta.0", githubStub(t, latest, releases))
	defer server.Close()

	stable, err := checker.CheckUpdateChannel(ChannelStable)
	if err != nil {
		t.Fatalf("stable check failed: %v", err)
	}
	if stable.LatestVersion != "v2026.11.20" || !stable.HasUpdate {
		t.Fatalf("stable result = %+v", stable)
	}

	beta, err := checker.CheckUpdateChannel(ChannelBeta)
	if err != nil {
		t.Fatalf("beta check failed: %v", err)
	}
	// beta 通道取当前可见的较高者，正式首版高于同号 beta。
	if beta.LatestVersion != "v2026.11.20" || !beta.HasUpdate || beta.Channel != ChannelBeta {
		t.Fatalf("beta result = %+v", beta)
	}
}

func TestCheckUpdateChannelPrefersNewerBeta(t *testing.T) {
	latest := ReleaseInfo{TagName: "v2026.11.19", PublishedAt: time.Now()}
	releases := []ReleaseInfo{
		{TagName: "v2026.11.19", PublishedAt: time.Now()},
		{TagName: "v2026.11.20-beta.3", Prerelease: true, PublishedAt: time.Now()},
	}
	checker, server := newChannelChecker("v2026.11.19", githubStub(t, latest, releases))
	defer server.Close()

	stable, err := checker.CheckUpdateChannel(ChannelStable)
	if err != nil || stable.HasUpdate {
		t.Fatalf("stable should stay on v2026.11.19, got %+v err=%v", stable, err)
	}
	beta, err := checker.CheckUpdateChannel(ChannelBeta)
	if err != nil || beta.LatestVersion != "v2026.11.20-beta.3" || !beta.HasUpdate {
		t.Fatalf("beta should see v2026.11.20-beta.3, got %+v err=%v", beta, err)
	}
}

func TestDesktopEndpoint(t *testing.T) {
	latest := ReleaseInfo{TagName: "v2026.11.19"}
	releases := []ReleaseInfo{
		{TagName: "v2026.11.19"},
		{TagName: "v2026.11.20-beta.2", Prerelease: true},
	}
	checker, server := newChannelChecker("v2026.11.19", githubStub(t, latest, releases))
	defer server.Close()

	stable, stableChannel, err := checker.DesktopEndpoint(ChannelStable)
	if err != nil || stable != desktopStableEndpoint || stableChannel != ChannelStable {
		t.Fatalf("stable endpoint = %q channel = %q err=%v", stable, stableChannel, err)
	}
	beta, betaChannel, err := checker.DesktopEndpoint(ChannelBeta)
	want := "https://github.com/mock/repo/releases/download/v2026.11.20-beta.2/desktop-latest.json"
	if err != nil || beta != want || betaChannel != ChannelBeta {
		t.Fatalf("beta endpoint = %q channel = %q want %q err=%v", beta, betaChannel, want, err)
	}
}

func TestDesktopEndpointFallsBackWithoutBeta(t *testing.T) {
	// 没有 beta 发布时不能把正式 tag 拼成下载地址，必须回退正式索引并报告实际通道。
	latest := ReleaseInfo{TagName: "v2026.11.17-1"}
	releases := []ReleaseInfo{
		{TagName: "v2026.11.17-1"},
		{TagName: "v2026.11.16-1"},
	}
	checker, server := newChannelChecker("v2026.11.16-1", githubStub(t, latest, releases))
	defer server.Close()

	endpoint, effective, err := checker.DesktopEndpoint(ChannelBeta)
	if err != nil || endpoint != desktopStableEndpoint || effective != ChannelStable {
		t.Fatalf("endpoint = %q channel = %q err=%v", endpoint, effective, err)
	}
}

type fakeStore map[string]string

func (s fakeStore) GetConfig(key string) (string, error) { return s[key], nil }
func (s fakeStore) SetConfig(key, value string) error {
	s[key] = value
	return nil
}

func TestServiceChannelPersists(t *testing.T) {
	store := fakeStore{}
	service := &Service{checker: &Checker{repo: "mock/repo", currentVersion: "v2026.11.20"}}
	if err := service.AttachStore(store); err != nil {
		t.Fatalf("attach store: %v", err)
	}
	if got := service.Channel(); got != ChannelStable {
		t.Fatalf("default channel = %q", got)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	put := httptest.NewRequest(http.MethodPut, "/api/version/channel", strings.NewReader(`{"channel":"beta"}`))
	putRecorder := httptest.NewRecorder()
	mux.ServeHTTP(putRecorder, put)
	if putRecorder.Code != http.StatusOK {
		t.Fatalf("put channel status %d: %s", putRecorder.Code, putRecorder.Body.String())
	}
	if store[channelConfigKey] != ChannelBeta || service.Channel() != ChannelBeta {
		t.Fatalf("channel not persisted: store=%v service=%s", store, service.Channel())
	}

	reloaded := &Service{checker: &Checker{repo: "mock/repo", currentVersion: "v2026.11.20"}}
	if err := reloaded.AttachStore(store); err != nil || reloaded.Channel() != ChannelBeta {
		t.Fatalf("reload channel = %s err=%v", reloaded.Channel(), err)
	}
}

func TestServiceChannelRejectsUnknown(t *testing.T) {
	service := &Service{checker: &Checker{repo: "mock/repo", currentVersion: "v2026.11.20"}}
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	put := httptest.NewRequest(http.MethodPut, "/api/version/channel", strings.NewReader(`{"channel":"nightly"}`))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, put)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if service.Channel() != ChannelStable {
		t.Fatalf("channel changed on invalid input: %s", service.Channel())
	}
}
