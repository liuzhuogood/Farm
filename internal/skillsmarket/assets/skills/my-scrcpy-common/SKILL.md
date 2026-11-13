---
name: my-scrcpy-common
description: Farm 通用操作技能：通过现代自动化接口观察页面、受控浏览与返回，应用技能专注业务规则与页面 Wiki。
---

# 通用操作技能 (my-scrcpy-common)

本技能提供 Farm 通用页面观察与操作规则。应用业务判断留给应用技能与助手，底层请求采用当前 `/fram.skill` 契约。

## 核心操作能力

| 通用操作 | 脚本与入口 | 说明与验证 |
|---|---|---|
| **观察页面** | `scripts/request_client.py` → `GET /api/automation/observe` | query `formats=ref,ocr` 返回紧凑行协议；需要层次树时使用 `formats=xml` |
| **看视频** | `POST /api/automation/act?serial=真实序列号&title=同一任务标题&task_id=TASK_ID` | 在当前可滚动区域内计算像素起终点，正文 `swipe @X1,Y1 @X2,Y2 300`；按授权停留、次数和总时限有界执行，每次后置 observe |
| **尝试 ESC 返回** | 当前统一操作没有 ESC 键 | 不以 BACK 代替 ESC；需要 ESC 时停止，不能调用已停用脚本绕过契约 |
| **Android 返回** | `POST /api/automation/act?serial=真实序列号&title=同一任务标题&task_id=TASK_ID` | 正文 `keyevent back`；先核对预期页面，每次后置 observe |
| **打开 APP** | 当前公布契约未提供应用启动接口 | 请用户打开目标应用，再通过 observe 确认前台包名与业务页面；不得编造启动事件 |
| **重启 APP** | 当前公布契约未提供停止/重启接口 | 如确需重启，停止本轮自动操作并请用户处理，返回后重新 observe |

技能包原有专用脚本与规则文件仍保留，但其中存在旧契约调用，不能作为本次现代接口流程的执行入口。这里统一使用 `scripts/request_client.py` 直接调用已公布接口；不宣称专用脚本已迁移，也不以旧接口补足未公布能力。

## 使用规范

每次设备动作采用闭环：`observe(ref,ocr) → 必要时 observe(xml) → 再 observe(ref,ocr) 核对上下文 → 单次动作 → 再 observe(ref,ocr) 验证业务结果`。每次明确设置 `diff=false` 获取全量。核对前后包名、页面标题、目标节点与屏幕方向；状态不稳定则最多重采 3 次，仍不稳定则停止。XML 节点必须唯一、启用、可操作且 bounds 在当前可视区域内；OCR 文字或 ref 编号本身不能证明节点可点击。紧凑协议首行不含 Activity，不得猜测 Activity 或不存在的字段；页面无法可靠识别时停止。

1. **地址与设备**：从 `FARM_API_URL` 或用户指令读取已确认的服务地址，不猜默认端口。先读 `/fram.skill`，再 `GET /api/automation/devices`；设备列表为纯文本，每行 `设备 "标识名" · serial "序列号" · 在线`。按用户指定设备核对真实 serial，禁止自行选第一台。后续请求的 query 必须传真实 serial，不能传标识名。列表不证明亮屏或解锁，需继续观察当前页面；不可读时请用户处理。
2. **契约与请求**：只用当前 `/fram.skill` 公布接口及本技能 `scripts/request_client.py`。先 `POST /api/automation/tasks?serial=真实序列号&title=AI生成的任务标题`，空正文开始完整任务，从 JSON 的 `task.id` 读取 `TASK_ID`。AI 根据整个目标生成简短 `TASK_TITLE`（1..120字符，不能含控制字符），全程复用，不要求用户代填。每批操作使用 `POST /api/automation/act?serial=真实序列号&title=同一任务标题&task_id=TASK_ID`；标题必须与该任务一致，不能按同名猜测归属。正文为纯文本，每行 `操作 参数`，`--text` 原样发送，不在正文重复设备。多行操作最多32个非空行，先全量校验再按序执行；sendkey 只接收本行剩余文本，输入内容不能含回车换行。认证只使用已确认的环境配置，不猜字段或打印凭据。
3. **定位**：`GET /api/automation/observe` 用 query 传 `serial`、`formats=ref,ocr`、`diff=false`、`settle_ms=300`，返回紧凑文本；`formats=xml` 返回原始 XML。首行 `screen WxH` 给出像素尺寸，`@X,Y` 直接用于点击，不归一化。XML bounds 中心取整数，先验证在当前屏幕内；页面或方向变化后重新定位。ref 仅用于观察，不作为动作参数。
4. **结果与重试**：响应为格式化纯文本，核对设备、状态、完成数及实际返回的步骤；只有外层状态为“已完成”、完成行数等于总行数，才表示指令执行完毕，随后仍须 observe 核对业务结果。逐行括号内的完成数表示该操作内部执行数量，如keyevent已执行按键数；不能与外层完成行数混用。check 的“命中/未命中”是检查结果，不等于点击成功。任一操作失败、check未命中或wait超时立即停止后续行；查看完成数、失败原始行号和原因。超时或部分执行先观察，禁止盲重放整个多行指令或多键序列。后置观察每隔1秒最多3次；动作最多尝试3次，敏感操作不自动重试。
5. **边界**：默认请求超时30秒；服务端整批最长60秒，多行指令使用 `--timeout 65`；单次sleep或wait为30000ms时也需留出请求耗时。不把 HTTP 200 或任意画面变化当作业务成功，不猜亮屏、锁屏、Activity 等未提供信息。日志仅记录必要步骤与页面证据，不泄露凭据和私人消息。

