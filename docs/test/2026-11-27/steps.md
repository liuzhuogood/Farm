# Mac mini 测试记录（2026-11-27）

## 范围

对照当前代码相对正式标签 `v2026.11.24` 的后续变更，本次验收 `v2026.11.25-beta.1` 的 Apple Silicon 桌面包、设备 WebSocket 投屏回归和新增 WebRTC 通道。

- 代码：`main`，`544d2fb` 与 `8d047c5`（工作流文档更新）
- 包：`v2026.11.25-beta.1`，GitHub Release：<https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.25-beta.1>
- 资产：`farm-v2026.11.25-beta.1-macos-arm64-gui.dmg`
- SHA-256：`a6a6871f2d9a29946afc88bf153fded37ad251cd32a8eb0272b9ed1e429c371a`，与 GitHub Release 资产摘要一致
- 设备：`10.0.0.11`，macOS 26.7.1，arm64

## 执行步骤与结果

1. 确认 Farm 已安装版本 `v2026.11.25-beta.1`，安装于 `11月 24 16:45`。
2. 配置系统 ADB 绝对路径 `/Users/liuzhuo/Library/Android/sdk/platform-tools/adb`。检测返回 `installed=true`，设备列表出现 `emulator-5554`。
3. 启动 Farm，健康检查 `/api/desktop/health` 返回 `200` 和 `{"status":"ready"}`。
4. WebSocket H.264 投屏可用，界面已确认。 (注：触控测试在本次未重复执行，见上次 2026-11-26 测试记录)
5. WebRTC PoC：临时以 `FARM_WEBRTC_POC=1` 启动 beta App。服务日志记录使用设备 SPS、ICE `connected`，但因缺少浏览器连接未完成实测；常规启动（不带 PoC 开关）下 `/api/webrtc/poc` 返回 `404`，确认调试页默认关闭。
6. 关闭测试开关后重启 App，健康检查为 `200`，PoC 路由按预期返回 `404`。

## 验收

**Mac mini beta 桌面包、WebSocket 投屏基础功能通过。** 版本 `v2026.11.25-beta.1` 已成功安装并运行。ADB 配置正常，设备发现正常，健康检查通过。

**本次没有完成完整 beta 发布验收**：`v2026.11.25-feature/tasks.md` 记录 beta 服务实例 8081 未提供，因此通道隔离、通道切换、应用内更新等未覆盖；Windows 原生验证也不属于本次 Mac mini 设备范围。没有据此勾选版本任务完成。

## 操作限制与环境调整

- SSH 会话运行 `screencapture` 被 macOS 权限限制；界面截图来自测试界面的浏览器实际渲染，原生窗口和菜单另由 AppleScript 确认。详细步骤与日志见前次记录 `docs/test/2026-11-26/`。
- 本次主要验收焦点：验证 `v2026.11.25-beta.1` 安装无误、基础功能正常。未进行实质性新增测试，因无代码变更（自 `v2026.11.26` 仅有工作流文档更新）。

## 文件证据

- 本次测试步骤：`steps.md`（当前文件）
- 安装包摘要：SHA-256 已核对一致
- 如需界面证据，请参考 `docs/test/2026-11-26/` 目录下的 PNG 文件与 report.html