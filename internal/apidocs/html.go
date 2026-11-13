package apidocs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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

// RenderAutomationHTML emits a compact Markdown-like contract in a minimal HTML
// shell. The skill uses concise text examples instead of exposing JSON schemas.
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
	md.WriteString("用于读取 Android 页面并操作设备。使用顺序：选择设备 → 开始任务 → 观察与多次操作 → 核对结果 → 结束任务。操作和观察使用简短文本；最后帧返回图片，产物目录返回 HTML。\n本页示例仅演示格式：serial、坐标和文字须换成当前设备观察到的实际值。\n\n")
	sections := []struct{ path, method, body string }{
		{"/api/automation/devices", "get", `## 1. 选择设备

GET /api/automation/devices

一行一台在线设备，例如：
设备 "Redmi" · serial "10.0.0.30:5555" · 在线

后续请求的 serial 填这一行的真实序列号。没有在线设备时会明确提示。
`},
		{"/api/automation/tasks", "post", `## 开始、查看和结束完整任务

先由 AI 生成任务标题，再请求（正文留空）：
POST /api/automation/tasks?serial=10.0.0.30:5555&title=检查设置

成功返回 201，JSON 的 task.id 是后续操作使用的 task_id；task.directory 是建议的设备相对目录，不会自动创建。
将一次完整目标的多次 act 归入同一 task_id，期间可多次 observe；act 成功只表示这一批执行完成，完整任务仍等待后续操作。
查看设备任务：GET /api/automation/tasks?serial=10.0.0.30:5555
查看单个任务：GET /api/automation/tasks/任务id?serial=10.0.0.30:5555
列表返回 JSON tasks 数组；详情返回 JSON task 对象。包含标题、状态、步骤、执行结果、错误、设备产物 URL 和结束摘要。
active=true 表示正在执行 act；state=running 且 active=false 表示等待下一批操作。步骤数是已提交的步骤数，不是预先知道的完整任务进度百分比。

核对业务结果后明确结束：
POST /api/automation/tasks/任务id/finish?serial=10.0.0.30:5555
正文：{"outcome":"done","summary":"已核对实际页面与结果"}

outcome 为 done、failed 或 stopped；summary 最多 4096 字节。done/failed 需等待当前 act 完成；stopped 会取消当前操作并等待本次清理（最多等待 5 秒，超时需重新查询）。结束后不可再追加 act。
步骤失败后先观察并分析；任务仍可提交修正后的下一批，或明确以 failed 结束，不盲目重放已执行步骤。
任务最多保留 500 步，运行中任务最多 16 个、已结束最多 256 个，并受元数据容量约束；达到容量返回明确错误。重启会将中断任务标为已停止，设备产物保留。
MCP 来源任务以现有审计记录只读展示，不能通过本接口结束；列表若 steps_truncated=true，用详情接口获取较早步骤。
`},
		{"/api/automation/observe", "get", `## 2. 观察页面

GET /api/automation/observe?serial=10.0.0.30:5555&format=text

serial：必填，设备序列号。
formats：可选，ref,ocr（默认）、ref 或 ocr。
diff：可选，默认 true，返回相对上次观察的变化；false 返回全量。
settle_ms：可选，观察前等待毫秒数，默认 300。
format=text：包含出错信息在内均返回文本。

返回示例：
screen 1080x2400 · app: Settings (com.android.settings)
[1] button "返回" @60,120
[2] input "搜索设置" @540,240 editable
ocr "WLAN" @160,420

@X,Y 是当前屏幕像素坐标，可直接交给 tap 或 swipe。
ref 优先读取已启用的 Farm Companion，失败时回退 uiautomator dump；仅 ocr 跳过控件树。树采集或解析失败会明确报错；默认 ref,ocr 在 OCR 不可用时保留树文字并记录原因，formats=ocr 时识别失败明确报错。
`},
		{"/api/automation/act", "post", `## 3. 执行操作

POST /api/automation/act?serial=10.0.0.30:5555&title=检查设置&task_id=开始任务返回的id

serial：必填，设备序列号。
title：必填，由调用的 AI 根据完整任务生成简短任务标题，例如“检查设置”或“核对钉钉下班打卡”，不要求用户代填。
标题去除首尾空格后为 1..120 个字符，不允许控制字符或重复 title 参数；缺少或无效会在执行任何操作前拒绝。
task_id：完整任务必传开始任务时返回的 id；同一任务的多次 act 必须使用相同 serial、title 和 task_id，步骤序号持续累积。
未传 task_id 的旧调用仍作为独立批次自动开始并结束；设备已有未结束任务时拒绝此类调用。每台设备只允许一个未结束的自动化任务。
标题放查询参数，中文、空格和特殊字符需 URL 编码；curl 可用 --url-query 'title=检查设置'。
正文：每行写“操作 参数”，支持一次提交多行，按从上到下顺序执行。

点击：        tap @540,960
系统按键：    keyevent home｜back,app_switch
输入文字：    sendkey 你好 Farm
设置剪贴板：  set-copy 你好 Farm
复制选中文字：copy
粘贴到输入框：paste
清空输入框：  clear
滑动：        swipe @540,1800 @540,500 300
检查文字：    check 设置
等待文字：    wait 10000 WLAN
固定延时：    sleep 500
ADB 命令：    adb shell getprop ro.product.model
新建目录：    mkdir task-output
截图保存：    screenshot task-output
录屏保存：    record 5 task-output
删除产物：    rm task-output

点击和滑动使用屏幕像素坐标。滑动末尾为可选毫秒时长，默认 300，范围 80..10000。
keyevent 支持 |、｜、英文逗号或中文逗号分隔，最多 32 项；先校验全部按键，再按书写顺序执行。
按键名：home、back、app_switch、power、volume_up、volume_down、enter、delete。
sendkey 将本行剩余原文输入当前焦点，保留空格和标点；先确认目标输入框。
set-copy 将本行剩余原文写入手机剪贴板，保留空格和标点，不触发粘贴。
copy、paste 均不带参数：copy 复制手机上已选中的文字到手机剪贴板；paste 将手机剪贴板内容粘贴到当前输入框。
复制前须选中文字，粘贴前须聚焦目标输入框；执行后通过 observe 核对选区或输入结果。
clear 不带参数，仅清空当前聚焦且可编辑的输入框，不改变手机剪贴板；没有有效输入框时明确报错。
读取页面文字统一调用 observe。
流程建议：点击等页面变化后，优先在同一批 act 中用 wait/check 判断页面是否就绪或目标状态是否出现，减少固定等待和逐步截图的往返。页面文字可确认结果时优先使用 observe；只有文字不足以判断、需要查看视觉布局或出现异常时再截图。按页面实际情况使用 sleep，不设固定次数或时长限制。
check 按默认 observe（ref,ocr）能读到的页面文字做一次包含检查，覆盖控件树文字和 OCR 文字；任一可见文字项包含关键词就命中。
wait 的第一个参数是最长等待毫秒数（1..30000），后面是等待出现的文字；每轮重新观察，使用与 check 相同的文字范围。
检查使用本轮完整观察结果，不受 diff 影响，不搜索页面元数据、坐标或原始 XML 属性。
sleep 是固定毫秒延时，范围 0..30000。
adb 后填写子命令和参数，不重复 adb 或设备选择参数；由 serial 指定设备，不通过电脑 Shell 执行。
参数支持单/双引号及反斜杠转义，例如 adb shell am start -n com.android.settings/.Settings；输出最多返回 1 MiB，超出会标记截断，非零退出立即停止后续操作。
mkdir、screenshot、record 的目录均为目标 Android 设备上的相对目录，统一位于 /sdcard/Farm/automation 下；例如 task-output 对应设备 /sdcard/Farm/automation/task-output。
目录禁止绝对路径及 .. 越界；本行剩余文本作为目录，保留目录中的空格，不加引号。产物保留在该设备，不自动导出或删除成功文件。
mkdir 递归创建设备目录；截图和录屏要求目录已存在，自动生成不重复文件名，成功返回设备相对路径和含 serial 的目录链接。
rm 相对路径 删除该设备产物根目录内指定的文件或目录（目录递归删除）；禁止删除根目录本身，禁止绝对路径、.. 越界及符号链接，目标不存在明确报错。
screenshot 与最后帧接口使用同一取帧来源：优先复用视频流最后解码帧，无可用帧时沿用现有截图回退。
原 JPEG 或 PNG 字节保留不变，按实际格式保存为 .jpg 或 .png，不额外转为 PNG；上限 20 MiB。
最后帧可能滞后，不保证对应刚完成的操作；截图前先通过 observe 或 wait 确认业务状态。
record 的第一个参数是录制秒数（1..30），录制无音频 MP4，完成后保留在指定设备目录并返回，文件上限 32 MiB；录屏前确认设备亮屏，零时长或无有效视频会明确失败。
录制、导出或校验失败不会发布最终文件；取消时停止本次录制并清理临时文件。
每次最多 32 条操作，总时限 60 秒（含录制和导出）；请求取消时停止。

返回示例：
设备 "10.0.0.30:5555"
状态 已完成
完成 1/1
第 1 行 keyevent：已完成（2/2）

全部命令通过格式校验后才开始执行。操作失败、check 未命中或 wait 超时会停止后续步骤，返回失败行与已完成数量；不要盲目重试整串。
执行完成仍须重新观察页面，确认实际结果。

### 单步示例：点击刚刚观察到的控件

请求地址：POST /api/automation/act?serial=10.0.0.30:5555&title=检查设置&task_id=开始任务返回的id
正文：
tap @540,960

然后再次 GET /api/automation/observe?serial=10.0.0.30:5555&format=text 核对页面。

### 多行示例：点击后等待并检查

请求地址：POST /api/automation/act?serial=10.0.0.30:5555&title=检查设置&task_id=开始任务返回的id
正文：
tap @540,960
sleep 500
wait 10000 设置
check WLAN

先点击，暂停半秒，最多等待 10 秒直到“设置”出现，再立即检查“WLAN”。任一步失败即停止。

用 curl 发送这段正文（请替换为实际设备和坐标）：
curl --url-query 'title=检查设置' --url-query 'task_id=开始任务返回的id' --data-binary @- 'http://127.0.0.1:1989/api/automation/act?serial=10.0.0.30:5555' <<'COMMANDS'
tap @540,960
sleep 500
wait 10000 设置
check WLAN
COMMANDS


### 输入示例：聚焦输入框，再发送文字

先观察页面，确认输入框位于 @540,240。
请求地址：POST /api/automation/act?serial=10.0.0.30:5555&title=检查设置&task_id=开始任务返回的id
正文：
tap @540,240
sleep 500
sendkey 测试内容
keyevent enter

提交输入前必须已获得该业务操作的授权。成功后重新观察，核对输入或搜索结果。

### 产物示例：新建目录、截图和录屏

正文：
mkdir task-output
screenshot task-output
record 5 task-output

每个成功指令返回保存路径；截图和录屏后，可访问返回的目录链接查看和下载产物。

### 失败示例：检查未命中

正文：
check 不存在的目标文字
keyevent back

返回示例：
设备 "10.0.0.30:5555"
状态 失败
完成 0/2
第 1 行 check：未命中（0/1）
失败行 1
错误 check_not_matched: "未找到目标文字"

back 不会执行。外层完成数按操作行计数；括号中的数量表示该行已完成的子操作（如多个按键）。
先重新观察定位原因，再决定下一步，不要把整段操作直接重放。
`},
		{"/api/automation/logs", "get", `## 4. 查看调用记录

GET /api/automation/logs?format=text&serial=10.0.0.30:5555&limit=20

format=text：逐行返回文本。
serial：可选，只看指定设备。
limit：可选，默认 100，最多 1024。
type：可选，按事件类型过滤，例如 automation.error。

每条 act 任务日志包含 title，fields.task_id 对应完整任务，fields.step_index 对应步骤（有步骤上下文时）；可将动作、检查过程和最终成功/失败对应到 AI 生成的任务标题；独立 observe 等非任务日志可没有 title。
format=text 示例：
2026-11-13T19:10:00+08:00 · 设备 "10.0.0.30:5555" · title="检查设置" · automation.response · "统一自动化批次完成"

最近记录在前；没有记录时明确提示。
`},
		{"/api/v1/vision/latest-frame", "get", `## 5. 获取最后帧图片

GET /api/v1/vision/latest-frame?serial=10.0.0.30:5555
POST /api/v1/vision/latest-frame?serial=10.0.0.30:5555

两种方法均直接返回图片二进制流，POST 无需正文；serial 必填。
图片格式由响应头标识，为 JPEG 或 PNG，可直接展示或保存。
来源为当前视频流最后解码帧；无可用帧时按现有取帧逻辑截图，失败明确返回错误。最后帧不保证对应刚发生的操作，检查业务结果仍需重新观察。

下载示例：
curl -fS 'http://127.0.0.1:1989/api/v1/vision/latest-frame?serial=10.0.0.30:5555' -o frame.img
curl -fS -X POST 'http://127.0.0.1:1989/api/v1/vision/latest-frame?serial=10.0.0.30:5555' -o frame.img
`},
		{"/api/automation/artifacts", "get", `## 6. 查看和下载目录产物

GET /api/automation/artifacts?serial=10.0.0.30:5555&directory=task-output

serial 必填，选择产物所在的在线设备；directory 必填，是该设备 /sdcard/Farm/automation 下的相对目录。不同设备可使用相同相对目录，各自读取各自文件。
含空格或特殊字符时按 URL 查询参数编码。
返回简单 HTML，列出文件名称、大小和每个文件的完整下载 URL，可供浏览器或 AI 直接读取。
递归列出子目录文件；隐藏临时文件，不跟随符号链接；最多显示 1000 个文件，达到上限时页面明确提示。单个文件下载最大 32 MiB。

直接下载单个文件：
GET /api/automation/artifacts/download?serial=10.0.0.30:5555&directory=task-output&file=实际文件名.png

file 为目录内相对路径，使用页面给出的 URL；从指定设备读取并返回文件附件下载流，路径越界、文件不存在或设备离线会明确失败。
`},
	}
	for _, section := range sections {
		if _, ok := spec.Paths[section.path][section.method]; ok {
			md.WriteString(section.body + "\n")
		}
	}
	content := escapePreText(md.String()) + "\n"
	return bytes.Replace(pageTemplate, []byte(automationMarkdownMarker), []byte(content), 1), nil
}

func escapePreText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
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