6. **决策建议不自动执行**：由助手结合本轮 observe 分析页面，仅作决策参考；不得假定 observe 返回旧脚本的 `observation_id`、`context_stable`、Activity 或额外的 AI 决策字段。执行前须重新核对当前页面与授权。
7. **返回判断谨慎**：像素或任意画面变动仅为辅助，必须确认预期弹层消除或页面返回。无法确认则记录“未知”，不得连续盲发 ESC/BACK。
8. **受控工作目录**：如需助手侧说明，可继续在 `~/.mywebscrcppy/tasks/<task-id>/` 维护必要记录；设备截图、录屏使用 `TASK_DIRECTORY` 指定的设备相对目录。无需另发心跳或日志；异常与结果通过真实任务和页面证据核对，避免泄露隐私。

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

### 2. 全量观察与层次树

```bash
python3 "${CLIENT_PY}" --base-url "${API_URL}" /api/automation/observe \
  --query "serial=${SERIAL}" --query formats=ref,ocr --query diff=false --query settle_ms=300
```

需要精确定位时：

```bash
python3 "${CLIENT_PY}" --base-url "${API_URL}" /api/automation/observe \
  --query "serial=${SERIAL}" --query formats=xml --query diff=false --query settle_ms=300
```

再次全量 observe 核对上下文后才可执行单次动作。

### 3. 动作与闭环核验

Android 返回示例（执行前先确认页面和返回的预期目标）：

```bash
python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/act --query "serial=${SERIAL}" --query "title=${TASK_TITLE:?缺少原任务标题}" --query "task_id=${TASK_ID:?缺少已开始任务ID}" \
  --text "keyevent back"
```

点击正文为 `tap @X,Y`，滑动正文为 `swipe @X1,Y1 @X2,Y2 300`，时长默认300ms、范围80~10000ms；都向 `/api/automation/act` 请求并通过 query 指定真实 serial。从当前唯一节点或滚动区域计算像素坐标，禁止硬编码或归一化。仅对已确认且已授权的焦点输入 `sendkey 已确认内容`。

`keyevent` 可将已授权按键名用 `|`、`｜`、`,`、`，` 串联，最多32项；中途失败查看“完成”数量，禁止盲重放。`check 关键词` 对默认 observe 的本轮控件树标签和 OCR 文字做包含判断；页面读取统一调用 observe。`set-copy 文字` 只写入手机剪贴板并保留原文，不粘贴；`copy` 无参数复制手机已选文字，`paste` 无参数粘贴手机剪贴板到当前输入框；`clear` 无参数清空当前聚焦可编辑输入框，不修改剪贴板；`sleep 500` 固定延时500毫秒，允许0~30000；`wait 10000 关键词` 最多等待10000毫秒直到文字出现，超时上限30000毫秒。多行最多32个非空行，全量预校验后执行，失败、check未命中或wait超时即停。

每次动作后重复第 2 步全量 observe 并验证业务结果，按使用规范最多 3 次有界处理；不将 HTTP 200 作为成功依据。

### 多行操作示例

仅在用户已授权、当前点击目标和“设置”“WLAN”文字均已核实时使用。坐标来自本轮观察，整批执行前重新确认页面；每行独立操作，失败即停。输入文本不允许换行，不能把多行消息当成sendkey参数。

```bash
BATCH_COMMAND="tap @${TARGET_X:?},${TARGET_Y:?}
sleep 500
check 设置
wait 10000 WLAN"
python3 "${CLIENT_PY}" --base-url "${API_URL}" --method POST /api/automation/act \
  --query "serial=${SERIAL}" --query "title=${TASK_TITLE:?缺少原任务标题}" --query "task_id=${TASK_ID:?缺少已开始任务ID}" --timeout 65 --text "$BATCH_COMMAND"
```

检查返回的完成数与失败原始行号，再全量observe核对实际页面。未完成的后续行不会自动执行，不重放已完成的操作。


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
