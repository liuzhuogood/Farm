package mcpconfig

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// editTOML retains comments and unrelated bytes; unusual inline/dotted Farm
// representations are left for manual editing rather than rewritten globally.
func editTOML(src []byte, expected map[string]any, install bool) ([]byte, error) {
	var out []byte
	if install {
		block, err := toml.Marshal(expected)
		if err != nil {
			return nil, err
		}
		out = append(append([]byte{}, src...), '\n')
		out = append(out, []byte("[mcp_servers.farm]\n")...)
		out = append(out, block...)
	} else {
		type span struct{ start, end int }
		var spans []span
		inFarm := false
		found := false
		p := unstable.Parser{}
		p.Reset(src)
		lineStart := func(offset int) int { return bytes.LastIndexByte(src[:offset], '\n') + 1 }
		lineEnd := func(offset int) int {
			if i := bytes.IndexByte(src[offset:], '\n'); i >= 0 {
				return offset + i + 1
			}
			return len(src)
		}
		for p.NextExpression() {
			node := p.Expression()
			if node.Kind != unstable.Table && node.Kind != unstable.ArrayTable && node.Kind != unstable.KeyValue {
				continue
			}
			keys := node.Key()
			var names []string
			offset := 0
			for keys.Next() {
				k := keys.Node()
				if len(names) == 0 {
					offset = int(k.Raw.Offset)
				}
				names = append(names, string(k.Data))
			}
			if node.Kind == unstable.Table || node.Kind == unstable.ArrayTable {
				inFarm = reflect.DeepEqual(names, []string{"mcp_servers", "farm"}) && node.Kind == unstable.Table
				if inFarm {
					found = true
					spans = append(spans, span{lineStart(offset), lineEnd(offset)})
				}
			} else if inFarm {
				v := node.Value()
				spans = append(spans, span{lineStart(offset), lineEnd(int(v.Raw.Offset + v.Raw.Length))})
			}
		}
		if err := p.Error(); err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("Farm 配置使用内联或点号格式，请在配置文件中手动卸载")
		}
		previous := 0
		for _, s := range spans {
			out = append(out, src[previous:s.start]...)
			previous = s.end
		}
		out = append(out, src[previous:]...)
	}
	var before, after map[string]any
	if err := toml.Unmarshal(src, &before); err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(out, &after); err != nil {
		return nil, fmt.Errorf("配置格式不支持安全局部修改，请手动配置")
	}
	originalServers, ok := before["mcp_servers"].(map[string]any)
	if !ok {
		originalServers = map[string]any{}
		before["mcp_servers"] = originalServers
	}
	if install {
		originalServers["farm"] = expected
	} else {
		delete(originalServers, "farm")
	}
	if _, exists := after["mcp_servers"]; !exists && len(originalServers) == 0 {
		after["mcp_servers"] = map[string]any{}
	}
	if !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("配置局部修改校验失败，未写入")
	}
	return out, nil
}
