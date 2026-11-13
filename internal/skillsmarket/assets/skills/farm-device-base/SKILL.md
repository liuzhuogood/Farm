---
name: farm-device-base
description: 使用 Farm 的现代自动化接口进行 Android 页面感知、XML 识别与归一化手势按键输入。
---

# 手机操作基础 (Device Base)

为 AI 助手提供通用的 Android 页面感知、控件层级树提取与精准输入模拟能力。

## 核心原则

每次设备动作采用闭环：`observe(ref,ocr) → 必要时 observe(xml) → 再 observe(ref,ocr) 核对上下文 → 单次动作 → 再 observe(ref,ocr) 验证业务结果`。每次明确设置 `diff:false` 获取全量。核对前后包名、页面标题、目标节点与屏幕方向；状态不稳定则最多重采 3 次，仍不稳定则停止。XML 节点必须唯一、启用、可操作且 bounds 在当前可视区域内；OCR 文字或 ref 编号本身不能证明节点可点击。紧凑协议首行不含 Activity，不得猜测 Activity 或不存在的字段；页面无法可靠识别时停止。

1. **地址与设备**：必须从环境变量 `FARM_API_URL` 或用户指令读取已确认的服务地址；用户提供地址后将其导出为 `FARM_API_URL`。缺失即停止确认，严禁回退到 `http://127.0.0.1:8080` 或任何默认端口。通过 `GET /api/devices` 核实目标 `serial` 的 `state == "device"`、`screen_on == true`、`screen_locked == false`；缺失、不就绪则停止。沿用用户明确指定的 serial，否则先向用户确认，禁止自动选第一台设备。
2. **契约来源**：先读取已确认的 Agent 侧服务地址 + `/fram.skill`，核对路径、参数、响应及认证约定，不要求读取完整 OpenAPI。只按已确认字段取值；字段缺失或契约不一致则停止报告，不猜测字段、不切回旧接口。需要认证时，按已确认配置使用 `FARM_API_TOKEN`、`FARM_API_TOKEN_HEADER` 和可选 `FARM_API_TOKEN_PREFIX`，禁止猜测认证头或打印凭据。
3. **坐标与模式**：`GET /api/v1/device-status?serial=...` 的 `status` 必须为 `ready`、`serial` 必须匹配；读取实际 `width`、`height`、`touch_mode`，宽高必须为正且与当前 observe 的 `screen WxH` 及方向一致。observe 的 `@X,Y` 是屏幕像素，当前行协议没有归一化坐标。点击转换为 `x = X / width`、`y = Y / height`；XML bounds 中心为 `X = (left + right) / 2`、`Y = (top + bottom) / 2`，滑动两端分别同样转换。先检查像素在屏幕内及归一化值在 0~1 内；禁止硬编码绝对像素或把像素直接传给 tap/swipe。旋转、尺寸或页面变化后重新采集，不复用旧坐标；未明确要求时省略 `mode` 以继承系统点击模式。
4. **执行结果与重试**：动作响应必须同时满足 `status == "executed"` 和 `executed == true`，仍需后续 observe 核对预期业务变化；不能把 HTTP 200、画面任意变动或动作自述当作成功。单次 HTTP 超时 30 秒；后置验证每隔 1 秒重采，最多 3 次，超时明确退出。动作失败或超时先 observe 核实是否已生效，再决定是否重试；包括首次在内动作尝试最多 3 次，每次重新定位，禁止盲目重放。敏感操作不得自动重试。记录 serial、步骤、request_id（响应提供时）、执行结果及前后页面证据，异常保留原始响应，日志不得泄露凭据和私人消息。
5. **请求工具**：所有请求使用当前技能目录内的 `scripts/request_client.py`，无需额外依赖。POST 的 serial 放在 JSON body；device-status 的 serial 放在 query。observe 的 `formats:["ref","ocr"]` 返回 `text/plain` 紧凑行协议，`formats:["xml"]` 返回 `text/xml` 原始层次树，均不可按 JSON 解析。

## 常用流程与脚本调用

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

### 1. 设备检查与连接确认

执行上面的 `/fram.skill`、`/api/devices` 与 `/api/v1/device-status` 检查，核实已确认的 serial、屏幕宽高和点击模式；只操作在线、亮屏且未锁定的目标设备。

### 2. 页面信息与控件树提取

获取前台包名、屏幕尺寸、原生控件与 OCR 补充文字：

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["ref","ocr"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

需要启用状态、可点击属性、父子关系与 bounds 时获取 XML，再次 observe 全量核对上下文后才能动作：

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["xml"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

`ref` 仅表示本轮观测中的节点编号，动作接口不接收 ref。若需要图像证据，可按 `/fram.skill` 调用 `GET /api/devices/{serial}/latest-frame`，不得把旧帧当成当前页面。

### 3. 按键与系统操作注入

返回键使用现代 send-key；serial 与 keycode 同在 body（主屏键为 keycode 3，执行前也须确认任务范围）：

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"keycode":4}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/v1/actions/send-key --json-file -
```

也可按场景选择系统事件返回，不能把两种返回命令连续执行：

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"event":"back"}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/v1/actions/events --json-file -
```

只有确认当前焦点是已授权的输入框时才注入文本；`INPUT_TEXT` 为已确认内容，不能向未知焦点注入：

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"text":sys.argv[2]}))' "${SERIAL}" "${INPUT_TEXT:?缺少已确认输入内容}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/v1/actions/send-key --json-file -
```

点击可见控件：

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

滑动使用 `POST /api/v1/actions/swipe`，body 为 `{"serial":"<已确认serial>","x":<起点X/width>,"y":<起点Y/height>,"x2":<终点X/width>,"y2":<终点Y/height>}`。两端从当前已确认的可滚动区域动态计算，检查区域边界，禁止滑过未授权敏感控件。

每次动作后重新执行第 2 步的全量 observe，核对返回后的页面、输入框实际文本或预期控件状态；失败按核心原则有界处理。
