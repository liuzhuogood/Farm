---
name: farm-kuaishou-daily
description: 快手客户端的界面识别、滑动浏览与日常活动打卡辅助流程，需要结合 Farm 设备控制。
---

# 快手日常助手 (Kuaishou Daily)

专为快手短视频客户端定制的界面识别与交互辅助技能，支持在已授权设备上完成推荐流浏览、特定组件识别与活动任务辅助流转。

## 核心原则

每次设备动作采用闭环：`observe(ref,ocr) → 必要时 observe(xml) → 再 observe(ref,ocr) 核对上下文 → 单次动作 → 再 observe(ref,ocr) 验证业务结果`。每次明确设置 `diff=false` 获取全量。核对前后包名、页面标题、目标节点与屏幕方向；状态不稳定则最多重采 3 次，仍不稳定则停止。XML 节点必须唯一、启用、可操作且 bounds 在当前可视区域内；OCR 文字或 ref 编号本身不能证明节点可点击。紧凑协议首行不含 Activity，不得猜测 Activity 或不存在的字段；页面无法可靠识别时停止。

1. **地址与设备**：从 `FARM_API_URL` 或用户指令读取已确认的服务地址，不猜默认端口。先读 `/fram.skill`，再 `GET /api/automation/devices`；设备列表为纯文本，每行 `设备 "标识名" · serial "序列号" · 在线`。按用户指定设备核对真实 serial，禁止自行选第一台。后续请求的 query 必须传真实 serial，不能传标识名。列表不证明亮屏或解锁，需继续观察当前页面；不可读时请用户处理。
2. **契约与请求**：只用当前 `/fram.skill` 公布接口及本技能 `scripts/request_client.py`。先 `POST /api/automation/tasks?serial=真实序列号&title=AI生成的任务标题`，空正文开始完整任务，从 JSON 的 `task.id` 读取 `TASK_ID`。AI 根据整个目标生成简短 `TASK_TITLE`（1..120字符，不能含控制字符），全程复用，不要求用户代填。每批操作使用 `POST /api/automation/act?serial=真实序列号&title=同一任务标题&task_id=TASK_ID`；标题必须与该任务一致，不能按同名猜测归属。正文为纯文本，每行 `操作 参数`，`--text` 原样发送，不在正文重复设备。多行操作最多32个非空行，先全量校验再按序执行；sendkey 只接收本行剩余文本，输入内容不能含回车换行。认证只使用已确认的环境配置，不猜字段或打印凭据。
3. **定位**：`GET /api/automation/observe` 用 query 传 `serial`、`formats=ref,ocr`、`diff=false`、`settle_ms=300`，返回紧凑文本；`formats=xml` 返回原始 XML。首行 `screen WxH` 给出像素尺寸，`@X,Y` 直接用于点击，不归一化。XML bounds 中心取整数，先验证在当前屏幕内；页面或方向变化后重新定位。ref 仅用于观察，不作为动作参数。
4. **结果与重试**：响应为格式化纯文本，核对设备、状态、完成数及实际返回的步骤；只有外层状态为“已完成”、完成行数等于总行数，才表示指令执行完毕，随后仍须 observe 核对业务结果。逐行括号内的完成数表示该操作内部执行数量，如keyevent已执行按键数；不能与外层完成行数混用。check 的“命中/未命中”是检查结果，不等于点击成功。任一操作失败、check未命中或wait超时立即停止后续行；查看完成数、失败原始行号和原因。超时或部分执行先观察，禁止盲重放整个多行指令或多键序列。后置观察每隔1秒最多3次；动作最多尝试3次，敏感操作不自动重试。
5. **边界**：默认请求超时30秒；服务端整批最长60秒，多行指令使用 `--timeout 65`；单次sleep或wait为30000ms时也需留出请求耗时。不把 HTTP 200 或任意画面变化当作业务成功，不猜亮屏、锁屏、Activity 等未提供信息。日志仅记录必要步骤与页面证据，不泄露凭据和私人消息。

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
python3 "${CLIENT_PY}" --base-url "${API_URL}" /api/automation/devices
# 根据设备列表和用户指定确认 SERIAL 后再继续，不在此设置默认 serial
if [ -z "${SERIAL:-}" ]; then
  echo "错误: 缺少已确认的 SERIAL，停止并向用户确认。" >&2
  exit 1
