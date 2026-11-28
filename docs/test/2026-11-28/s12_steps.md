# S12 测试记录（2026-11-28）

## 范围

验证最新发布包在 S12（Mac Intel / macOS 11.7.6）上的替换安装与启动，并复测 11-27 登记的 [#4](https://github.com/liuzhuogood/Farm/issues/4)。

自 2026-11-27 起的变动只有一个功能提交：`1f6aab8`（SDP `profile-level-id` 归一化，修复 [#5](https://github.com/liuzhuogood/Farm/issues/5)），其余为测试与文档提交（`c6b6561`、`a6bfaa4`、`dc663cb`）。该修复目前只在 `main`，未进任何发布包；S12 侧因桌面包无法启动而完全无法覆盖。

- 代码：`main`，`dc663cb`（与 `origin/main` 一致，未拉取到新提交）
- 包：`v2026.11.25-beta.1`（该 Release 仍是 `releases/latest` 之前的最后一个包，11-24 之后没有新构建）
- 资产：`farm-v2026.11.25-beta.1-macos-amd64-gui.dmg`（22,681,040 字节）
- SHA-256：`532ca1e50c9f5b8c282c9a62a6e9f50893bf375d93435f7081341b7bf74823ea`，与 Release 资产 digest 一致
- 设备：S12，MacBook Pro 2013 / Intel Core i7-4850HQ / macOS 11.7.6 (20G1231) / x86_64

## 执行步骤与结果

1. `git fetch`：`main` 与 `origin/main` 一致（`dc663cb`），无可拉取提交；工作区既有改动（`desktop/src/links.rs`、`frontend/src/components/player/FloatingPlayer.vue`）与本次测试无关，未触碰。
2. 在 S12 上从 Release 下载 macOS amd64 DMG，`shasum -a 256` 与 Release digest 一致。
3. 卸载旧安装后替换安装：挂载 DMG，删除旧 `/Applications/Farm.app`，复制 `Farm.app`，剥离 `com.apple.quarantine`。Bundle 版本 `2026.11.25-beta.1`，`LSMinimumSystemVersion` 声明 `10.13`；签名 adhoc，`spctl` 判定 `rejected`（与既往一致）。
4. `caffeinate -u -t 3` 唤醒显示器后 `open -a /Applications/Farm.app`，7 秒后截屏：弹出“Farm 意外退出”，未出现主窗口；`pgrep` 无残留进程。
5. 同一次启动产生两份新崩溃报告：`farm` 为 `DYLD, [0x4] Symbol missing`，`farm-gui` 为 `EXC_CRASH (SIGABRT)`，崩溃线程栈落在 `tao::platform_impl::platform::app_delegate::did_finish_launching` 的 Rust panic。
6. 命令行运行包内服务 `Contents/MacOS/farm --no-gui`：dyld 缺 `_SecTrustCopyCertificateChain`（该符号自 macOS 12 起提供），`exit=134`。
7. 健康检查 `http://127.0.0.1:1989/api/desktop/health` 返回 `http=000`。
8. 补充核对同一 Release 的 CLI 资产 `farm-macos-amd64-cli`：同样是 `LC_BUILD_VERSION minos 12.0`，运行即 dyld 缺符号 `exit=134`，说明 Intel Mac 两个通道都不可用，不只是 GUI 包。

## 验收

**不通过（与 11-27 结论一致，仍复现）。** 安装与摘要校验通过，但桌面包在 macOS 11.7.6 上启动即被 dyld 终止；设备发现、投屏（含本次 `1f6aab8` 的 WebRTC profile 修复）、自动化触控、依赖管理、应用内更新等全部无法验证。

未覆盖的功能面：

- `1f6aab8` 的 SDP `profile-level-id` 归一化（未进发布包，且 S12 无法启动服务）。
- `a37fa2a` 的设备自动恢复、投屏诊断、熄屏策略与播放器前端。
- 设备发现、WS/WebRTC 投屏、自动化接口、依赖管理、应用内更新。

## 结论与建议

- 根因未变：DMG 与 `Info.plist` 声明 `10.13`，随包 Go 侧二进制要求 `12.0`（Go 1.25 起最低 macOS 12）。
- 两个可选方向（如 #4 所述）：按 macOS 11 部署目标构建随包 Go 服务；或确定只支持 macOS 12+ 时把 `LSMinimumSystemVersion` 改为 `12.0` 并在发版说明写明支持范围，避免用户只看到“意外退出”。
- 在 issue 关闭前，S12 每日复测无法产生新信息，只能重复确认同一崩溃；复测应在修复后的新包（`beta.2` 之后）上做。

## 文件证据

- 步骤：`s12_steps.md`（当前文件）
- 日志：`s12-logs.txt`（环境、摘要、安装结果、启动输出、崩溃摘要、CLI 对照，已脱敏）
- 截图：`s12-launch-crash.png`（“Farm 意外退出”对话框，桌面无账号/主机名信息）
- 报告：`s12_report.html`
