package automation

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RawNode 代表 XML 反序列化时的原始节点
type RawNode struct {
	Index       int        `xml:"index,attr"`
	Editable    string     `xml:"editable,attr"`
	Text        string     `xml:"text,attr"`
	ResourceID  string     `xml:"resource-id,attr"`
	Class       string     `xml:"class,attr"`
	Package     string     `xml:"package,attr"`
	ContentDesc string     `xml:"content-desc,attr"`
	Checkable   string     `xml:"checkable,attr"`
	Checked     string     `xml:"checked,attr"`
	Clickable   string     `xml:"clickable,attr"`
	Enabled     string     `xml:"enabled,attr"`
	Focusable   string     `xml:"focusable,attr"`
	Focused     string     `xml:"focused,attr"`
	Scrollable  string     `xml:"scrollable,attr"`
	LongClick   string     `xml:"long-clickable,attr"`
	Password    string     `xml:"password,attr"`
	Selected    string     `xml:"selected,attr"`
	Bounds      string     `xml:"bounds,attr"`
	Children    []*RawNode `xml:"node"`
}

// HierarchyRoot 代表 hierarchy 根节点
type HierarchyRoot struct {
	XMLName  xml.Name   `xml:"hierarchy"`
	Rotation int        `xml:"rotation,attr"`
	Nodes    []*RawNode `xml:"node"`
}

var boundsRegex = regexp.MustCompile(`\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]`)

func parseBounds(boundsStr string) (left, top, right, bottom int, ok bool) {
	m := boundsRegex.FindStringSubmatch(boundsStr)
	if len(m) != 5 {
		return 0, 0, 0, 0, false
	}
	left, _ = strconv.Atoi(m[1])
	top, _ = strconv.Atoi(m[2])
	right, _ = strconv.Atoi(m[3])
	bottom, _ = strconv.Atoi(m[4])
	return left, top, right, bottom, true
}

func parseBoolAttr(val string) bool {
	return strings.EqualFold(val, "true")
}

var genericContainers = []string{
	"viewgroup", "linearlayout", "framelayout", "relativelayout",
	"coordinatorlayout", "constraintlayout", "drawerlayout",
	"recyclerview", "listview", "scrollview", "viewpager",
}

func isGenericContainerClass(cls string) bool {
	lower := strings.ToLower(cls)
	for _, gc := range genericContainers {
		if strings.Contains(lower, gc) {
			return true
		}
	}
	return false
}

// resolveRole 根据节点属性和 class 推断控件语义角色
func resolveRole(node *RawNode) string {
	cls := strings.ToLower(node.Class)
	switch {
	case parseBoolAttr(node.Editable):
		return "input"
	case strings.Contains(cls, "edittext") || strings.Contains(cls, "input"):
		return "input"
	case strings.Contains(cls, "switch") || strings.Contains(cls, "toggle"):
		return "switch"
	case strings.Contains(cls, "checkbox"):
		return "checkbox"
	case strings.Contains(cls, "radiobutton") || strings.Contains(cls, "radio"):
		return "radio"
	case strings.Contains(cls, "button"):
		return "button"
	case strings.Contains(cls, "imageview") || strings.Contains(cls, "image"):
		if parseBoolAttr(node.Clickable) {
			return "button"
		}
		return "icon"
	case strings.Contains(cls, "textview"):
		if parseBoolAttr(node.Clickable) {
			return "button"
		}
		return "text"
	case strings.Contains(cls, "seekbar") || strings.Contains(cls, "progressbar"):
		return "slider"
	default:
		if parseBoolAttr(node.Clickable) {
			return "button"
		}
		if parseBoolAttr(node.Checkable) {
			return "checkbox"
		}
		if strings.TrimSpace(node.Text) != "" {
			return "text"
		}
		return "item"
	}
}

// collectChildText 递归提取子节点的所有非空文本
func collectChildText(node *RawNode) []string {
	var texts []string
	for _, child := range node.Children {
		if t := strings.TrimSpace(child.Text); t != "" {
			texts = append(texts, t)
		} else if d := strings.TrimSpace(child.ContentDesc); d != "" {
			texts = append(texts, d)
		}
		texts = append(texts, collectChildText(child)...)
	}
	return texts
}

// resolveLabel 计算节点的描述标签
func resolveLabel(node *RawNode) string {
	if t := strings.TrimSpace(node.Text); t != "" {
		return t
	}
	if d := strings.TrimSpace(node.ContentDesc); d != "" {
		return d
	}
	cls := strings.ToLower(node.Class)
	isInput := parseBoolAttr(node.Editable) || strings.Contains(cls, "edittext") || strings.Contains(cls, "input")
	// 如果是可点击容器或输入框但本身无文字，聚合子树文本
	if parseBoolAttr(node.Clickable) || isInput {
		childTexts := collectChildText(node)
		if len(childTexts) > 0 {
			return strings.Join(childTexts, " ")
		}
	}
	if id := strings.TrimSpace(node.ResourceID); id != "" && !isGenericContainerClass(node.Class) {
		if idx := strings.LastIndex(id, "/"); idx >= 0 && idx+1 < len(id) {
			return id[idx+1:]
		}
		return id
	}
	return ""
}

