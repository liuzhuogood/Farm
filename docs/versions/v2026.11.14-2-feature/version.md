# v2026.11.14-2：AI 自动化技能文档契约面板、安装合一与端到端自动化验收

## 1. 版本定位与概述

本版本在 `v2026.11.14-1` 的基础上，完善了 AI 集成模块的技能文档契约面板与基础技能安装合一交互，强化了 MCP 服务安全网关的语义感知拦截能力，并建立了覆盖全流程的自动化端到端测试与自包含 HTML 审计报告体系。

## 2. 当前版本完成记录

### 已完成 (Completed)
- [x] **AI 技能文档契约面板 (`SkillContractPanel.vue`)**：
  - 在 AI 集成页建立统一契约面板，以 `/fram.skill` 为唯一权威来源，整合展示设备发现、任务生命周期、统一感知与多行纯文本动作协议规范。
  - 支持动作速查（`tap`、`swipe`、`keyevent`、`sendkey`、`sleep`、`check`、`home`、`back`、`app_start`、`app_stop` 等）与实时命令示例切换、复制提示词。
  - 基础设备技能与安装合一：直接展示基础技能安装状态，支持一键安装至本地 AI 助手环境（Pi / Claude Code / Codex）。
- [x] **统一感知与动作协议增强**：
  - 技能描述与文档契约更新（`internal/skillsmarket/catalog.go`）。
  - `internal/mcp/server.go`：安全评估拦截支持意图与标签感知 `policyIn.ElementLabel = in.Intent`，防止向密码/凭据框输入。
  - 术语对齐：更新 `docs/glossary.md`，登记「自动化技能文档面板」。
- [x] **全流程自动化端到端测试套件 (LZAUTO)**：
  - 编写 `scripts/run_comprehensive_e2e_tests.py`，覆盖三大测试维度共 17 个端到端用例：
    1. UI & 技能集成：AI 集成页加载与契约面板展示、动作速查与命令示例查看、基础技能一键安装；
    2. 技能核心协议：契约读取与在线设备发现、任务创建、统一感知、多行纯文本动作批次、文本注入与组合按键、产物生成下载、任务核对与结束闭环；
    3. 原生 MCP 服务与网关级安全门禁：MCP 握手自省、设备会话建立、高危敏感操作拦截与审批单派发、人工审批放行、密码凭据框绝对阻断、任务归档与自包含 HTML 报告导出。
  - 生成自包含 HTML 审计与证据报告 `docs/verification/2026-11-14-ai-e2e-report.html` 及配套证据截图/日志。
- [x] **跨平台环境与单测稳定性**：
  - 适配 Windows 环境下单测文件权限与命令行可执行文件识别（`artifacts_http_test.go`、`tasks_persistence_test.go`、`service_test.go`、`manager_test.go`）。
  - 全平台发布流程与 CI 检查项验证。

### 待开发 (Pending / Planned)
- [ ] **多平台设备底层契约扩展**：针对 iOS（WebDriverAgent）与鸿蒙等更多设备底层的通用契约与统一感知适配。
- [ ] **技能市场在线生态与订阅**：支持自动化技能市场的动态在线拉取、第三方社区源订阅与热更新。
- [ ] **视觉辅助自适应降级**：在目标应用缺失 Accessibility 节点树或防抓取时，自动切换为纯视觉 OCR 坐标推断执行。
- [ ] **多模态视觉流水线组合**：在原生 MCP 工具集中支持 Template / YOLO / VLM 的动态流水线组合与复杂场景感知。

## 3. 验证与交付

- **全流程端到端自动化测试**：执行 `scripts/run_comprehensive_e2e_tests.py`，17 项用例全部 **PASS**，证据与自包含 HTML 报告归档至 `docs/verification/`。
- **Go 单元与回归测试**：`go test ./...` 与 `go test -count=3 ./internal/db` 全量通过。
- **Python SDK 完整测试**：`packages/python-sdk/tests` 100 项单测全量通过。
- **前端与桌面编译**：`frontend` 生产构建成功，`tauri` / `Cargo` 嵌入服务与打包检查通过。
- **版本一致性**：`node scripts/sync-version.mjs --check` 与 `node --test scripts/sync-version.test.mjs` 校验通过。
