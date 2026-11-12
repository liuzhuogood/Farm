# v2026.11.12-feature：现代自动化控制套件与紧凑感知接口重构

## 状态
- 2026-11-12 规划建立，进入特性设计与实现工作流。
- 承接自动化接口革新需求：借鉴行业与竞品（pinky）最佳实践，彻底重构针对 AI Agent 友好的自动化感知与控制体系。
- 2026-11-12 FD1、FD2、统一感知接口（`POST /api/automation/observe`）、自动化接口日志与 macmini 端到端验收已完成。
- `/api-docs-automation.html` 已收敛为服务端渲染的精简 Markdown-like 接口说明，页面直接包含筛选后接口的参数与请求/响应 Schema。

## 目标与背景
现有的 `/api-docs-automation.html` 聚合了过多历史零散接口（共 68 个端点，涵盖零散动作、文件传输、模板匹配、OCR 启停等），且数据返回格式多为冗长嵌套的 JSON，导致 AI Agent 在长会话中面临严重的 Token 膨胀、坐标运算幻觉与高交互延迟。

本版本核心目标是打造极简、紧凑、专为 AI Agent 设计的现代自动化套件：
1. **感知接口（数据表示层）**：单接口支持按需配置输出紧凑协议（`ref` 元素树 + `ocr` 视觉补全），彻底抹平传统 JSON 冗余；明确 XMLDump 定位（仅作内部解析提取与低级调试，绝不直接喂给模型）。
2. **控制接口（伪指令动作层）**：统一控制入口，支持解析高亲和伪控制指令（如 `tap(ref=12)`、`tap(x=540, y=960)`、`type('admin', ref=2)`），支持批量执行。
3. **上下文优化（Diff 增量机制）**：引入动作后增量感知对比，连续操作只返回 `+`、`-`、`~` 差异行，大幅降低 Agent 上下文开销。
4. **自动化文档收敛**：更新 `configs/automation-apis.json`，让 `/api-docs-automation.html` 只展示聚焦、纯粹的现代化自动化套件接口。

## 交付文档
- [设计文档](design.md)：系统架构、行协议规范、伪指令解析器、Diff 算法与接口契约设计。
- [任务清单](tasks.md)：端到端研发、单元测试、文档收敛与门禁验证任务。

## 已交付能力

### 紧凑感知协议
统一数据模型 `Screen` / `Element` / `OCRItem` / `Event`，行协议序列化与反序列化对称可逆：

```text
screen 1080x2400 · app: Settings (com.android.settings) · portrait · keyboard hidden · focus [2]
[2] input "搜索设置" @603,220 focused editable
[19] switch "深色主题" @943,1773 checked
ocr "已连接到 5G_WiFi" @320,480
```

### 统一感知接口
`POST /api/automation/observe` 聚合 Accessibility 树与 OCR 视觉，按 `formats` 过滤（`ref` / `ocr` / `xml`），
支持 `diff` 增量对比与 `settle_ms` 画面静止等待。OCR 文本与 Accessibility 节点做空间去重。

### Diff 差异对比引擎
多轮最优匹配（ResourceID → Bounds → 语义标签与中心距离），输出 `+` 新增 / `-` 移除 / `~` 变更；
在应用切换、屏幕旋转、变更行数超阈值时自动降级为全量输出。

### AI 自动化接口日志
自动化接口输出结构化日志（标准输出 + 内存环形缓冲），覆盖请求、阶段、差异、动作、响应与错误七类事件；
新增 `GET /api/automation/logs` 自省端点，支持按 `serial` / `type` / `limit` 过滤。

### AI 自动化接口文档
`/api-docs-automation.html` 由服务端读取筛选后的 OpenAPI 规范并渲染为精简文本，包含接口路径、方法、参数签名、请求 Schema 与响应 Schema。
浏览器直接收到完整 HTML 内容，不需要执行 JavaScript 或二次请求；自动化清单收敛至 13 个已实现接口。
当前点击、滑动、按键与视觉条件操作通过现有接口暴露；统一伪指令入口 `/api/automation/act` 尚未实现，文档不会将其列为可调用能力。

### 端到端验收
macmini（10.0.0.11）本地完成 13 个真实设备验收用例（Pixel_7_2 模拟器 / Android 14 / 1080x2400 / Indra OCR sidecar），
全部通过。报告见 [evidence/macmini-e2e-report.html](evidence/macmini-e2e-report.html)。