fi
```

### 开始完整任务

确认 `SERIAL` 后、首批动作前执行。`TASK_TITLE` 由 AI 根据整个目标生成；后续批次、重试和恢复均复用同一标题与 ID。

```bash
TASK_TITLE="${TASK_TITLE:?请由AI生成整个任务标题}"
TASK_JSON="$(python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST \
  /api/automation/tasks --query "serial=${SERIAL}" --query "title=${TASK_TITLE}")" || exit 1
TASK_ID="$(python3 -c 'import json, sys; print(json.loads(sys.argv[1])["task"]["id"])' "${TASK_JSON}")" || exit 1
TASK_DIRECTORY="$(python3 -c 'import json, sys; print(json.loads(sys.argv[1])["task"]["directory"])' "${TASK_JSON}")" || exit 1
```

`TASK_DIRECTORY` 是设备 `/sdcard/Farm/automation` 下的建议相对工作目录，创建任务不会自动创建目录。需要截图、录屏时，先通过携带同一 `task_id` 的 act 执行 `mkdir ${TASK_DIRECTORY}`，再保存到该目录。单批结束后任务仍保持进行中；发生失败先观察核对，可在同一任务内有界恢复。确认整体结果后按文末流程明确结束，避免失败或脚本退出后遗留任务。

旧调用仍可省略 `task_id`，由服务器生成并结束独立批次任务；设备存在明确任务时会拒绝此类调用，不能用省略 ID 绕过任务归属。本技能以下流程统一使用显式任务。

以下命令按当前页面独立选用；每次先观察再动作。正文是纯文本命令，不包装对象。设备只放 query，单行参数原样保留；用户输入含回车换行时停止，不能让输入变成额外操作行。

### 1. 确认前台应用与屏幕就绪

```bash
python3 "${CLIENT_PY}" --base-url "${API_URL}" /api/automation/observe \
  --query "serial=${SERIAL}" --query formats=ref,ocr --query diff=false --query settle_ms=300
```

核对首行实际包名与已确认的目标快手包名（如用户已确认的 `com.smile.gifmaker`），再用页面标题与推荐流节点确认业务页面。当前非目标前台时请用户切换后重新 observe；不得假定紧凑协议含 Activity。

### 2. 界面分析与动态手势滑动

```bash
python3 "${CLIENT_PY}" --base-url "${API_URL}" /api/automation/observe \
  --query "serial=${SERIAL}" --query formats=xml --query diff=false --query settle_ms=300
```

从 XML 唯一定位可滚动的推荐流区域，校验 `scrollable="true"`、`enabled="true"` 及 bounds 在当前屏幕内。将其实际边界保存为 `LEFT`、`TOP`、`RIGHT`、`BOTTOM`，把本轮 observe 的 `screen WxH` 尺寸保存为 `SCREEN_WIDTH`、`SCREEN_HEIGHT`。再次全量 observe 确认上下文稳定，然后动态计算区域内从下至上的像素滑动：

```bash
SWIPE_COMMAND="$(python3 - "${SCREEN_WIDTH:?}" "${SCREEN_HEIGHT:?}" "${LEFT:?}" "${TOP:?}" "${RIGHT:?}" "${BOTTOM:?}" <<'PYCODE'
import math, sys
w, h, left, top, right, bottom = map(float, sys.argv[1:])
if not all(map(math.isfinite, (w, h, left, top, right, bottom))) or w <= 0 or h <= 0 or not (0 <= left < right <= w and 0 <= top < bottom <= h):
    raise SystemExit("尺寸或滚动区域无效，停止")
