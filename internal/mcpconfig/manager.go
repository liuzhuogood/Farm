// Package mcpconfig manages only the Farm entry in supported MCP clients.
package mcpconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

type Client struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Config string `json:"config"`
}
type Manager struct{ home, endpoint string }

func New(home, endpoint string) *Manager {
	if canonical, err := filepath.EvalSymlinks(home); err == nil {
		home = canonical
	}
	return &Manager{home: home, endpoint: endpoint}
}
func (m *Manager) clients() []Client {
	return []Client{
		{ID: "codex", Name: "Codex", Path: filepath.Join(m.home, ".codex", "config.toml")},
		{ID: "claude-code", Name: "Claude Code", Path: filepath.Join(m.home, ".claude.json")},
		{ID: "cursor", Name: "Cursor", Path: filepath.Join(m.home, ".cursor", "mcp.json")},
	}
}
func (m *Manager) entry(id string) map[string]any {
	e := map[string]any{"url": m.endpoint}
	if id == "claude-code" {
		e["type"] = "http"
	}
	return e
}
func key(c Client) string {
	if c.ID == "codex" {
		return "mcp_servers"
	}
	return "mcpServers"
}
func read(c Client) ([]byte, map[string]any, os.FileMode, error) {
	data := map[string]any{}
	mode := os.FileMode(0600)
	info, err := os.Lstat(c.Path)
	if os.IsNotExist(err) {
		return nil, data, mode, nil
	}
	if err != nil {
		return nil, nil, mode, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, mode, fmt.Errorf("配置必须是普通文件，不能是链接")
	}
	if info.Size() > 4<<20 {
		return nil, nil, mode, fmt.Errorf("配置文件超过 4 MiB")
	}
	b, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, nil, mode, err
	}
	mode = info.Mode().Perm()
	if c.ID == "codex" {
		err = toml.Unmarshal(b, &data)
	} else {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		err = dec.Decode(&data)
		if err == nil && !json.Valid(b) {
			err = fmt.Errorf("无效 JSON")
		}
	}
	if err != nil || data == nil {
		return nil, nil, mode, fmt.Errorf("配置格式无效，请先修复配置文件")
	}
	if v, ok := data[key(c)]; ok {
		if _, ok := v.(map[string]any); !ok {
			return nil, nil, mode, fmt.Errorf("%s 必须是对象", key(c))
		}
	}
	return b, data, mode, nil
}
func (m *Manager) describe(c Client) Client {
	e := m.entry(c.ID)
	if c.ID == "codex" {
		b, _ := toml.Marshal(map[string]any{key(c): map[string]any{"farm": e}})
		c.Config = string(b)
	} else {
		b, _ := json.MarshalIndent(map[string]any{key(c): map[string]any{"farm": e}}, "", "  ")
		c.Config = string(b)
	}
	_, data, _, err := read(c)
	c.Status = "not_installed"
	if err != nil {
		c.Status = "error"
		c.Error = err.Error()
		return c
	}
	if servers, ok := data[key(c)].(map[string]any); ok {
		if old, exists := servers["farm"]; exists {
			if reflect.DeepEqual(old, e) {
				c.Status = "installed"
			} else {
				c.Status = "conflict"
				c.Error = "已有不同的 Farm 配置，请先核对配置文件"
			}
		}
	}
	return c
}
func (m *Manager) List() []Client {
	cs := m.clients()
	for i := range cs {
		cs[i] = m.describe(cs[i])
	}
	return cs
}
func (m *Manager) Mutate(ctx context.Context, id string, install bool) (Client, error) {
	var c Client
	for _, item := range m.clients() {
		if item.ID == id {
			c = item
			break
		}
	}
	if c.ID == "" {
		return c, fmt.Errorf("不支持的客户端")
	}
	parent := filepath.Dir(c.Path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return c, err
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return c, err
	}
	// Reject redirected configuration directories rather than writing through links.
	if filepath.Clean(realParent) != filepath.Clean(parent) {
		return c, fmt.Errorf("配置目录不能包含符号链接")
	}
	lock := flock.New(c.Path + ".farm.lock")
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	locked, err := lock.TryLockContext(lockCtx, 30*time.Millisecond)
	if err != nil || !locked {
		return c, fmt.Errorf("配置正在被修改，请稍后重试")
	}
	defer lock.Unlock()
	original, data, mode, err := read(c)
	if err != nil {
		return c, err
	}
	servers, ok := data[key(c)].(map[string]any)
	if !ok {
		servers = map[string]any{}
		data[key(c)] = servers
	}
	expected := m.entry(c.ID)
	if old, exists := servers["farm"]; exists {
		if !reflect.DeepEqual(old, expected) {
			return c, fmt.Errorf("已有不同的 Farm 配置，未覆盖或删除")
		}
		if install {
			return m.describe(c), nil
		}
	} else if !install {
		return m.describe(c), nil
	}
	if install {
		servers["farm"] = expected
	} else {
		delete(servers, "farm")
	}
	var out []byte
	if c.ID == "codex" {
		out, err = editTOML(original, expected, install)
	} else {
		out, err = json.MarshalIndent(data, "", "  ")
		out = append(out, '\n')
	}
	if err != nil {
		return c, err
	}
	f, err := os.CreateTemp(parent, ".farm-mcp-*")
	if err != nil {
		return c, err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(out)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return c, err
	}
	if closeErr != nil {
		return c, closeErr
	}
	current, _, _, err := read(c)
	if err != nil {
		return c, err
	}
	if !bytes.Equal(current, original) {
		return c, fmt.Errorf("配置在操作期间被其他程序修改，未写入")
	}
	if err = os.Rename(name, c.Path); err != nil {
		return c, err
	}
	result := m.describe(c)
	want := "not_installed"
	if install {
		want = "installed"
	}
	if result.Status != want {
		return result, fmt.Errorf("配置写入后的核验未通过")
	}
	log.Printf("[mcpconfig] client=%s installed=%t", c.ID, install)
	return result, nil
}

// Endpoint converts wildcard listen addresses to a local client endpoint.
func Endpoint(addr string, https bool) string {
	scheme := "http"
	if https {
		scheme = "https"
	}
	addr = strings.Replace(addr, "0.0.0.0:", "127.0.0.1:", 1)
	addr = strings.Replace(addr, "[::]:", "127.0.0.1:", 1)
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	return scheme + "://" + addr + "/mcp"
}
