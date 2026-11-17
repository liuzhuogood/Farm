# v2026.11.17-1：应用内检查更新与自动更新设置

## 1. 版本概述

在既有桌面更新器能力之上补全用户可用的更新入口与设置：原生菜单/托盘手动检查更新、系统设置中的检查与自动更新开关、底部胶囊提示替代首页顶栏更新标签、统一更新弹窗并附 GitHub 发布页链接。同时收敛平台缺少更新包时的错误提示，并修复弹窗关闭语义与陈旧进度条。

## 2. 主要变更

- 新增 `desktop/src/menu.rs` 的 `check-update` 菜单项（macOS 主菜单与托盘同名项），唤醒主窗口并通知前端检查更新。
- 新增 `frontend/src/composables/useUpdateSettings.ts`：`检查更新`、`自动更新` 持久化，关闭前者时后者同步关闭。
- 新增 `frontend/src/components/updater/UpdateCenter.vue`：底部胶囊提示 + 菜单事件监听 + 启动检查。
- `SystemConfigDialog` 通用设置新增“更新”开关组；`AppHeader` 移除版本胶囊与更新徽标。
- `UpdateNoticeDialog` 统一“软件更新”标题、左下 GitHub 发布页、按状态切换操作按钮。
- `stores/updater.ts` 增加 `initialize`、`checkAndOpen`、`dismissedVersion`，并在检查开始时重置升级进度。
- `desktop/src/updater.rs` 将 `ReleaseNotFound`、`TargetNotFound(s)` 转为用户可读原因。
- 发布侧：`release-assets.mjs`、`desktop-manifest`、`release.yml`、`macos-dmg.yml` 支持 macOS `.app.tar.gz` 与签名。

## 3. 验证

- `node --test frontend/tests/updateSettings.test.mjs`：4 项通过。
- `node scripts/verification/release-manifest.mjs`：平台文件名、manifest 合并、双架构分离、缺件拒绝通过。
- `node --test scripts/sync-version.test.mjs`：2 项通过；`node scripts/sync-version.mjs --check` 版本一致。
- `pnpm --dir frontend build` 与 `go test ./...` 通过。
- `--verify-native` 构建后 `desktop-update-menu.mjs` 与 `desktop-native.mjs` 通过。
- 隔离服务下 `/api/version/check` 返回 `current=v2026.11.17-1`，GitHub Release 真实可达。
- 隔离实例实测设置开关联动与刷新后持久化。

## 4. 未验证范围

- 真实签名更新包的下载、验签、安装与重启由发布后另行实测。
- Apple 正式签名与公证未配置，macOS 仍为 ad-hoc 签名。
- Windows/Linux 安装器与真实设备验收不覆盖。