cx = (left + right) / 2
y_start = top + (bottom - top) * 0.8
y_end = top + (bottom - top) * 0.2
print(f"swipe @{int(cx)},{int(y_start)} @{int(cx)},{int(y_end)} 300")
PYCODE
)" || exit 1
python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/act --query "serial=${SERIAL}" --query "title=${TASK_TITLE:?缺少原任务标题}" --query "task_id=${TASK_ID:?缺少已开始任务ID}" --text "$SWIPE_COMMAND"
```

滑动后按有界后置验证再次全量 observe，确认仍在目标推荐流、视频内容已切换且就绪。仅包名未变不能证明滑动成功；按已授权观看时长停留，无可靠业务证据则记录未确认并停止。

### 3. 日常签到与活动入口识别与安全点击

1. **上下文校验**：全量 observe 确认当前仍是目标快手页面，操作属于用户授权活动范围。
2. **唯一节点定位**：必要时 observe XML 检索“签到”“福利”“领金币”等当前实际可见节点；匹配明确属性并校验 `enabled="true"`、`clickable="true"`，或唯一定位符合条件的可点击父级。
3. **动态坐标**：解析当前 bounds 中心，验证在屏幕内，并再次全量 observe 核对上下文，不能复用旧 ref 或旧坐标。
4. **点击请求**：按以下流程传入当前屏幕像素坐标。

先将本轮 observe 的 `screen WxH` 宽高存入 `SCREEN_WIDTH`、`SCREEN_HEIGHT`，将唯一可点击节点的当前像素中心存入 `TARGET_X`、`TARGET_Y`；缺少任一值即停止。按当前像素坐标发送：

```bash
TAP_COMMAND="$(python3 - "${SCREEN_WIDTH:?}" "${SCREEN_HEIGHT:?}" "${TARGET_X:?}" "${TARGET_Y:?}" <<'PYCODE'
import math, sys
w, h, cx, cy = map(float, sys.argv[1:])
if not all(map(math.isfinite, (w, h, cx, cy))) or w <= 0 or h <= 0 or not (0 <= cx < w and 0 <= cy < h):
    raise SystemExit("尺寸或目标坐标无效，停止")
print(f"tap @{int(cx)},{int(cy)}")
PYCODE
)" || exit 1
python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/act --query "serial=${SERIAL}" --query "title=${TASK_TITLE:?缺少原任务标题}" --query "task_id=${TASK_ID:?缺少已开始任务ID}" --text "$TAP_COMMAND"
```

5. **业务验证**：点击后全量 observe，必要时再次 XML 验证目标活动弹窗或签到状态是否实际更新；最多 3 次有界重采。未确认时先核实是否已执行，禁止重复领取、误点付费或敏感操作。


### 核对结果并结束任务

最终全量 observe 核对业务结果后，AI 设置 `TASK_OUTCOME=done` 和实际完成摘要；确认无法完成则用 `failed`，用户要求停止则用 `stopped`。步骤失败或脚本提前退出后也须核查任务状态并明确结束；不能只根据 HTTP 200 或动作完成数写 done。摘要不包含凭据、私人消息或回复正文。

```bash
python3 - "${TASK_OUTCOME:?请由AI根据核对结果设置done或failed}" "${TASK_SUMMARY:?缺少实际结果摘要}" <<'PYFINISH' | \
  python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST \
  "/api/automation/tasks/${TASK_ID:?缺少已开始任务ID}/finish" --query "serial=${SERIAL}" --json-file -
import json, sys
outcome, summary = sys.argv[1:]
if outcome not in {"done", "failed", "stopped"}:
    raise SystemExit("任务结束状态无效")
print(json.dumps({"outcome": outcome, "summary": summary}, ensure_ascii=False))
PYFINISH
```

核对 JSON `task.id` 为原 `TASK_ID`、`task.state` 与结束结果一致、`task.active` 为 false。结束失败或超时应查询 `GET /api/automation/tasks/${TASK_ID}?serial=真实序列号` 核对实际状态；不得假定已结束，也不能向已结束任务继续追加动作。
