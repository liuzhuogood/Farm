# Windows 11 测试记录（2026-11-27）

## 范围

对照正式标签 `v2026.11.24`，本次验收 Release `v2026.11.25-beta.1` 的 Windows x86_64 桌面包在 Windows 11 上的安装与运行。

- 代码：`main`，`520f0ad`；工作区干净，`git pull` 后与远端一致。
- 本包内变动功能：`cb904a8` 新增设备 H.264 直通 WebRTC 视频通道、`b820a41` 按设备真实 SPS 协商并完善关键帧闸门；WS 投屏通道保持不变。
- `a37fa2a`（设备自动恢复与投屏诊断，11-26）在 `v2026.11.25-beta.1` 之后，**不在本包内**，本次未覆盖。
- 测试机：`WIN11TEST`（10.0.0.119），Windows 11 企业版 LTSC 2024 build 26100，4 vCPU / 4 GB，VMware 桥接、无 GPU 加速；浏览器 Edge 158.0.4370.33。
- Android 设备：Redmi M2004J7AC（atom），Android 12 / MIUI 13.0.8，WiFi ADB 连接（serial `10.0.0.30:5555`），视频流 `448x1024` H.264。

## 资产与替换安装

- 包：`farm-windows-gui-amd64.exe`，44,710,400 字节，SHA-256 `779c4065fa62a71e14eec83301112bbfe11b2f0d3a41f696564c1ed7606706f6`，与 Release 资产 digest 一致。
- 替换目标：`C:\Users\test\Desktop\Farm.exe`；原 `v2026.11.24-beta.1`（SHA-256 `98c9bec2…`）备份为 `Farm-2026.11.24-beta.1.exe.bak`。
- 安装后文件即上述 SHA-256，版本号 `2026.11.25-beta.1`。

## 执行步骤与结果

| # | 步骤 | 结果 |
| --- | --- | --- |
| 1 | `git pull`（main）+ 读取 11-26 至今天的提交，确认包内变动功能 | 通过 |
| 2 | 下载 `v2026.11.25-beta.1` Windows GUI 资产并核对 SHA-256 与 Release digest | 通过 |
| 3 | 停进程、备份旧 `Farm.exe`、替换安装、校验安装后摘要 | 通过 |
| 4 | 交互会话启动桌面端，窗口 `Farm` 正常创建 | 通过 |
| 5 | `GET /api/version/health` → `v2026.11.25-beta.1`；`/api/version/status` → `idle` | 通过 |
| 6 | `GET /api/devices` 返回在线设备（WiFi，`auto_reconnect: true`），设备列表页显示 Redmi 卡片 | 通过 |
| 7 | `GET /api/screen-state?lock=1`：`screenOn=false / screenLocked=true`；`KEYCODE_WAKEUP` 后亮屏，`wm dismiss-keyguard` 后 `screenLocked=false` | 通过 |
| 8 | WS H.264 投屏：设备页与小窗均显示实时画面（Settings 页面完整可见） | 通过 |
| 9 | 鼠标点击镜像画面：点设备页设备卡片打开小窗（符合"点击设备默认打开小窗"），在小窗画面点设备坐标 `@540,1250` 打开 WLAN 页，`observe` 与截图确认页面切换 | 通过 |
| 10 | 自动化接口：`/api/automation/observe`（text 与 UI 树）、`/api/automation/act`（含中文 `check 设置` 命中 `tree:uiautomator`）、`/api/adb/commands` | 通过 |
| 11 | 桌面页面：设备列表、任务记录（显示本次自动化任务记录）、API 页均正常渲染 | 通过 |
| 12 | WebRTC：默认启动 `/api/webrtc/poc` 为 404；`FARM_WEBRTC_POC=1` 时调试页 200。`POST /api/webrtc/offer` 返回 `codec=h264`、`448x1024`、`profile-level-id=64000a`、`sprop-parameter-sets`，`DELETE /api/webrtc/session` 返回 204 | 部分通过 |
| 13 | 浏览器（Edge 158）调试页点「连接」→ `失败: 400 Bad Request`；服务日志 `m=video 0 …` + `unable to start track, codec is not supported by remote` | **不通过**，见 [issue #5](https://github.com/liuzhuogood/Farm/issues/5) |
| 14 | 关闭 PoC 开关重启，健康检查 200、`/api/webrtc/poc` 回到 404，进程正常 | 通过 |

## 结论

Windows 11 桌面包 `v2026.11.25-beta.1` 的安装、启动、ADB 设备发现、WS 投屏、鼠标输入与自动化接口全部通过；WS 通道在本次 WebRTC 失败期间持续正常，未被影响。

新功能 WebRTC 视频通道在浏览器侧协商失败（浏览器把 answer 的视频 m-line 置 0），已登记 [#5](https://github.com/liuzhuogood/Farm/issues/5)，未据此勾选 `v2026.11.25-feature/tasks.md` 的 beta 验收项。

## 未覆盖 / 未判定

- `a37fa2a` 设备自动恢复与投屏诊断（在 beta.1 之后，包内没有）。
- 应用内更新：虚机内更新地址不可达（`Updater 检查更新失败`），本次不判定为缺陷。
- beta 服务实例相关的通道隔离、通道切换（8081 实例仍不存在，与 macmini 记录一致）。
- 长时间投屏稳定性、多设备群控未执行。

## 环境调整与还原

- 设备侧：为测试稳定把 `screen_off_timeout` 从 `60000` 临时改为 `600000`、`svc power stayon true`；结束时已还原为 `60000` 与 `stayon false`。
- 测试机侧：RDP 会话断开导致截屏拿不到桌面，用 `tscon 1 /dest:console` 把 `test` 会话切到控制台后完成截图；新增的辅助脚本与日志位于测试机 `C:\Users\test\t_*.ps1`。
- 桌面端最终以常规方式（不带 `FARM_WEBRTC_POC`）运行 `v2026.11.25-beta.1`，设备仍在线。

## 文件

- 步骤：`windows11_steps.md`（当前文件）
- 日志：`windows11_logs.txt`
- 报告：`windows11_report.html`
- 截图：`windows11_01-before-install.png` ～ `windows11_09-restored.png`（第 12/13 步的调试页截图已对设备 serial 打码）
