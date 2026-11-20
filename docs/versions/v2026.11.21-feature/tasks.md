# v2026.11.21-feature 任务清单

设计：[2026-11-21-app-shell-navigation.md](../../designs/2026-11-21-app-shell-navigation.md)

## 任务

- [x] **S1 版本与设计记录**
  - 新建版本目录、设计文档与任务清单；`VERSION` 进位 `v2026.11.21`。

- [x] **S2 全设备任务记录接口**
  - 新增只读 `GET /api/automation/task-records`，汇总运行中任务与 MCP 历史，按开始时间倒序。
  - 原 `/api/automation/tasks?serial=`、详情与结束接口保持不变。
  - 补 Go 测试与 OpenAPI。

- [x] **S3 左侧菜单与页面**
  - `AppShell` + 可收起 `AppSidebar`，状态写入本地偏好。
  - 设备列表、任务记录、AI 集成、API 四个入口；设置底部展开。
  - 新增 `TaskRecordsView`、`ApiView`；首页与 AI 集成页移除顶部栏。

- [x] **S4 设置页与依赖即时保存**
  - `/settings/general`、`/settings/service`、`/settings/dependencies` 三个独立页面。
  - 通用设置合并原投屏设置；移除 `SystemConfigDialog` 的主界面入口。
  - 依赖页首次加载骨架屏；配置与 OCR 修改防抖自动保存，移除保存按钮。

- [x] **S5 桌面快捷操作与文档**
  - 桌面端新增“恢复所有小窗”命令；重启 ADB 走 `POST /api/adb/restart`。
  - 更新术语字典与相关页面指向。

- [ ] **S6 构建、自测与 beta 发布**
  - [x] 本地 `pnpm --dir frontend build`、`go test ./...`、Rust 测试。
  - [x] 修复生产二进制直接访问 `/tasks`、`/settings/*` 等路由时的 SPA fallback。
  - [x] Mac mini（10.0.0.11）实机自测并留存证据。
  - beta 发布流水线支持 `v*-beta.N`，推送 `v2026.11.21-beta.1` 并核对 Release。

## 验收记录

| 步骤 | 结论 | 证据 |
| --- | --- | --- |
| S1 | 通过 | `docs/versions/v2026.11.21-feature/version.md`、`docs/designs/2026-11-21-app-shell-navigation.md` |
| S2 | 通过 | `go test ./...`；`internal/automation/task_records_test.go` |
| S3 | 通过 | 本机与 Mac mini 实测侧栏展开/收起、四个主入口；`evidence/macmini/{home,tasks,tasks-detail,api,sidebar-collapsed}.png` |
| S4 | 通过 | `evidence/macmini/{settings-menu,settings-general,settings-service,settings-dependencies,settings-dependencies-skeleton}.png` |
| S5 | 通过 | 桌面命令与权限测试通过；Mac mini 浏览器实测“重启 ADB 服务”，设备重新发现；`evidence/macmini/restart-adb.png` |
| S6 构建与自测 | 通过 | 前端构建、完整 Go 测试、Rust 测试、版本同步测试；`evidence/local-smoke/`、`evidence/macmini/` |
| S6 beta 发布 | 待发布 | 推送 tag 后核对 GitHub Release |
