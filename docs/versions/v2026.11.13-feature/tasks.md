# v2026.11.13-feature 开发任务清单

状态：已完成 (FD1 至 FD6 全量通过并完成真实环境端到端验收)。

## FD1: 安全策略引擎（Policy Engine）实现
- [x] 定义安全决策模型（`Decision`、`Rule`、`Preset`、`Verdict`、`Input`）。
- [x] 实现特征识别器（支付/购买关键词识别、敏感密码框检测 `IsCredentialField`、高危操作检测）。
- [x] 实现最严优先匹配准则与预设策略切换（`PresetPayments` / `PresetCommits` / `PresetEvery` / `PresetNone`）。
- [x] 编写策略引擎完整单元测试（覆盖多语言关键词、密码框绝对拦截、预设模式切换边界）。

## FD2: 人工审批中心（Approval Center）实现
- [x] 实现安全审批状态机（`Open` -> `Decided` -> `Used` / `Withdrawn` / `Expired`）。
- [x] 实现调用上下文强校验绑定（`Device` + `Tool` + `Args` 哈希），防止参数篡改与跨设备重放。
- [x] 实现超时自动拒绝机制（默认 60s）与并发安全等待通道（`Wait` / `Decide` / `Withdraw`）。
- [x] 编写审批中心单元测试（覆盖单次有效性、过期销毁、并发竞争与重复认领防护）。

## FD3: 任务历史审计存储与离线导出
- [x] 设计并初始化 SQLite 数据表结构（`tasks`、`steps`、`approvals`、`media`）。
- [x] 实现步骤记录与屏幕快照归档（最长边 1280px 降采样 JPEG 存储）。
- [x] 实现任务审计检索（支持关键字、状态、时间范围过滤）。
- [x] 实现自包含单文件 HTML 任务报告导出引擎（内联 base64 截图与审批时间线）。
- [x] 编写任务存储与导出单元测试。

## FD4: 原生 MCP 服务（Model Context Protocol）接入
- [x] 引入/实现标准 MCP JSON-RPC 通信协议（支持 Streamable HTTP / SSE 与 stdio 双模式）。
- [x] 注册并实现核心工具映射（`phone_connect`、`phone_observe`、`phone_tap`、`phone_type`、`phone_swipe`、`phone_key`、`phone_launch`、`phone_handoff`、`phone_request_approval`、`phone_release`）。
- [x] 在 MCP 工具执行流中前置挂载 Policy 评估与 Approval 拦截，完整闭环 `approval_pending` 与重放协议。
- [x] 编写 MCP 工具集契约测试与 JSON-RPC 交互测试。

## FD5: HTTP 路由与自省接口暴露
- [x] 暴露审批交互端点（`GET /api/safety/approvals`、`POST /api/safety/approvals/{id}/decide`）。
- [x] 暴露历史查询与导出端点（`GET /api/safety/history/tasks`、`GET /api/safety/history/tasks/{id}/export`）。
- [x] 暴露安全策略配置端点（`GET/PUT /api/safety/policy`）。
- [x] 在 `main.go` 中完成各子服务装配并注入现有数据通道。

## FD6: macmini 端到端验收与测试报告生成
- [x] 在真实环境（macmini + Android 模拟器/设备）上启动服务与 MCP 端点。
- [x] 构造高危敏感操作（如模拟支付、密码输入），验证审批阻断、桌面/API决策与放行全流程。
- [x] 执行端到端完整任务，记录前后步骤快照，生成结构化日志与截图证据。
- [x] 导出真实任务 HTML 报告并做自检渲染校验。
- [x] 归档验收证据到 `docs/versions/v2026.11.13-feature/evidence/`。

### 验收产物
- 全局验收报告：`docs/versions/v2026.11.13-feature/evidence/macmini-e2e-report.html`
- 单任务审计报告：`docs/versions/v2026.11.13-feature/evidence/farm-task-example.html`
- 端到端测试脚本：`scripts/test_mcp_e2e.py`
