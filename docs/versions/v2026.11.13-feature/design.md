# v2026.11.13-feature 设计规范：AI 安全审核拦截、任务历史审计与原生 MCP 协议支持

## 1. 架构目标与背景

随着 Farm 自动化接口现代化重构（紧凑感知行协议与统一感知接口）的完成，AI Agent 已经能够以极高密度和极低延迟读取设备状态并下发动作。然而，当外部 Agent（如 Claude Code、Cursor、Codex 等）直接接管物理手机或远程设备时，面临三大关键安全与审计痛点：

1. **不可控的高危动作**：Agent 误点“确认转账”、“清空数据”、“同意授权”、“发送短信”等不可逆破坏性操作，缺乏人机回环（Human-in-the-loop）阻断机制。
2. **缺乏审计追踪**：任务执行过程分散在临时网络日志中，缺乏以“任务（Task）-步骤（Step）”为维度的结构化记录，没有操作前后的屏幕快照对比，事后无法复盘。
3. **生态接入协议割裂**：当前外部 AI 助手普遍采用 Anthropic 主导的 Model Context Protocol (MCP) 标准，而 Farm 之前仅暴露纯 HTTP/WebSocket 接口，缺乏开箱即用的原生 MCP Server 能力。

本版本借鉴开源先进竞品架构，在 Farm 中建立**完全独立于模型之外的网关级安全门禁与任务审计系统**：
- **安全策略引擎（Policy Engine）**：前置拦截高危动作（支付、转账、删除、发送、权限设置等），敏感输入（密码、验证码框）绝对阻断并强制要求人工接管。
- **人工审批中心（Approval Center）**：当动作被判定为 `Ask` 时，生成全局单次有效且绑定上下文的审批单，返回 `approval_pending` 挂起 Agent，通过 Farm 桌面端/Web 端弹窗由用户决策（允许一次/本次允许/始终允许/拒绝/接管）。
- **任务历史审计系统（Task History）**：以 SQLite + 缩略图存储任务完整生命周期，记录步骤前后截图、耗时、意图、审批流水，并支持一键导出自包含 HTML 离线报告。
- **原生 MCP 服务（Model Context Protocol）**：内嵌标准 MCP Server（支持 Streamable HTTP / SSE 与 stdio），提供规范的 `phone_*` 系列标准工具，无缝接入 Claude Desktop、Claude Code、Cursor 等全生态 Agent。

---

## 2. 系统核心架构

```mermaid
flowchart TD
    subgraph Agent [外部 AI Agent / 客户端]
        Claude[Claude Code / Desktop]
        Cursor[Cursor / Codex]
        CustomScript[自定义脚本 / SDK]
    end

    subgraph FarmServer [Farm 服务端核心]
        MCP[MCP Server (/mcp 或 stdio)]
        HTTPAPI[自动化 REST API (/api/automation/*)]

        subgraph Gateway [安全与审计网关]
            Policy[安全策略引擎 (Policy Engine)]
            Approval[人工审批中心 (Approval Center)]
            History[任务历史存储 (Task History)]
        end

        subgraph Core [底层控制与感知]
            Observe[紧凑感知 Observe]
            Action[动作执行 (SDK / ADB)]
            ScreenCap[屏幕帧截取]
        end
    end

    subgraph UI [用户交互界面]
        WebApproval[Web / 桌面端审批弹窗]
        ActivityUI[任务历史与回放界面]
    end

    Agent -->|JSON-RPC| MCP
    Agent -->|HTTP POST| HTTPAPI
    MCP --> Gateway
    HTTPAPI --> Gateway

    Policy -->|判定 Ask| Approval
    Policy -->|判定 Deny| Gateway
    Approval <-->|推送待审批 / 提交决策| WebApproval

    Gateway -->|执行合法动作| Action
    Gateway -->|采集前后快照| Observe
    Gateway -->|记录步骤与截图| History
    History <-->|查询 / 回放 / 导出| ActivityUI
```

---

## 3. 详细设计与核心组件

### 3.1 安全策略引擎 (`internal/safety/policy.go`, `engine.go`)

#### 规则匹配决策（Decisions）
- `Allow (0)`：安全动作，直接放行。
- `Ask (1)`：需人工确认的高危动作，向审批中心申请审批。
- `Deny (2)`：绝对禁止动作（如向密码框/验证码框注入文本、进入受限敏感应用等），立即返回拒绝。

规则匹配时遵循**最严格优先准则**：`Deny > Ask > Allow`。

#### 预设策略层级（Presets）
1. `PresetPayments (2)`：默认推荐预设。仅针对支付、转账、购买等直接涉及资产的动作发起审批。
2. `PresetCommits (1)`：除支付外，扩展到发送消息、提交订单、删除数据、拨打电话、修改系统设置、跳转外链等所有具有承诺性/破坏性的操作。
3. `PresetEvery (3)`：最严模式，每一个操作步骤均需人工审核批准。
4. `PresetNone (0)`：关闭预设，完全按自定义 rules 规则判定。

