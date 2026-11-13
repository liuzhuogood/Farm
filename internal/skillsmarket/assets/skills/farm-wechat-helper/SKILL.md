---
name: farm-wechat-helper
description: 微信联系人与会话检索、未读消息读取与上下文草拟回复辅助。仅读取与草拟，绝不自动外发任何消息。
---

# 微信消息助手 (WeChat Helper)

协助在手机微信中检索用户指定的联系人或群组会话，读取最新文本消息，并在 AI 助手内草拟回复。

## 核心安全准则（强制）

> 本技能仅读取与草拟，**绝不自动向外发送任何消息**。回复草稿只在 AI 助手内展示，必须由用户审阅并在手机微信内手动发送。严禁注入回复到聊天输入框，严禁任何按键、系统事件、手势或模拟点击触发“发送”，严禁发送回车键（keycode 66）或 `event:"enter"` 触发外发。

## 核心原则

每次设备动作采用闭环：`observe(ref,ocr) → 必要时 observe(xml) → 再 observe(ref,ocr) 核对上下文 → 单次动作 → 再 observe(ref,ocr) 验证业务结果`。每次明确设置 `diff:false` 获取全量。核对前后包名、页面标题、目标节点与屏幕方向；状态不稳定则最多重采 3 次，仍不稳定则停止。XML 节点必须唯一、启用、可操作且 bounds 在当前可视区域内；OCR 文字或 ref 编号本身不能证明节点可点击。紧凑协议首行不含 Activity，不得猜测 Activity 或不存在的字段；页面无法可靠识别时停止。

1. **地址与设备**：必须从环境变量 `FARM_API_URL` 或用户指令读取已确认的服务地址；用户提供地址后将其导出为 `FARM_API_URL`。缺失即停止确认，严禁回退到 `http://127.0.0.1:8080` 或任何默认端口。通过 `GET /api/devices` 核实目标 `serial` 的 `state == "device"`、`screen_on == true`、`screen_locked == false`；缺失、不就绪则停止。沿用用户明确指定的 serial，否则先向用户确认，禁止自动选第一台设备。
2. **契约来源**：先读取已确认的 Agent 侧服务地址 + `/fram.skill`，核对路径、参数、响应及认证约定，不要求读取完整 OpenAPI。只按已确认字段取值；字段缺失或契约不一致则停止报告，不猜测字段、不切回旧接口。需要认证时，按已确认配置使用 `FARM_API_TOKEN`、`FARM_API_TOKEN_HEADER` 和可选 `FARM_API_TOKEN_PREFIX`，禁止猜测认证头或打印凭据。
3. **坐标与模式**：`GET /api/v1/device-status?serial=...` 的 `status` 必须为 `ready`、`serial` 必须匹配；读取实际 `width`、`height`、`touch_mode`，宽高必须为正且与当前 observe 的 `screen WxH` 及方向一致。observe 的 `@X,Y` 是屏幕像素，当前行协议没有归一化坐标。点击转换为 `x = X / width`、`y = Y / height`；XML bounds 中心为 `X = (left + right) / 2`、`Y = (top + bottom) / 2`，滑动两端分别同样转换。先检查像素在屏幕内及归一化值在 0~1 内；禁止硬编码绝对像素或把像素直接传给 tap/swipe。旋转、尺寸或页面变化后重新采集，不复用旧坐标；未明确要求时省略 `mode` 以继承系统点击模式。
4. **执行结果与重试**：动作响应必须同时满足 `status == "executed"` 和 `executed == true`，仍需后续 observe 核对预期业务变化；不能把 HTTP 200、画面任意变动或动作自述当作成功。单次 HTTP 超时 30 秒；后置验证每隔 1 秒重采，最多 3 次，超时明确退出。动作失败或超时先 observe 核实是否已生效，再决定是否重试；包括首次在内动作尝试最多 3 次，每次重新定位，禁止盲目重放。敏感操作不得自动重试。记录 serial、步骤、request_id（响应提供时）、执行结果及前后页面证据，异常保留原始响应，日志不得泄露凭据和私人消息。
5. **请求工具**：所有请求使用当前技能目录内的 `scripts/request_client.py`，无需额外依赖。POST 的 serial 放在 JSON body；device-status 的 serial 放在 query。observe 的 `formats:["ref","ocr"]` 返回 `text/plain` 紧凑行协议，`formats:["xml"]` 返回 `text/xml` 原始层次树，均不可按 JSON 解析。

6. **只读草拟**：导航只用于用户确认的联系人与会话；不得修改消息、联系人或群配置。检索文本只能输入到已经确认的搜索框，禁止注入聊天输入框；无法区分搜索框与聊天框时停止。草稿、私人消息不写日志，不自动外发。

## 脚本调用与标准流程

在任意工作目录下调用前，先确认脚本路径与目标设备。助手须将 `<CONFIRMED_ABSOLUTE_SKILL_DIR>` 替换为本次实际读取的 SKILL.md 所在绝对目录；严禁用 `BASH_SOURCE` 或 `$0` 推导交互式终端中的技能目录。

