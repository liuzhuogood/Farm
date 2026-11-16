# v2026.11.16-1：设备维护接口、macOS 托盘图标与投屏小窗交互升级

## 1. 版本定位与概述

本版本基于 `v2026.11.16`（`9b43ed0`）之上的 3 个功能提交：新增设备重启 ADB 服务接口与桌面菜单入口、macOS 托盘改用独立纯白透明图标、首页设备卡片默认进入设备详情页并新增打开小窗入口。发布前完成 Go 全量单测、Python SDK 100 项单测、前端生产构建与 LZAUTO 三维度 17 项全流程端到端复验，全部通过，无需修复。

## 2. 当前版本完成记录

### 已完成 (Completed)
- [x] **设备：重启 ADB 服务接口**：新增重启 ADB 服务接口与桌面菜单入口（`2a0854e`），`internal/device` 管理器配套单测通过，OpenAPI 契约已同步（`web/openapi.json`）。
- [x] **桌面：macOS 托盘图标重绘**：托盘改用独立纯白透明图标（`da31751`），新增 `render-tray.swift` 渲染脚本与原始素材，深浅色菜单栏下均清晰可见。
- [x] **投屏：首页卡片与小窗交互升级**（`aa44893`）：
  - 首页卡片（deck/phone 视图）点击改为进入设备详情页，小窗改由详情页创建；
  - 详情页工具栏最前面新增"打开小窗"，返回、主页、任务切换前置并用分隔符分组；
  - 小窗顶部把手仅在指针进入顶部 24px 热区时出现，避免遮挡画面；
  - 新增 `useFloatingPlayers` 共享浮窗列表，详情页与首页共用同一份小窗状态；
  - 术语表同步投屏小窗、小窗顶栏与手机卡片视图指向。
- [x] **Go 全量单测**：`go test ./...` 全部通过（含 `internal/device` 新增单测与 `packages/python-sdk` 契约测试）。
- [x] **Python SDK 单测**：`packages/python-sdk` 100 项测试全量通过（`python -m unittest discover`）。
- [x] **前端生产构建**：`frontend` Vite 构建成功，产物无错误。
- [x] **LZAUTO 全流程端到端测试**：`scripts/run_comprehensive_e2e_tests.py` 三维度 17 项用例全部 PASS：
  1. UI & 技能集成：AI 集成页加载与契约面板展示、动作速查与命令示例、基础技能一键安装（3 项）；
  2. 技能核心协议：契约读取与在线设备发现、任务创建、统一感知、多行纯文本动作批次、文本注入与组合按键、产物生成下载、任务核对与结束闭环（7 项）；
  3. 原生 MCP 与安全网关：握手自省、工具自省、设备会话建立、高危操作拦截与审批派发、人工审批放行、密码凭据框绝对阻断、任务归档与 HTML 报告导出（7 项）。
- [x] **验证证据刷新归档**：`docs/verification/2026-11-14-ai-e2e-report.html`、Markdown 报告与 `evidence-2026-11-14/` 截图、日志按本次复验结果更新。
- [x] **版本一致性**：`VERSION` 与 `frontend` / `desktop` / `python-sdk` 各包版本同步为 `v2026.11.16-1`，`node scripts/sync-version.mjs --check` 校验通过。

### 待开发 (Pending / Planned)
- [ ] **多平台设备底层契约扩展**：iOS（WebDriverAgent）与鸿蒙等更多设备底层的通用契约与统一感知适配。
- [ ] **技能市场在线生态与订阅**：自动化技能市场的动态在线拉取、第三方社区源订阅与热更新。
- [ ] **视觉辅助自适应降级**：目标应用缺失 Accessibility 节点树或防抓取时，自动切换纯视觉 OCR 坐标推断执行。
- [ ] **多模态视觉流水线组合**：原生 MCP 工具集中 Template / YOLO / VLM 的动态流水线组合与复杂场景感知。

## 3. 验证与交付

- **测试结论**：Go、Python SDK、前端构建与 17 项 LZAUTO 端到端用例全部通过，本轮未发现需修复问题。
- **证据归档**：`docs/verification/`（自包含 HTML 报告、Markdown 概要与截图日志证据）。
- **发布方式**：推送 `v2026.11.16-1` 标签触发 `release.yml`，完成五平台 CLI 构建、签名桌面包、macOS DMG、Release 与 Docker 发布。
- **未验证范围**：Windows 签名桌面包与更新器签名在 CI 侧产物验证；macOS DMG 由 `macos-dmg.yml` 远端构建，本机未复跑签名安装流程。
