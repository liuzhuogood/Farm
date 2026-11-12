package automation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Element 代表屏幕上的单个 Accessibility 控件
type Element struct {
	Ref        int    `json:"ref"`
	Role       string `json:"role"`  // button, input, text, switch, checkbox, radio, icon, item...
	Label      string `json:"label"` // 文本内容、说明或标识
	CenterX    int    `json:"center_x"`
	CenterY    int    `json:"center_y"`
	Bounds     [4]int `json:"bounds"` // left, top, right, bottom
	ResourceID string `json:"resource_id,omitempty"`
	Class      string `json:"class,omitempty"`
	Package    string `json:"package,omitempty"`
	Focused    bool   `json:"focused,omitempty"`
	Editable   bool   `json:"editable,omitempty"`
	Checked    bool   `json:"checked,omitempty"`
	Disabled   bool   `json:"disabled,omitempty"`
	Selected   bool   `json:"selected,omitempty"`
	Scrollable bool   `json:"scrollable,omitempty"`
	Password   bool   `json:"password,omitempty"`
}

// OCRItem 代表 OCR 识别出的视觉文本项
type OCRItem struct {
	Text       string  `json:"text"`
	CenterX    int     `json:"center_x"`
	CenterY    int     `json:"center_y"`
	Bounds     [4]int  `json:"bounds"` // left, top, right, bottom
	Confidence float64 `json:"confidence"`
}

// Event 代表临时发生的系统事件（如 Toast 或通知）
type Event struct {
	Type    string `json:"type"`             // "toast", "notification"
	Sender  string `json:"sender,omitempty"` // WeChat, etc.
	Message string `json:"message"`
}

// Screen 代表一屏 UI 的感知快照
type Screen struct {
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	AppName         string    `json:"app_name,omitempty"`
	PackageName     string    `json:"package_name,omitempty"`
	Activity        string    `json:"activity,omitempty"`
	Orientation     string    `json:"orientation"` // "portrait" or "landscape"
	KeyboardVisible bool      `json:"keyboard_visible"`
	FocusRef        int       `json:"focus_ref,omitempty"`
	Events          []Event   `json:"events,omitempty"`
	Elements        []Element `json:"elements"`
	OCR             []OCRItem `json:"ocr,omitempty"`
	RawXML          string    `json:"raw_xml,omitempty"`
	CapturedAt      time.Time `json:"captured_at"`
}

// escapeLabel 对 label 中的反斜杠、双引号及换行符进行转义
func escapeLabel(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch b {
		case '\\':
			buf.WriteString(`\\`)
		case '"':
			buf.WriteString(`\"`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			buf.WriteByte(b)
		}
	}
	return buf.String()
}