// isMeaningful 检查节点是否应纳入紧凑感知协议
func isMeaningful(node *RawNode, label string, w, h int) bool {
	// 尺寸过小或完全不可见的容器丢弃
	if w <= 0 || h <= 0 {
		return false
	}

	cls := strings.ToLower(node.Class)
	interactive := parseBoolAttr(node.Editable) || parseBoolAttr(node.Clickable) ||
		parseBoolAttr(node.Checkable) ||
		parseBoolAttr(node.Scrollable) ||
		parseBoolAttr(node.LongClick) ||
		parseBoolAttr(node.Focused) ||
		strings.Contains(cls, "edittext") ||
		strings.Contains(cls, "switch") ||
		strings.Contains(cls, "checkbox") ||
		strings.Contains(cls, "radio") ||
		strings.Contains(cls, "button")

	// 纯布局容器若非交互型则忽略
	if isGenericContainerClass(cls) && !interactive {
		return false
	}

	if interactive {
		return true
	}

	// 具备可见文字或描述的节点有意义
	if strings.TrimSpace(node.Text) != "" || strings.TrimSpace(node.ContentDesc) != "" {
		return true
	}

	return false
}

// ParseHierarchyXML 解析 Accessibility XML 快照，提取紧凑元素列表与首个焦点 ref
func ParseHierarchyXML(xmlData []byte, displayWidth, displayHeight int) (elements []Element, focusRef int, err error) {
	if len(xmlData) == 0 {
		return nil, 0, nil
	}

	var root HierarchyRoot
	if err := xml.Unmarshal(xmlData, &root); err != nil {
		// 某些 dump 可能直接以 <node> 为根
		var singleNode RawNode
		if err2 := xml.Unmarshal(xmlData, &singleNode); err2 == nil {
			root.Nodes = []*RawNode{&singleNode}
		} else {
			return nil, 0, fmt.Errorf("parse hierarchy xml: %w", err)
		}
	}

	refCounter := 1
	elements = make([]Element, 0, 32)

	var walk func(node *RawNode, insideInput bool)
	walk = func(node *RawNode, insideInput bool) {
		if node == nil {
			return
		}

		l, t, r, b, ok := parseBounds(node.Bounds)
		if ok {
			w := r - l
			h := b - t

			// 剔除屏幕外元素（若指定了分辨率）
			inScreen := true
			if displayWidth > 0 && displayHeight > 0 {
				if r <= 0 || b <= 0 || l >= displayWidth || t >= displayHeight {
					inScreen = false
				}
			}

			cls := strings.ToLower(node.Class)
			isInput := parseBoolAttr(node.Editable) || strings.Contains(cls, "edittext") || strings.Contains(cls, "input")

			if inScreen && (!insideInput || isInput) {
				label := resolveLabel(node)

				if isMeaningful(node, label, w, h) {
					// 避免父子元素在完全相同坐标且标签相同时产生重复行
					isDuplicate := false
					if len(elements) > 0 {
						last := elements[len(elements)-1]
						if last.Bounds[0] == l && last.Bounds[1] == t && last.Bounds[2] == r && last.Bounds[3] == b && last.Label == label {
							isDuplicate = true
						}
					}

					if !isDuplicate {
						elem := Element{
							Ref:        refCounter,
							Role:       resolveRole(node),
							Label:      label,
							CenterX:    (l + r) / 2,
							CenterY:    (t + b) / 2,
							Bounds:     [4]int{l, t, r, b},
							ResourceID: node.ResourceID,
							Class:      node.Class,
							Package:    node.Package,
							Focused:    parseBoolAttr(node.Focused),
							Checked:    parseBoolAttr(node.Checked),
							Scrollable: parseBoolAttr(node.Scrollable),
							Password:   parseBoolAttr(node.Password),
							Selected:   parseBoolAttr(node.Selected),
						}

						// enabled 默认 true，若为 false 标记 disabled
						if strings.EqualFold(node.Enabled, "false") {
							elem.Disabled = true
						}

						// editable: EditText 或 input 角色
						if node.Editable != "" {
							// Companion supplies the actual AccessibilityNodeInfo state.
							elem.Editable = parseBoolAttr(node.Editable)
						} else if isInput || elem.Role == "input" {
							// UIAutomator XML has no editable attribute; retain its class mapping.
							elem.Editable = true
						}

						if elem.Focused && focusRef == 0 {
							focusRef = elem.Ref
						}

						elements = append(elements, elem)
						refCounter++
					}
				}
			}

			for _, child := range node.Children {
				walk(child, insideInput || isInput)
			}
			return
		}

		for _, child := range node.Children {
			walk(child, insideInput)
		}
	}

	for _, node := range root.Nodes {
		walk(node, false)
	}

	return elements, focusRef, nil
}
