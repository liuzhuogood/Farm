# S12 测试记录（2026-11-27）

## 范围

验证最新发布包在 S12（Mac Intel）上的替换安装与启动。自 2026-11-26 起的变更含 `a37fa2a`（设备自动恢复、投屏诊断、熄屏策略与播放器前端）及后续测试/工作流文档提交（`8d047c5`、`f5e3c6b`、`520f0ad`、`37c6525`）；本轮只能验证到安装与启动，`a37fa2a` 的功能面因启动崩溃未覆盖。

- 代码：`main`，`37c6525`
- 包：`v2026.11.25-beta.1`，GitHub Release：<https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.25-beta.1>
- 资产：`farm-v2026.11.25-beta.1-macos-amd64-gui.dmg`（22,681,040 字节）
- SHA-256：`532ca1e50c9f5b8c282c9a62a6e9f50893bf375d93435f7081341b7bf74823ea`，与 Release 资产 digest 一致
- 设备：S12，MacBook Pro 2013 / Intel Core i7-4850HQ / macOS 11.7.6 (20G1231) / x86_64，此前未安装 Farm（该机型最高只能升到 macOS 11）

## 执行步骤与结果

1. 下载 macOS amd64 DMG，`shasum -a 256` 与 Release 摘要一致。
2. 挂载 DMG（校验通过），把 `Farm.app` 复制到 `/Applications`，剥离 `com.apple.quarantine`。Bundle 版本 `CFBundleShortVersionString` 与 `CFBundleVersion` 均为 `2026.11.25-beta.1`；`LSMinimumSystemVersion` 声明为 `10.13`；签名 adhoc，`spctl` 判定 `rejected`，剥离隔离属性后安装成功。
3. 打开 App：弹出“Farm 意外退出”对话框，未出现主窗口。
4. 命令行直接运行包内服务 `Contents/MacOS/farm --no-gui`：

   ```
   dyld: Symbol not found: _SecTrustCopyCertificateChain
     Referenced from: /Applications/Farm.app/Contents/MacOS/farm (which was built for Mac OS X 12.0)
     Expected in: /System/Library/Frameworks/Security.framework/Versions/A/Security
   exit=134
   ```

5. 崩溃报告记录 `Termination Reason: DYLD, [0x4] Symbol missing`；`desktop.log` 为同一条 dyld 错误。
6. 健康检查 `http://127.0.0.1:1989/api/desktop/health` 返回 `http=000`（服务未监听）。
7. 核对最低系统版本：外壳 `farm-gui` 为 `LC_VERSION_MIN_MACOSX 10.13`，随包的 Go 服务 `farm` 为 `LC_BUILD_VERSION minos 12.0`，并引用 macOS 12 才提供的 `_SecTrustCopyCertificateChain`。
8. 历史版本回查（挂载 DMG 读 Mach-O 头）：`v2026.11.24`、`v2026.11.17-1` 的 amd64 包同样是 `farm` minos `12.0`、`Info.plist` 声明 `10.13`，说明该声明不一致不是 beta.1 新引入，S12 一直无法运行这些版本。

## 验收

**不通过。** 安装与摘要校验通过，但包内 Go 服务二进制要求 macOS 12.0，S12 最高只能到 macOS 11.7.6，启动即被 dyld 终止，设备发现、投屏、自动化等全部功能无法验证。

根因是包内最低系统版本声明不一致：DMG 与 `Info.plist` 声明 `10.13`，Go 侧 sidecar 实际要求 `12.0`（Go 1.25 起最低 macOS 12）。已按 `docs/workflows/issues.md` 登记 [#4](https://github.com/liuzhuogood/Farm/issues/4)。

未覆盖：`a37fa2a` 涉及的设备自动恢复、投屏诊断、熄屏策略与播放器前端，以及设备发现、投屏、自动化触控、依赖管理、应用内更新等 S12 侧功能。

## 操作限制与环境调整

- SSH 会话里 `screencapture` 在 macOS 11 上可直接出图；`sudo launchctl asuser <uid> sudo -u <user> /usr/sbin/screencapture` 也可用，仅显示器休眠时只会拍到壁纸，先 `caffeinate -u -t 5` 唤醒再拍。已回写避坑指南。
- 用 AppleScript 驱动 `Terminal` 会被 TCC 自动化授权拦住并挂起（`osascript` 不返回、退出码 142），截图脚本改用 `open -a Terminal <script>` 触发；卡住的授权弹窗用 `killall tccd` 收掉。
- 落库截图已裁掉用户名与主机名。

## 文件证据

- 步骤：`s12_steps.md`（当前文件）
- 日志：`s12-logs.txt`（环境、摘要、安装结果、启动输出、崩溃摘要，已脱敏）
- 截图：`s12-launch-crash.png`（“Farm 意外退出”对话框 + 命令行 dyld 错误）
- 报告：`s12_report.html`
