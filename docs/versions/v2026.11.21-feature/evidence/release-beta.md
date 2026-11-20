# v2026.11.21-beta.1 发布验收

- 发布时间：2026-11-21 03:16（Asia/Shanghai）
- Release：https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.21-beta.1
- Actions：https://github.com/liuzhuogood/Farm/actions/runs/44797883718
- 标签提交：`51be32c457a7d544705d18649ee8abe61c1d0fcd`
- 发布类型：预发布；Docker 与 PyPI 发布按 beta 规则跳过
- 正式最新版：仍为 `v2026.11.17-1`

## 产物

Release 共 14 个资产，包含：

- Linux / macOS / Windows CLI：5 个
- Windows GUI 安装器与独立 GUI：2 个
- macOS arm64 / amd64 DMG：2 个
- macOS 自动更新包及签名：4 个
- `desktop-latest.json`：1 个

`desktop-latest.json` 的版本为 `v2026.11.21-beta.1`，包含 `darwin-aarch64`、`darwin-x86_64`、`windows-x86_64` 三个平台。

## Mac mini 复测

在 `10.0.0.11` 下载发布版 `farm-macos-arm64-cli`：

- SHA-256：`ed8860eac7e95bd0606fd3a9bd5e94d47877b8f5db05f763f2c2029a38b33c4a`
- `--version`：`v2026.11.21-beta.1`
- `/api/version/health`：`v2026.11.21-beta.1`
- `/`、`/tasks`、`/settings/dependencies`：HTTP 200，`text/html`
- 发布版依赖页无“保存”按钮，直接访问和刷新正常
- 发布版任务记录页截图：`macmini/release-beta-tasks.png`
- 发布版依赖页截图：`macmini/release-beta-dependencies.png`

macOS arm64 DMG：

- `CFBundleShortVersionString` / `CFBundleVersion`：`2026.11.21-beta.1`
- `codesign --verify --deep --strict`：通过

## 未覆盖

- 本次未在 Mac mini GUI 会话中覆盖安装 DMG，避免影响现有 `/Applications/Farm.app`。
- “恢复所有小窗”依赖 Tauri 桌面运行时，本次 Mac mini 浏览器复测未触发该命令；桌面命令与权限测试已在本机通过。
