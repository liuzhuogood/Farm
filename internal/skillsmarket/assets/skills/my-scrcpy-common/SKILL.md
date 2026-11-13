---
name: my-scrcpy-common
description: Farm 通用操作技能：通过现代自动化接口观察页面、受控浏览与返回，应用技能专注业务规则与页面 Wiki。
---

# 通用操作技能 (my-scrcpy-common)

本技能提供 Farm 通用页面观察与操作规则。应用业务判断留给应用技能与助手，底层请求采用当前 `/fram.skill` 契约。

## 核心操作能力

| 通用操作 | 脚本与入口 | 说明与验证 |
|---|---|---|
| **观察页面** | `scripts/request_client.py` → `POST /api/automation/observe` | `formats:["ref","ocr"]` 返回紧凑行协议；需要层次树时使用 `["xml"]` |
| **看视频** | `POST /api/v1/actions/swipe` | 在当前可滚动区域内计算起终点，使用归一化 `x,y,x2,y2`；按授权停留、次数和总时限有界执行，每次后置 observe |
| **尝试返回** | `POST /api/v1/actions/send-key` | `{"serial":"<已确认serial>","keycode":111}` 为 ESC；不能当作 Android BACK，实际效果必须 observe 核对 |
| **Android 返回** | `POST /api/v1/actions/send-key` 或 `POST /api/v1/actions/events` | 分别使用 `keycode:4` 或 `event:"back"`，每次只选择一种方式 |
| **打开 APP** | 当前公布契约未提供应用启动接口 | 请用户打开目标应用，再通过 observe 确认前台包名与业务页面；不得编造启动事件 |
| **重启 APP** | 当前公布契约未提供停止/重启接口 | 如确需重启，停止本轮自动操作并请用户处理，返回后重新 observe |

技能包原有专用脚本与规则文件仍保留，但其中存在旧契约调用，不能作为本次现代接口流程的执行入口。这里统一使用 `scripts/request_client.py` 直接调用已公布接口；不宣称专用脚本已迁移，也不以旧接口补足未公布能力。

## 使用规范

每次设备动作采用闭环：`observe(ref,ocr) → 必要时 observe(xml) → 再 observe(ref,ocr) 核对上下文 → 单次动作 → 再 observe(ref,ocr) 验证业务结果`。每次明确设置 `diff:false` 获取全量。核对前后包名、页面标题、目标节点与屏幕方向；状态不稳定则最多重采 3 次，仍不稳定则停止。XML 节点必须唯一、启用、可操作且 bounds 在当前可视区域内；OCR 文字或 ref 编号本身不能证明节点可点击。紧凑协议首行不含 Activity，不得猜测 Activity 或不存在的字段；页面无法可靠识别时停止。

1. **地址与设备**：必须从环境变量 `FARM_API_URL` 或用户指令读取已确认的服务地址；用户提供地址后将其导出为 `FARM_API_URL`。缺失即停止确认，严禁回退到 `http://127.0.0.1:8080` 或任何默认端口。通过 `GET /api/devices` 核实目标 `serial` 的 `state == "device"`、`screen_on == true`、`screen_locked == false`；缺失、不就绪则停止。沿用用户明确指定的 serial，否则先向用户确认，禁止自动选第一台设备。
2. **契约来源**：先读取已确认的 Agent 侧服务地址 + `/fram.skill`，核对路径、参数、响应及认证约定，不要求读取完整 OpenAPI。只按已确认字段取值；字段缺失或契约不一致则停止报告，不猜测字段、不切回旧接口。需要认证时，按已确认配置使用 `FARM_API_TOKEN`、`FARM_API_TOKEN_HEADER` 和可选 `FARM_API_TOKEN_PREFIX`，禁止猜测认证头或打印凭据。
3. **坐标与模式**：`GET /api/v1/device-status?serial=...` 的 `status` 必须为 `ready`、`serial` 必须匹配；读取实际 `width`、`height`、`touch_mode`，宽高必须为正且与当前 observe 的 `screen WxH` 及方向一致。observe 的 `@X,Y` 是屏幕像素，当前行协议没有归一化坐标。点击转换为 `x = X / width`、`y = Y / height`；XML bounds 中心为 `X = (left + right) / 2`、`Y = (top + bottom) / 2`，滑动两端分别同样转换。先检查像素在屏幕内及归一化值在 0~1 内；禁止硬编码绝对像素或把像素直接传给 tap/swipe。旋转、尺寸或页面变化后重新采集，不复用旧坐标；未明确要求时省略 `mode` 以继承系统点击模式。
4. **执行结果与重试**：动作响应必须同时满足 `status == "executed"` 和 `executed == true`，仍需后续 observe 核对预期业务变化；不能把 HTTP 200、画面任意变动或动作自述当作成功。单次 HTTP 超时 30 秒；后置验证每隔 1 秒重采，最多 3 次，超时明确退出。动作失败或超时先 observe 核实是否已生效，再决定是否重试；包括首次在内动作尝试最多 3 次，每次重新定位，禁止盲目重放。敏感操作不得自动重试。记录 serial、步骤、request_id（响应提供时）、执行结果及前后页面证据，异常保留原始响应，日志不得泄露凭据和私人消息。
5. **请求工具**：所有请求使用当前技能目录内的 `scripts/request_client.py`，无需额外依赖。POST 的 serial 放在 JSON body；device-status 的 serial 放在 query。observe 的 `formats:["ref","ocr"]` 返回 `text/plain` 紧凑行协议，`formats:["xml"]` 返回 `text/xml` 原始层次树，均不可按 JSON 解析。

6. **决策建议不自动执行**：由助手结合本轮 observe 分析页面，仅作决策参考；不得假定 observe 返回旧脚本的 `observation_id`、`context_stable`、Activity 或 AI decision JSON。执行前须重新核对当前页面与授权。
7. **返回判断谨慎**：像素或任意画面变动仅为辅助，必须确认预期弹层消除或页面返回。无法确认则记录“未知”，不得连续盲发 ESC/BACK。
8. **受控工作目录**：在 `~/.mywebscrcppy/tasks/<task-id>/` 的独立任务目录维护必要证据，无需向服务端上报心跳或日志；网络异常、超时和执行状态需在本地记录，避免泄露隐私。

### 1. 契约与设备确认

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

### 2. 全量观察与层次树

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["ref","ocr"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

需要精确定位时：

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["xml"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

再次全量 observe 核对上下文后才可执行单次动作。

### 3. 动作与闭环核验

Android 返回示例（执行前先确认页面和返回的预期目标）：

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"keycode":4}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/v1/actions/send-key --json-file -
```

点击使用 `POST /api/v1/actions/tap` 的 `{"serial":"<已确认serial>","x":<X/width>,"y":<Y/height>}`；滑动使用 `POST /api/v1/actions/swipe` 的 `{"serial":"<已确认serial>","x":<起点X/width>,"y":<起点Y/height>,"x2":<终点X/width>,"y2":<终点Y/height>}`。从当前唯一节点或滚动区域动态计算坐标，禁止硬编码绝对像素。输入仅可对已确认且已授权的焦点使用 `POST /api/v1/actions/send-key` 的 `{"serial":"<已确认serial>","text":"<已确认内容>"}`。

每次动作后重复第 2 步全量 observe 并验证业务结果，按使用规范最多 3 次有界处理；不将 HTTP 200 作为成功依据。