#### 敏感特征检测器
- **密码与验证码字段拦截**：若目标 Element 具备 `password=true`，或提示词包含密码/PIN/验证码等特征，禁止 Agent 通过 `type` 写入，强制提示调用 `phone_handoff` 或通知人工输入。
- **关键词与动作推导**：通过 Element 的 Label、Role 及上下文语境提取动作类别（`Payment`、`SendMessage`、`Commit` 等）。

---

### 3.2 人工审批中心 (`internal/safety/approval.go`)

1. **唯一性与防伪篡改绑定（Binding）**：
   - 每个审批单持有由安全随机数生成的 `ID`（格式 `ap_xxxxxx`）。
   - 绑定 `Device`（设备序列号）、`Tool`（操作类型）、`Args`（参数特征摘要）。
   - 严禁跨设备复用、跨参数伪造。
2. **生命周期与状态机**：
   - `Open`：等待用户决策。默认 60 秒超时，超时自动转为 `Deny`。
   - `Decided`：用户已作出决策（`AllowOnce` / `AllowTask` / `AllowAlways` / `Deny` / `TakeOver`），等待 Agent 调用方认领。
   - `Used`：决策已被认领执行，单次失效。
   - `Withdrawn`：任务被用户强制终止或主动撤回。
3. **Agent 轮询/挂起协议**：
   - 当遇到 `Ask` 时，接口立即返回状态 `approval_pending`，携带 `approval_id` 及说明理由 `intent`。
   - Agent 再次发起相同动作并附带该 `approval_id` 时，阻塞等待直至用户决策或超时。

---

### 3.3 任务历史与步骤审计 (`internal/safety/history.go`)

在本地 SQLite 数据库中记录完整审计流水：
- **`tasks` 表**：记录任务 ID、所属会话、目标设备、Agent 名称、任务标题、开始与结束时间、总步骤数、审批次数、最终状态（`done` / `failed` / `stopped`）。
- **`steps` 表**：记录步骤索引、工具名、调用参数 JSON、意图说明（intent）、执行耗时、操作前截图 ID、操作后截图 ID、目标元素坐标与命中标签。
- **`approvals` 表**：记录关联的任务 ID、步骤号、触发规则名称、标题与明细、决策结果、用户批注（note）、等待毫秒数。
- **`media` 表与磁盘目录**：保存步骤前后对比缩略图（JPEG 压缩，最长边 1280px，按年月归档）。
- **报告导出引擎**：支持按任务 ID 导出单文件自包含 HTML 审计报告（图片使用 base64 内嵌），包含时间线、步骤详情与审批证据。

---

### 3.4 原生 MCP 协议支持 (`internal/mcp/`)

完全遵循 Model Context Protocol 标准实现标准 MCP Server：

| 工具名称 | 对应底层能力 | 说明 |
| :--- | :--- | :--- |
| `phone_connect` | 会话建立与设备绑定 | 开启指定设备的任务会话，返回首屏感知数据 |
| `phone_observe` | 紧凑感知输出 | 返回当前屏幕扁平化行协议（支持 ref + ocr）与可选截图 |
| `phone_tap` | 点击操作 | 支持 ref 点击或 x,y 绝对像素坐标点击，受安全策略拦截 |
| `phone_type` | 文本输入 | 向指定 ref 或当前焦点输入文本，密码框受限保护 |
| `phone_swipe` | 滑动操作 | 支持上下左右方向滑动或起点/终点像素滑动 |
| `phone_key` | 按键模拟 | 支持 Home、Back、AppSwitch、Enter 等系统导航按键 |
| `phone_launch` | 应用打开 | 启动指定包名应用或系统 Activity |
| `phone_handoff` | 人工接管交互 | 当需要输密码、过滑块验证码或解锁屏幕时挂起并交还人工 |
| `phone_request_approval` | 主动请求确认 | Agent 主动向用户提问并获取 Yes/No 决策 |
| `phone_release` | 结束任务并释放设备 | 结束任务会话，写入完成摘要，可选触发导出离线报告 |

---

## 4. 接口契约补充

- `POST /mcp`：标准 Streamable HTTP / SSE MCP 协议通信端点。
- `GET /api/safety/approvals`：获取当前待决策的审批列表。
- `POST /api/safety/approvals/{id}/decide`：用户提交决策（`allow_once`、`allow_task`、`deny` 等）。
- `GET /api/safety/history/tasks`：分页查询历史任务审计列表。
- `GET /api/safety/history/tasks/{id}`：获取单个任务详细步骤与时间线。
- `GET /api/safety/history/tasks/{id}/export`：下载单任务自包含 HTML 审计报告。
- `GET/PUT /api/safety/policy`：查询与更新当前安全审核配置（预设等级、黑名单等）。
