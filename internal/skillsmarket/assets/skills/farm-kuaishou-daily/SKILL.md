---
name: farm-kuaishou-daily
description: 快手客户端的界面识别、滑动浏览与日常活动打卡辅助流程，需要结合 Farm 设备控制。
---

# 快手日常助手 (Kuaishou Daily)

专为快手短视频客户端定制的界面识别与交互辅助技能，支持在已授权设备上完成推荐流浏览、特定组件识别与活动任务辅助流转。

## 核心原则

每次设备动作采用闭环：`observe(ref,ocr) → 必要时 observe(xml) → 再 observe(ref,ocr) 核对上下文 → 单次动作 → 再 observe(ref,ocr) 验证业务结果`。每次明确设置 `diff:false` 获取全量。核对前后包名、页面标题、目标节点与屏幕方向；状态不稳定则最多重采 3 次，仍不稳定则停止。XML 节点必须唯一、启用、可操作且 bounds 在当前可视区域内；OCR 文字或 ref 编号本身不能证明节点可点击。紧凑协议首行不含 Activity，不得猜测 Activity 或不存在的字段；页面无法可靠识别时停止。

1. **地址与设备**：必须从环境变量 `FARM_API_URL` 或用户指令读取已确认的服务地址；用户提供地址后将其导出为 `FARM_API_URL`。缺失即停止确认，严禁回退到 `http://127.0.0.1:8080` 或任何默认端口。通过 `GET /api/devices` 核实目标 `serial` 的 `state == "device"`、`screen_on == true`、`screen_locked == false`；缺失、不就绪则停止。沿用用户明确指定的 serial，否则先向用户确认，禁止自动选第一台设备。
2. **契约来源**：先读取已确认的 Agent 侧服务地址 + `/fram.skill`，核对路径、参数、响应及认证约定，不要求读取完整 OpenAPI。只按已确认字段取值；字段缺失或契约不一致则停止报告，不猜测字段、不切回旧接口。需要认证时，按已确认配置使用 `FARM_API_TOKEN`、`FARM_API_TOKEN_HEADER` 和可选 `FARM_API_TOKEN_PREFIX`，禁止猜测认证头或打印凭据。
3. **坐标与模式**：`GET /api/v1/device-status?serial=...` 的 `status` 必须为 `ready`、`serial` 必须匹配；读取实际 `width`、`height`、`touch_mode`，宽高必须为正且与当前 observe 的 `screen WxH` 及方向一致。observe 的 `@X,Y` 是屏幕像素，当前行协议没有归一化坐标。点击转换为 `x = X / width`、`y = Y / height`；XML bounds 中心为 `X = (left + right) / 2`、`Y = (top + bottom) / 2`，滑动两端分别同样转换。先检查像素在屏幕内及归一化值在 0~1 内；禁止硬编码绝对像素或把像素直接传给 tap/swipe。旋转、尺寸或页面变化后重新采集，不复用旧坐标；未明确要求时省略 `mode` 以继承系统点击模式。
4. **执行结果与重试**：动作响应必须同时满足 `status == "executed"` 和 `executed == true`，仍需后续 observe 核对预期业务变化；不能把 HTTP 200、画面任意变动或动作自述当作成功。单次 HTTP 超时 30 秒；后置验证每隔 1 秒重采，最多 3 次，超时明确退出。动作失败或超时先 observe 核实是否已生效，再决定是否重试；包括首次在内动作尝试最多 3 次，每次重新定位，禁止盲目重放。敏感操作不得自动重试。记录 serial、步骤、request_id（响应提供时）、执行结果及前后页面证据，异常保留原始响应，日志不得泄露凭据和私人消息。
5. **请求工具**：所有请求使用当前技能目录内的 `scripts/request_client.py`，无需额外依赖。POST 的 serial 放在 JSON body；device-status 的 serial 放在 query。observe 的 `formats:["ref","ocr"]` 返回 `text/plain` 紧凑行协议，`formats:["xml"]` 返回 `text/xml` 原始层次树，均不可按 JSON 解析。

6. **业务授权**：严禁未经授权的账户敏感操作，如私信回复、打赏、支付等；日常打卡与浏览需受控进行。先确认当前应用为用户指定的快手版本，不根据相似包名猜测同系变体。缺少目标应用或授权时停止，不盲目启动应用或注入输入。

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

### 1. 确认前台应用与屏幕就绪

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["ref","ocr"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

核对首行实际包名与已确认的目标快手包名（如用户已确认的 `com.smile.gifmaker`），再用页面标题与推荐流节点确认业务页面。当前非目标前台时请用户切换后重新 observe；不得假定紧凑协议含 Activity。

### 2. 界面分析与动态手势滑动

```bash
python3 -c 'import json,sys; print(json.dumps({"serial":sys.argv[1],"formats":["xml"],"diff":False,"settle_ms":300}))' "${SERIAL}" | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/observe --json-file -
```

从 XML 唯一定位可滚动的推荐流区域，校验 `scrollable="true"`、`enabled="true"` 及 bounds 在当前屏幕内。将其实际边界保存为 `LEFT`、`TOP`、`RIGHT`、`BOTTOM`，把本轮 device-status 尺寸保存为 `SCREEN_WIDTH`、`SCREEN_HEIGHT`。再次全量 observe 确认上下文稳定，然后动态计算区域内从下至上的滑动并归一化：

```bash
SWIPE_PAYLOAD="$(python3 - "${SERIAL:?}" "${SCREEN_WIDTH:?}" "${SCREEN_HEIGHT:?}" "${LEFT:?}" "${TOP:?}" "${RIGHT:?}" "${BOTTOM:?}" <<'PYCODE'
import json, math, sys
serial = sys.argv[1]
w, h, left, top, right, bottom = map(float, sys.argv[2:])
if not all(map(math.isfinite, (w, h, left, top, right, bottom))) or w <= 0 or h <= 0 or not (0 <= left < right <= w and 0 <= top < bottom <= h):
    raise SystemExit("尺寸或滚动区域无效，停止")
cx = (left + right) / 2
y_start = top + (bottom - top) * 0.8
y_end = top + (bottom - top) * 0.2
print(json.dumps({"serial": serial, "x": cx / w, "y": y_start / h, "x2": cx / w, "y2": y_end / h}))
PYCODE
)" || exit 1
printf '%s\n' "$SWIPE_PAYLOAD" | python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/v1/actions/swipe --json-file -
```

滑动后按有界后置验证再次全量 observe，确认仍在目标推荐流、视频内容已切换且就绪。仅包名未变不能证明滑动成功；按已授权观看时长停留，无可靠业务证据则记录未确认并停止。

### 3. 日常签到与活动入口识别与安全点击

1. **上下文校验**：全量 observe 确认当前仍是目标快手页面，操作属于用户授权活动范围。
2. **唯一节点定位**：必要时 observe XML 检索“签到”“福利”“领金币”等当前实际可见节点；匹配明确属性并校验 `enabled="true"`、`clickable="true"`，或唯一定位符合条件的可点击父级。
3. **动态坐标**：解析当前 bounds 中心，验证在屏幕内，并再次全量 observe 核对上下文，不能复用旧 ref 或旧坐标。
4. **点击请求**：按以下流程从像素转成归一化坐标。

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

5. **业务验证**：点击后全量 observe，必要时再次 XML 验证目标活动弹窗或签到状态是否实际更新；最多 3 次有界重采。未确认时先核实是否已执行，禁止重复领取、误点付费或敏感操作。
