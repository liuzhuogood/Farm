package mcpconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestInstallUninstallPreservesOtherConfiguration(t *testing.T) {
	for _, id := range []string{"codex", "claude-code", "cursor"} {
		t.Run(id, func(t *testing.T) {
			m := New(t.TempDir(), "http://127.0.0.1:8080/mcp")
			var client Client
			for _, c := range m.clients() {
				if c.ID == id {
					client = c
				}
			}
			if err := os.MkdirAll(filepath.Dir(client.Path), 0700); err != nil {
				t.Fatal(err)
			}
			data := map[string]any{"preferences": map[string]any{"theme": "dark"}, key(client): map[string]any{"other": map[string]any{"command": "original"}}}
			var b []byte
			if id == "codex" {
				b, _ = toml.Marshal(data)
			} else {
				b, _ = json.Marshal(data)
			}
			if err := os.WriteFile(client.Path, b, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := m.Mutate(context.Background(), id, true)
			if err != nil || got.Status != "installed" {
				t.Fatalf("install: %+v %v", got, err)
			}
			got, err = m.Mutate(context.Background(), id, true)
			if err != nil || got.Status != "installed" {
				t.Fatalf("idempotent install: %+v %v", got, err)
			}
			got, err = m.Mutate(context.Background(), id, false)
			if err != nil || got.Status != "not_installed" {
				t.Fatalf("uninstall: %+v %v", got, err)
			}
			_, after, _, err := read(client)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, data) {
				t.Fatalf("unrelated values changed: %#v", after)
			}
		})
	}
}
func TestRejectDamagedConflictingAndLinkedConfiguration(t *testing.T) {
	for _, input := range []string{`{"broken":`, `null`, `{"mcpServers":[]}`, `{"mcpServers":{"farm":{"url":"https://other.test/mcp"}}}`} {
		t.Run(input, func(t *testing.T) {
			m := New(t.TempDir(), "http://127.0.0.1:8080/mcp")
			file := filepath.Join(m.home, ".claude.json")
			if err := os.WriteFile(file, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			for _, install := range []bool{true, false} {
				if _, err := m.Mutate(context.Background(), "claude-code", install); err == nil {
					t.Fatal("expected rejection")
				}
				after, _ := os.ReadFile(file)
				if string(after) != input {
					t.Fatal("configuration changed")
				}
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		m := New(t.TempDir(), "http://127.0.0.1:8080/mcp")
		target := filepath.Join(t.TempDir(), "target")
		os.WriteFile(target, []byte(`{}`), 0600)
		os.Symlink(target, filepath.Join(m.home, ".claude.json"))
		if _, err := m.Mutate(context.Background(), "claude-code", true); err == nil {
			t.Fatal("expected symlink rejection")
		}
	})
}
func TestRoutesRejectRemoteAndCrossOrigin(t *testing.T) {
	m := New(t.TempDir(), "http://127.0.0.1:8080/mcp")
	mux := http.NewServeMux()
	RegisterRoutes(mux, m)
	for _, tc := range []struct {
		remote, origin string
		want           int
	}{{"192.168.1.2:1234", "", 403}, {"127.0.0.1:1234", "http://evil.test", 403}, {"127.0.0.1:1234", "http://example.com", 200}} {
		req := httptest.NewRequest("POST", "http://example.com/api/ai-integrations/mcp/claude-code", nil)
		req.RemoteAddr = tc.remote
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
	}
}

func TestViteProxyOrigin(t *testing.T) {
	m := New(t.TempDir(), "http://127.0.0.1:8080/mcp")
	mux := http.NewServeMux()
	RegisterRoutes(mux, m)
	for _, tc := range []struct {
		origin string
		status int
	}{{"http://127.0.0.1:5173", 200}, {"http://localhost:5173", 200}, {"http://evil.test:5173", 403}, {"http://127.0.0.1:9999", 403}} {
		r := httptest.NewRequest("GET", "http://localhost:8080/api/ai-integrations/mcp", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("origin %s: %d", tc.origin, w.Code)
		}
	}
}

func TestCodexFreshAndComments(t *testing.T) {
	for _, initial := range []string{"", "# keep my note\nmodel = 'example'\n[mcp_servers.other]\n# keep this too\nurl = 'https://other.test/mcp'\n"} {
		t.Run(initial, func(t *testing.T) {
			m := New(t.TempDir(), "http://127.0.0.1:8080/mcp")
			path := filepath.Join(m.home, ".codex", "config.toml")
			if initial != "" {
				os.MkdirAll(filepath.Dir(path), 0700)
				os.WriteFile(path, []byte(initial), 0600)
			}
			if _, err := m.Mutate(context.Background(), "codex", true); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Mutate(context.Background(), "codex", false); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(path)
			if string(after) != initial+"\n" {
				t.Fatalf("other bytes changed: %q", after)
			}
		})
	}
}