// unescapeLabel 还原被转义的 label 文本
func unescapeLabel(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case '"':
				buf.WriteByte('"')
			case '\\':
				buf.WriteByte('\\')
			default:
				buf.WriteByte('\\')
				buf.WriteByte(s[i])
			}
		} else {
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

// Line 将 Element 格式化为行协议字符串:
// [{ref}] {role} "{label}" @{centerX},{centerY} {states...}
func (e *Element) Line() string {
	role := e.Role
	if role == "" {
		role = "item"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "[%d] %s \"%s\" @%d,%d", e.Ref, role, escapeLabel(e.Label), e.CenterX, e.CenterY)

	// 状态按固定顺序追加
	if e.Focused {
		sb.WriteString(" focused")
	}
	if e.Editable {
		sb.WriteString(" editable")
	}
	if e.Checked {
		sb.WriteString(" checked")
	}
	if e.Disabled {
		sb.WriteString(" disabled")
	}
	if e.Selected {
		sb.WriteString(" selected")
	}
	if e.Scrollable {
		sb.WriteString(" scrollable")
	}
	if e.Password {
		sb.WriteString(" password")
	}

	return sb.String()
}

// Line 将 OCRItem 格式化为行协议字符串:
// ocr "{text}" @{centerX},{centerY}
func (o *OCRItem) Line() string {
	return fmt.Sprintf("ocr \"%s\" @%d,%d", escapeLabel(o.Text), o.CenterX, o.CenterY)
}

// Line 将 Event 格式化为行协议字符串
func (ev *Event) Line() string {
	switch ev.Type {
	case "toast":
		return fmt.Sprintf("toast: \"%s\"", escapeLabel(ev.Message))
	case "notification":
		if ev.Sender != "" {
			return fmt.Sprintf("notification: %s · \"%s\"", ev.Sender, escapeLabel(ev.Message))
		}
		return fmt.Sprintf("notification: \"%s\"", escapeLabel(ev.Message))
	default:
		if ev.Sender != "" {
			return fmt.Sprintf("%s: %s · \"%s\"", ev.Type, ev.Sender, escapeLabel(ev.Message))
		}
		return fmt.Sprintf("%s: \"%s\"", ev.Type, escapeLabel(ev.Message))
	}
}

// MetadataLine 格式化 Screen 的首行元数据:
// screen 1080x2400 · app: Settings (com.android.settings) · portrait · keyboard hidden · focus [3]
func (s *Screen) MetadataLine() string {
	var parts []string

	// 1. screen WxH
	w := s.Width
	h := s.Height
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 2400
	}
	parts = append(parts, fmt.Sprintf("screen %dx%d", w, h))

	// 2. app
	appName := s.AppName
	pkg := s.PackageName
	if appName != "" && pkg != "" && appName != pkg {
		parts = append(parts, fmt.Sprintf("app: %s (%s)", appName, pkg))
	} else if appName != "" {
		parts = append(parts, fmt.Sprintf("app: %s", appName))
	} else if pkg != "" {
		parts = append(parts, fmt.Sprintf("app: %s", pkg))
	} else {
		parts = append(parts, "app: unknown")
	}

	// 3. orientation
	orient := s.Orientation
	if orient == "" {
		if w <= h {
			orient = "portrait"
		} else {
			orient = "landscape"
		}
	}
	parts = append(parts, orient)

	// 4. keyboard
	if s.KeyboardVisible {
		parts = append(parts, "keyboard visible")
	} else {
		parts = append(parts, "keyboard hidden")
	}

	// 5. focus
	if s.FocusRef > 0 {
		parts = append(parts, fmt.Sprintf("focus [%d]", s.FocusRef))
	} else {
		parts = append(parts, "focus none")
	}

	return strings.Join(parts, " · ")
}

// Render 将全量 Screen 按选定格式序列化为纯文本
func (s *Screen) Render(formats []string) string {
	includeRef, includeOCR := parseFormats(formats)

	var lines []string

	// 首行元数据
	lines = append(lines, s.MetadataLine())

	// 事件行
	for _, ev := range s.Events {
		lines = append(lines, ev.Line())
	}

	// 原生组件行
	if includeRef {
		for i := range s.Elements {
			lines = append(lines, s.Elements[i].Line())
		}
	}

	// OCR 补充行
	if includeOCR {
		for i := range s.OCR {
			lines = append(lines, s.OCR[i].Line())
		}
	}

	return strings.Join(lines, "\n")
}

// parseFormats 判定是否包含 ref 与 ocr 模式
func parseFormats(formats []string) (bool, bool) {
	if len(formats) == 0 {
		return true, true // 默认同时包含 ref 与 ocr
	}
	includeRef := false
	includeOCR := false
	for _, f := range formats {
		lower := strings.ToLower(strings.TrimSpace(f))
		if lower == "ref" {
			includeRef = true
		} else if lower == "ocr" {
			includeOCR = true
		}
	}
	return includeRef, includeOCR
}