```bash
set -euo pipefail
SKILL_DIR="<CONFIRMED_ABSOLUTE_SKILL_DIR>"
CLIENT_PY="${SKILL_DIR}/scripts/request_client.py"
if [ -z "${FARM_API_URL:-}" ]; then
  echo "错误: 缺少已确认的 FARM_API_URL，停止并确认，禁止默认地址。" >&2
  exit 1
fi
API_URL="${FARM_API_URL}"

# 首先读取当前 Agent 自动化契约，再确认目标设备
python3 "${CLIENT_PY}" --base-url "${API_URL}" /fram.skill
python3 "${CLIENT_PY}" --base-url "${API_URL}" /api/devices
# 根据设备列表和用户指定确认 SERIAL 后再继续，不在此设置默认 serial
if [ -z "${SERIAL:-}" ]; then
  echo "错误: 缺少已确认的 SERIAL，停止并向用户确认。" >&2
  exit 1
fi
python3 "${CLIENT_PY}" --base-url "${API_URL}" /api/v1/device-status --query "serial=${SERIAL}"
```

以下命令是按当前页面选用的独立步骤，禁止未核对页面便连续执行动作示例。JSON 通过 `json.dumps` 编码，避免文本与 serial 的引号、换行破坏请求。

### 0. 契约确认与设备可用性检查

按上面的编号顺序读取 `/fram.skill`，查询 `/api/devices` 并核对用户确认的 serial，获取 `/api/v1/device-status` 的实际尺寸与点击模式；设备不就绪立即停止。

### 1. 确认微信处于前台且设备就绪（闭环第一步：observe）

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["ref","ocr"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

核对首行包名为 `com.tencent.mm`，结合页面标题与控件确认当前页面。亮屏和解锁状态按 `/api/devices` 已确认字段检查；不从 `keyboard hidden` 猜测屏幕是否点亮。非微信或无法判断时请用户切换并重新观察。

### 2. 检索指定联系人或会话（observe XML → 轻触 → 再 observe）

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["xml"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

唯一定位当前搜索按钮或用户指定的会话条目，验证可见、启用、可点击及 bounds，在用户确认后导航。再次全量 observe 核对上下文，然后按以下方式归一化并轻触：

先将本轮确认的 device-status `width`、`height` 存入 `SCREEN_WIDTH`、`SCREEN_HEIGHT`，将唯一可点击节点的当前像素中心存入 `TARGET_X`、`TARGET_Y`；缺少任一值即停止。转换并发送：

```bash
TAP_PAYLOAD="$(python3 - "${SERIAL:?}" "${SCREEN_WIDTH:?}" "${SCREEN_HEIGHT:?}" "${TARGET_X:?}" "${TARGET_Y:?}" <<'PYCODE'
import json, math, sys
serial = sys.argv[1]
w, h, cx, cy = map(float, sys.argv[2:])
if not all(map(math.isfinite, (w, h, cx, cy))) or w <= 0 or h <= 0 or not (0 <= cx < w and 0 <= cy < h):
    raise SystemExit("尺寸或目标坐标无效，停止")
print(json.dumps({"serial": serial, "x": cx / w, "y": cy / h}))
PYCODE
)" || exit 1
printf '%s\n' "$TAP_PAYLOAD" | python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/v1/actions/tap --json-file -
```

进入已确认的搜索框后，如确需检索，仅注入用户指定的查询词，禁止包含换行、回车或消息正文：

```bash
SEARCH_PAYLOAD="$(python3 - "${SERIAL:?}" "${SEARCH_TEXT:?缺少已确认查询词}" <<'PYCODE'
import json, sys
serial, text = sys.argv[1:]
if not text.strip() or "\n" in text or "\r" in text:
    raise SystemExit("查询词为空或含换行，停止")
print(json.dumps({"serial": serial, "text": text}))
PYCODE
)" || exit 1
printf '%s\n' "$SEARCH_PAYLOAD" | python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/v1/actions/send-key --json-file -
```

每次点击或输入后全量 observe，确认搜索词实际显示、搜索结果与目标身份一致、进入的是用户指定会话。对方昵称可能重名，不足以唯一确认时停止；不得仅凭相似名字、包名未变或假定的聊天 Activity 判断成功。

### 3. 读取未读或最新消息（闭环后置观察：observe XML）

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["xml"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

1. 结合会话标题、文本节点、列表顺序及可核实的发送者标识读取消息；OCR 只作补充，不从头像位置或相似字段猜测身份。
2. 整理最近 3~5 条实际可见消息；缺少发送者或时间则明确标注未知，不补造字段。不确定时最多重采 3 次后停止。
3. 将必要的消息摘要呈现给用户，不自动标记、删除或转发消息。

### 4. 草拟回复建议（仅在 AI 助手内输出，绝不自动外发）

结合已核实上下文，在 AI 助手回复中给出简短确认、详细回复等建议草稿供用户选择，并提示用户审阅后在手机微信内手动发送。到此结束，不向聊天输入框注入草稿，不触发任何发送动作。
