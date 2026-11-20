# v2026.11.21-feature 任务清单

设计：[2026-11-21-app-shell-navigation.md](../../designs/2026-11-21-app-shell-navigation.md)

## 待开发

- [ ] **S1 版本与设计记录**
  - 新建版本目录、设计文档与任务清单；`VERSION` 进位 `v2026.11.21`。

- [ ] **S2 全设备任务记录接口**
  - 新增只读 `GET /api/automation/task-records`，汇总运行中任务与 MCP 历史，按开始时间倒序。
  - 原 `/api/automation/tasks?serial=`、详情与结束接口保持不变。
  - 补 Go 测试与 OpenAPI。

- [ ] **S3 左侧菜单与页面**
  - `AppShell` + 可收起 `AppSidebar`，状态写入本地偏好。
  - 设备列表、任务记录、AI 集成、API 四个入口；设置底部展开。
  - 新增 `TaskRecordsView`、`ApiView`；首页与 AI 集成页移除顶部栏。

- [ ] **S4 设置页与依赖即时保存**
  - `/settings/general`、`/settings/service`、`/settings/dependencies` 三个独立页面。
  - 通用设置合并原投屏设置；移除 `SystemConfigDialog` 的主界面入口。
  - 依赖页首次加载骨架屏；配置与 OCR 修改防抖自动保存，移除保存按钮。

- [ ] **S5 桌面快捷操作与文档**
  - 桌面端新增“恢复所有小窗”命令；重启 ADB 走 `POST /api/adb/restart`。
  - 更新术语字典与相关页面指向。

- [ ] **S6 构建、自测与 beta 发布**
  - 本地 `pnpm --dir frontend build`、`go test ./...`、Rust 测试。
  - Mac mini（10.0.0.11）实机自测并留存证据。
  - beta 发布流水线支持 `v*-beta.N`，推送 `v2026.11.21-beta.1` 并核对 Release。

## 验收记录

| 步骤 | 结论 | 证据 |
| --- | --- | --- |
| S1 | 待验证 | — |
| S2–S6 | 待开工 | — |