var (
	elementLinePattern = regexp.MustCompile(`^\[(\d+)\]\s+([a-zA-Z0-9_-]+)\s+"((?:[^"\\]|\\.)*)"\s+@(-?\d+),(-?\d+)(?:\s+(.*))?$`)
	ocrLinePattern     = regexp.MustCompile(`^ocr\s+"((?:[^"\\]|\\.)*)"\s+@(-?\d+),(-?\d+)$`)
	toastLinePattern   = regexp.MustCompile(`^toast:\s+"((?:[^"\\]|\\.)*)"$`)
	notifyLinePattern  = regexp.MustCompile(`^notification:\s+(?:(.*?)\s+·\s+)?"((?:[^"\\]|\\.)*)"$`)
	metaLinePattern    = regexp.MustCompile(`^screen\s+(\d+)x(\d+)\s+·\s+app:\s+(.*?)\s+·\s+(portrait|landscape)\s+·\s+keyboard\s+(hidden|visible)\s+·\s+focus\s+(\S+)$`)
)

// ParseElementLine 解析单行 Element 字符串
func ParseElementLine(line string) (*Element, error) {
	matches := elementLinePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) < 6 {
		return nil, fmt.Errorf("invalid element line format: %q", line)
	}

	ref, _ := strconv.Atoi(matches[1])
	role := matches[2]
	label := unescapeLabel(matches[3])
	cx, _ := strconv.Atoi(matches[4])
	cy, _ := strconv.Atoi(matches[5])

	elem := &Element{
		Ref:     ref,
		Role:    role,
		Label:   label,
		CenterX: cx,
		CenterY: cy,
	}

	if len(matches) >= 7 && matches[6] != "" {
		states := strings.Fields(matches[6])
		for _, s := range states {
			switch s {
			case "focused":
				elem.Focused = true
			case "editable":
				elem.Editable = true
			case "checked":
				elem.Checked = true
			case "disabled":
				elem.Disabled = true
			case "selected":
				elem.Selected = true
			case "scrollable":
				elem.Scrollable = true
			case "password":
				elem.Password = true
			}
		}
	}

	return elem, nil
}

// ParseOCRLine 解析单行 OCR 文本字符串
func ParseOCRLine(line string) (*OCRItem, error) {
	matches := ocrLinePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) < 4 {
		return nil, fmt.Errorf("invalid ocr line format: %q", line)
	}

	text := unescapeLabel(matches[1])
	cx, _ := strconv.Atoi(matches[2])
	cy, _ := strconv.Atoi(matches[3])

	return &OCRItem{
		Text:    text,
		CenterX: cx,
		CenterY: cy,
	}, nil
}

// ParseEventLine 解析单行事件通告字符串
func ParseEventLine(line string) (*Event, error) {
	trimmed := strings.TrimSpace(line)
	if m := toastLinePattern.FindStringSubmatch(trimmed); len(m) >= 2 {
		return &Event{
			Type:    "toast",
			Message: unescapeLabel(m[1]),
		}, nil
	}
	if m := notifyLinePattern.FindStringSubmatch(trimmed); len(m) >= 3 {
		return &Event{
			Type:    "notification",
			Sender:  strings.TrimSpace(m[1]),
			Message: unescapeLabel(m[2]),
		}, nil
	}
	return nil, fmt.Errorf("unrecognized event line: %q", line)
}

// ParseMetadataLine 解析 Screen 首行元数据字符串
func ParseMetadataLine(line string) (width int, height int, appName string, pkg string, orient string, keyboardVisible bool, focusRef int, err error) {
	m := metaLinePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(m) < 6 {
		err = fmt.Errorf("invalid metadata line format: %q", line)
		return
	}

	width, _ = strconv.Atoi(m[1])
	height, _ = strconv.Atoi(m[2])

	rawApp := m[3]
	if strings.HasSuffix(rawApp, ")") && strings.Contains(rawApp, " (") {
		idx := strings.LastIndex(rawApp, " (")
		appName = strings.TrimSpace(rawApp[:idx])
		pkg = strings.TrimSuffix(strings.TrimSpace(rawApp[idx+2:]), ")")
	} else if rawApp != "unknown" {
		pkg = rawApp
	}

	orient = m[4]
	keyboardVisible = m[5] == "visible"

	focusStr := m[6]
	if strings.HasPrefix(focusStr, "[") && strings.HasSuffix(focusStr, "]") {
		focusRef, _ = strconv.Atoi(strings.Trim(focusStr, "[]"))
	}

	return
}
