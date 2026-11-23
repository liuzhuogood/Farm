# v2026.11.12-feature 开发任务清单

状态：规划完成、待实施。

## FD1: 契约与接口原型定义
- [ ] 确立统一数据模型 `Screen`、`Element`、`Event` 及行协议序列化器（`Line()`、`Render()`）。
- [ ] 编写紧凑行格式化与反序列化单元测试，确保特殊字符与中文转义准确。

## FD2: 感知引擎与 Diff 差异对比实现
- [ ] 实现 `Observe` 核心逻辑，聚合 Accessibility 树与 OCR 输出，完成空间中心坐标自动计算。
- [ ] 实现 `Diff(prev, next *Screen)` 引擎，完成新增 (`+`)、移除 (`-`)、变更 (`~`) 行对比逻辑。
- [ ] 增加 `formats` 过滤器（支持 `ref`、`ocr` 及 `xml` 调试回退）。
- [ ] 编写感知与 Diff 的单测验证（含全量/增量回退边界测试）。

## FD3: 控制动作解析器与统一执行入口
- [ ] 实现伪控制语言解析器（支持 `tap(...)`、`type(...)`、`swipe(...)`、`key(...)` 的语法解析与参数绑定）。
- [ ] 暴露 `POST /api/automation/observe`、`POST /api/automation/act`、`POST /api/automation/batch` 路由端点。
- [ ] 动作执行后默认自动级联返回最新的 Screen 或 Diff 文本，闭环单次调用交互。
- [ ] 编写动作解析与执行的单元测试。

## FD4: 自动化文档收敛与全量门禁
- [ ] 更新 `configs/automation-apis.json`，清理历史分散接口，只展示核心自动化端点。
- [ ] 更新 OpenAPI / Swagger 文档契约定义，同步更新 `/api-docs-automation.html`。
- [ ] 执行全包后端回归测试：`go test ./internal/...`。
- [ ] 执行前端构建与类型校验：`pnpm --dir frontend build`。

## FD5: 原子提交与任务收口
- [ ] 按规范暂存并生成语义化 Conventional Commit。
- [ ] 汇总交付证据并归档版本记录。
