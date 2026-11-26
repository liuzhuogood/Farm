# Mac mini 测试记录（2026-11-26）

## 范围

这是仓库首次保存到 `docs/test/` 的 Mac mini 测试记录，之前没有可读取的上次测试日期。对照当前代码相对正式标签 `v2026.11.24` 的后续变更，本次重点验收 `v2026.11.25-beta.1` 的 Apple Silicon 桌面包、设备 WebSocket 投屏回归和新增 WebRTC 通道。

- 代码：`main`，`544d2fb`
- 包：`v2026.11.25-beta.1`，GitHub Release：<https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.25-beta.1>
- 资产：`farm-v2026.11.25-beta.1-macos-arm64-gui.dmg`
- SHA-256：`a6a6871f2d9a29946afc88bf153fded37ad251cd32a8eb0272b9ed1e429c371a`，与 GitHub Release 资产摘要一致
- 设备：`10.0.0.11`，macOS 26.7.1，arm64；Android 测试设备 `emulator-5554`

## 执行步骤与结果

1. 记录原安装版本 `2026.11.17-1`，从 GitHub 下载 beta.1 arm64 DMG，并核对 SHA-256。
2. 正常退出旧版，安装到 `/Applications/Farm.app`，启动新 App。Bundle 版本和构建号均为 `2026.11.25-beta.1`；桌面服务 `/api/desktop/health` 返回 `200` 和 `{"status":"ready"}`。AppleScript 确认 Farm 原生窗口存在，原生“窗口”菜单含“恢复主窗口”等项目。
3. 首次打开时设备页提示 ADB 不可用。Mac mini 的 Android SDK ADB 和在线模拟器均存在；桌面应用没有继承 shell 的 PATH。按依赖设置已确认的结构，通过受认证的依赖配置接口设定系统 ADB 绝对路径 `/Users/liuzhuo/Library/Android/sdk/platform-tools/adb`。检测返回 `installed=true`，设备列表出现 `emulator-5554`。该路径配置保留在测试机，供后续使用。
4. 打开设备卡片，WebSocket H.264 服务端会话就绪（`920x2048`）。在计算器真实点击 `2 + 2 =`，设备画面显示 `4`；截图见 `macmini-touch-control.png`。
5. 为运行包内仅供验证的 WebRTC PoC，临时以 `FARM_WEBRTC_POC=1` 启动 beta App。浏览器经 SSH 隧道连接 `emulator-5554`，协商 H.264 WebRTC 并显示实时画面；截图状态为 `460x1024`、约 `1fps`、`116kbps`、抖动缓冲 `4ms`、丢包 `0`。服务日志记录使用设备 SPS、ICE `connected`、发送 84 帧且丢弃 0 帧。
6. 在 PoC 页面断开后，按正常环境重新启动 Farm（不带 PoC 开关）。最终 `/api/desktop/health` 为 `200`，`/api/webrtc/poc` 返回 `404`，确认调试页默认关闭。

## 验收

**Mac mini beta 桌面包、WebSocket 投屏和 WebRTC 端到端链路通过。** 安装摘要、App 启动、桌面服务健康、设备发现、触控、H.264 帧传输、WebRTC 协商和正常关闭均有记录。服务端详细日志见 [`logs.txt`](./logs.txt)，界面证据见本目录 PNG 文件。

本次没有完成完整 beta 发布验收：`v2026.11.25-feature/tasks.md` 记录 beta 服务实例 8081 未提供，故通道隔离、beta/正式通道切换、应用内更新等未覆盖；Windows 原生验证也不属于本次 Mac mini 设备范围。没有据此勾选版本任务完成。

## 操作限制与环境调整

- 通过外部浏览器代理打开依赖设置时，设置保存请求因缺少 Tauri 注入的控制令牌返回 `unauthorized`；随后使用 Mac mini 本机就绪文件中的令牌，在远端本机执行相同的受认证配置接口调用。令牌未写入测试记录。
- SSH 会话运行 `screencapture` 被 macOS 拒绝（`could not create image from display`）。截图采用 Ego Browser 对测试界面的实际渲染；原生窗口和菜单通过 AppleScript 查询与点击确认。
- WebRTC PoC 开关仅在测试启动期间设置；最终 App 以默认环境重启，PoC 路由已确认关闭。
