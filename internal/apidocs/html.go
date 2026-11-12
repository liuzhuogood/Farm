package apidocs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const automationMarkdownMarker = "<!-- SERVER_RENDERED_MARKDOWN -->"

type automationSpec struct {
	Info struct {
		Title string `json:"title"`
	} `json:"info"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

type operation struct {
	Tags        []string                   `json:"tags"`
	Summary     string                     `json:"summary"`
	Description string                     `json:"description"`
	Parameters  []json.RawMessage          `json:"parameters"`
	RequestBody json.RawMessage            `json:"requestBody"`
	Responses   map[string]json.RawMessage `json:"responses"`
}

// RenderAutomationHTML emits a compact Markdown-like contract in a minimal HTML
// shell. All endpoint content is in the response body; no client-side fetch is used.
func RenderAutomationHTML(pageTemplate, specJSON []byte) ([]byte, error) {
	if !bytes.Contains(pageTemplate, []byte(automationMarkdownMarker)) {
		return nil, fmt.Errorf("automation HTML template is missing server render marker")
	}
	var spec automationSpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, fmt.Errorf("decode filtered OpenAPI: %w", err)
	}

	var md strings.Builder
	title := spec.Info.Title
	if title == "" {
		title = "Farm AI 自动化接口"
	}
	md.WriteString("# " + title + "\n\n")
	md.WriteString("服务端生成；接口参数与请求/响应结构直接包含在本文档中。\n\n")
	md.WriteString("## 调用约定\n")
	md.WriteString("- JSON 请求使用 `Content-Type: application/json`。\n")
	md.WriteString("- 设备操作通过 `serial` 指定设备。\n")
	md.WriteString("- 点击/滑动 `x,y` 使用 0~1 归一化坐标；感知输出 `@X,Y` 是屏幕像素坐标，按屏幕宽高归一化后传入动作接口。\n")
	md.WriteString("- 推荐闭环：`GET /api/devices` → `POST /api/automation/observe` → 动作接口 → 再次 observe 确认变化。\n")
	md.WriteString("- 当前控制能力由下列 tap/swipe/key 接口提供；统一伪指令入口 `/api/automation/act` 尚未实现。\n\n")

	paths := make([]string, 0, len(spec.Paths))
	for path := range spec.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	methodOrder := []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"}
	lastTag := ""
	for _, path := range paths {
		for _, method := range methodOrder {
			raw, ok := spec.Paths[path][method]
			if !ok {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, fmt.Errorf("decode operation %s %s: %w", strings.ToUpper(method), path, err)
			}
			tag := "其他"
			if len(op.Tags) > 0 && op.Tags[0] != "" {
				tag = op.Tags[0]
			}
			if tag != lastTag {
				md.WriteString("## " + tag + "\n\n")
				lastTag = tag
			}
			md.WriteString("### " + strings.ToUpper(method) + " " + path)
			if op.Summary != "" {
				md.WriteString(" — " + oneLine(op.Summary))
			}
			md.WriteByte('\n')
			if desc := oneLine(op.Description); desc != "" {
				md.WriteString(desc + "\n")
			}
			for _, rawParameter := range op.Parameters {
				var p struct {
					Name        string          `json:"name"`
					In          string          `json:"in"`
					Required    bool            `json:"required"`
					Description string          `json:"description"`
					Schema      json.RawMessage `json:"schema"`
				}
				if err := json.Unmarshal(rawParameter, &p); err != nil {
					return nil, fmt.Errorf("decode parameter for %s %s: %w", strings.ToUpper(method), path, err)
				}
				marker := "?"
				if p.Required {
					marker = "!"
				}
				md.WriteString("- `" + p.Name + "` (" + p.In + marker + "): " + compactSchema(p.Schema, spec.Components.Schemas, map[string]bool{}) + descriptionSuffix(p.Description) + "\n")
			}
			if len(op.RequestBody) > 0 && string(op.RequestBody) != "null" {
				var body struct {
					Required bool `json:"required"`
					Content  map[string]struct {
						Schema json.RawMessage `json:"schema"`
					} `json:"content"`
				}
				if err := json.Unmarshal(op.RequestBody, &body); err != nil {
					return nil, fmt.Errorf("decode request body for %s %s: %w", strings.ToUpper(method), path, err)
				}
				required := "optional"
				if body.Required {
					required = "required"
				}
				for _, contentType := range sortedKeys(body.Content) {
					md.WriteString("- Body (" + required + ", " + contentType + "): `" + compactSchema(body.Content[contentType].Schema, spec.Components.Schemas, map[string]bool{}) + "`\n")
				}
			}
			for _, status := range sortedKeys(op.Responses) {
				var response struct {
					Description string `json:"description"`
					Content     map[string]struct {
						Schema json.RawMessage `json:"schema"`
					} `json:"content"`
				}
				if err := json.Unmarshal(op.Responses[status], &response); err != nil {
					return nil, fmt.Errorf("decode response %s for %s %s: %w", status, strings.ToUpper(method), path, err)
				}
				md.WriteString("- Response `" + status + "`: " + oneLine(response.Description))
				for _, contentType := range sortedKeys(response.Content) {
					md.WriteString("; " + contentType + " `" + compactSchema(response.Content[contentType].Schema, spec.Components.Schemas, map[string]bool{}) + "`")
				}
				md.WriteByte('\n')
			}
			md.WriteByte('\n')
		}
	}
	return bytes.Replace(pageTemplate, []byte(automationMarkdownMarker), []byte(escapePreText(md.String())), 1), nil
}

func escapePreText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

func compactSchema(raw json.RawMessage, components map[string]json.RawMessage, stack map[string]bool) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "any"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	encoded, err := json.Marshal(compactSchemaValue(value, components, stack))
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

func compactSchemaValue(value any, components map[string]json.RawMessage, stack map[string]bool) any {
	switch v := value.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok && strings.HasPrefix(ref, "#/components/schemas/") {
			name := strings.TrimPrefix(ref, "#/components/schemas/")
			if !stack[name] {
				if target, ok := components[name]; ok {
					stack[name] = true
					var expanded any
					if json.Unmarshal(target, &expanded) == nil {
						out := compactSchemaValue(expanded, components, stack)
						delete(stack, name)
						return out
					}
					delete(stack, name)
				}
			}
			return map[string]any{"$ref": name}
		}
		out := make(map[string]any)
		for key, child := range v {
			switch key {
			case "description", "examples", "example", "title", "externalDocs", "deprecated":
				continue
			}
			out[key] = compactSchemaValue(child, components, stack)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = compactSchemaValue(child, components, stack)
		}
		return out
	default:
		return value
	}
}

func descriptionSuffix(description string) string {
	if description == "" {
		return ""
	}
	return " — " + oneLine(description)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ServeAutomationHTML serves the complete server-rendered automation API reference.
func (m *Manager) ServeAutomationHTML(w http.ResponseWriter, r *http.Request, pageTemplate []byte) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	spec, err := m.GetAutomationSpec()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to generate automation openapi spec: %v", err), http.StatusInternalServerError)
		return
	}
	page, err := RenderAutomationHTML(pageTemplate, spec)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to render automation API page: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(page)
	}
}
