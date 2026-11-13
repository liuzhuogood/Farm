# v2026.11.12-feature 开发任务清单

状态：进行中（FD1、FD2、统一感知接口、AI 自动化接口日志、macmini 端到端验收及自动化文档服务端渲染已完成）。

## FD1: 契约与接口原型定义
- [x] 确立统一数据模型 `Screen`、`Element`、`Event` 及行协议序列化器（`Line()`、`Render()`）。
- [x] 编写紧凑行格式化与反序列化单元测试，确保特殊字符与中文转义准确。

## FD2: 感知引擎与 Diff 差异对比实现
- [x] 实现 `Observe` 核心逻辑，聚合 Accessibility 树与 OCR 输出，完成空间中心坐标自动计算。
- [x] 实现 `Diff(prev, next *Screen)` 引擎，完成新增 (`+`)、移除 (`-`)、变更 (`~`) 行对比逻辑。
- [x] 增加 `formats` 过滤器（支持 `ref`、`ocr` 及 `xml` 调试回退）。
- [x] 编写感知与 Diff 的单测验证（含全量/增量回退边界测试）。

## FD3: 控制动作解析器与统一执行入口
- [ ] 实现伪控制语言解析器（支持 `tap(...)`、`type(...)`、`swipe(...)`、`key(...)` 的语法解析与参数绑定）。
- [x] 暴露 `POST /api/automation/observe` 路由端点。
- [ ] 动作执行后默认自动级联返回最新的 Screen 或 Diff 文本，闭环单次调用交互。
- [x] 编写动作解析与执行的单元测试。

## FD4: 自动化文档收敛与全量门禁
- [x] 更新 `configs/automation-apis.json`，将页面收敛为 13 个核心接口：感知、日志、设备上下文、动作控制与视觉条件操作。
- [x] 更新 OpenAPI / Swagger 文档契约定义，同步更新 `/api-docs-automation.html`。
- [x] 将 `/api-docs-automation.html` 改为服务端渲染的精简 Markdown-like 文本；参数与紧凑请求/响应 Schema 直接包含在 HTML 响应中，无客户端脚本或二次请求。
- [x] 执行全包后端回归测试：`go test ./internal/...`。
- [ ] 执行前端构建与类型校验：`pnpm --dir frontend build`。

## FD5: 原子提交与任务收口
- [ ] 按规范暂存并生成语义化 Conventional Commit。
- [ ] 汇总交付证据并归档版本记录。

## FD6: AI 自动化接口日志与 macmini 端到端验收
- [x] 新增 `internal/automation/logging.go`：结构化日志（标准输出 + 内存环形缓冲 1024 条）。
- [x] 覆盖七类事件：`automation.request` / `automation.observe` / `automation.observe.stage` / `automation.diff` / `automation.action` / `automation.response` / `automation.error`。
- [x] 暴露 `GET /api/automation/logs` 自省端点（支持 `serial` / `type` / `limit` 过滤）。
- [x] 补充 `internal/automation/logging_test.go`（6 个日志用例，全部通过）。
- [x] macmini 端到端验收：Pixel_7_2 模拟器（Android 14，1080x2400）+ Indra OCR sidecar，13 个用例全部通过。
- [x] 归档验收报告与日志切片至 `evidence/` 目录。

### FD6 验收结论

| 用例 | 验证项 | 结论 |
| --- | --- | --- |
| T1 | 全量感知 ref + ocr 双通道 | PASS |
| T2 | XML 调试回退 formats=["xml"] | PASS |
| T3 | 增量 Diff（画面无变化） | PASS |
| T4 | Diff 降级（前台应用切换） | PASS |
| T5 | 焦点与键盘 focused editable 标记 | PASS |
| T6 | 增量 Diff（滚动产生 +/-/~） | PASS |
| T7 | switch 状态变化 → ~ 变更行 | PASS |
| T8 | formats 过滤（仅 ref） | PASS |
| T9 | 动作接口与动作日志 | PASS |
| T10 | 错误路径（缺少 serial） | PASS |
| T11 | 错误路径（不存在的设备） | PASS |
| T12 | 日志自省接口 /api/automation/logs | PASS |
| T13 | AI 闭环 observe → 定位 → tap → observe(diff) | PASS |

报告：[evidence/macmini-e2e-report.html](evidence/macmini-e2e-report.html)（截图与日志已内联，可离线分发）。

自动化接口文档已改为服务端渲染：`/api-docs-automation.html` 返回紧凑 HTML 文本，接口签名、参数与请求/响应 Schema 均在 HTML 响应中，无客户端脚本或二次请求。


## 2026-11-13：按用户确认改用文本技能协议

本节覆盖早期伪函数/Schema展示方案的当前落地状态，原规划未勾选项不视为已实现。

- [x] 新增 `/api/automation/devices`，一行一个在线设备；保留原 `/api/devices`。
- [x] 新增 `/api/automation/act?serial=...`，正文每行“操作 参数”，支持八种操作、像素坐标、多键分隔符与多行顺序执行。
- [x] 全语法预校验、失败即停、失败行/完成数、每设备互斥、有界容量、取消与超时。
- [x] Unicode sendkey 走已有 SDK 剪贴板粘贴，旧输入接口保持。
- [x] 新入口 SDK/HID 滑动按请求时长发送中间触点，取消时收尾抬起；SDK真机截图确认实际滚动，旧调用分支保留。
- [x] 技能页收敛至四个 AI 专用入口，移除 Schema，补选设备/观察/操作/核对、单步/多步/输入/失败/curl 示例。
- [x] 四份技能及请求客户端迁移至文本正文、GET observe 与明确错误处理。
- [x] 自动化、文档、技能包及根包相关测试通过；浏览器与 Redmi 验证见验证记录。
- [x] 更新本机1989后端并保留原App备份。
- [ ] 旧 Companion 读树/回退后恢复超时的独立排查（本轮观察到，未修改该已有模块）。
- [ ] 提交与推送（未获此项指令）。
