# Windows 11 测试记录（2026-11-28）

## 范围

对照 `main` `1f6aab8`（11-27 23:19），11-27 至今只有该提交改动代码（`internal/devicertc/bridge.go`、`bridge_test.go`），其余提交均为文档。

- 变动功能：SDP 的 `profile-level-id` 归一化——设备 SPS 为 High（本机实测 `64000a`）时浏览器拒收，answer 把视频 m-line 置 0；归一化后声明 `42e01f`，`sprop-parameter-sets` 仍取设备真实参数集。目标问题：[#5](https://github.com/liuzhuogood/Farm/issues/5)。
- 最新 Release 仍是 `v2026.11.25-beta.1`（2026-11-24），**不含该修复**；本次分两条路径：
  1. 打包路径：装 Release 包 `v2026.11.25-beta.1`，复核安装、启动、ADB、WS 投屏、自动化接口（回归）。
  2. 修复路径：用 `main` 源码本地交叉编译 Windows 服务二进制（与桌面端内嵌的服务同一条代码路径与 HTTP 接口），在测试机独立端口跑，用 Edge 走 PoC 页验证浏览器协商与解码。
- 测试机：`WIN11TEST`（10.0.0.119），Windows 11 企业版 LTSC 2024 build 26100，4 GB / 2 vCPU，VMware 桥接无 GPU 加速；Edge `158.0.4370.33`。
- Android 设备：Redmi M2004J7AC（atom），Android 12 / MIUI 13.0.8，WiFi ADB（serial `10.0.0.30:5555`），视频流 `448x1024` H.264。

## 资产

| 用途 | 文件 | 大小 | SHA-256 |
| --- | --- | --- | --- |
| 打包路径 | `farm-windows-gui-amd64.exe`（Release `v2026.11.25-beta.1`） | 44,710,400 | `779c4065…6706f6`，与 Release digest 一致 |
| 修复路径 | 本地 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` | 28,719,104 | `8a2c13c9df58f424e6411b33896be9d2a3f19447bc1795e8bbf2e318420a82d1` |

- 替换目标 `C:\Users\test\Desktop\Farm.exe`，旧版同号包备份为 `Farm-2026.11.25-beta.1.exe.bak`。
- 修复版部署为 `C:\Users\test\farm-fixed\farm-fixed.exe`，以 `--no-gui --listen 0.0.0.0:18080 --data-dir C:\Users\test\farm-fixed\data` 运行，`FARM_WEBRTC_POC=1`、`FARM_ADB` 指向 App 已下载的 `dependencies\adb\adb.exe`。
- 工作区另有未提交改动 `desktop/src/links.rs`、`frontend/src/components/player/FloatingPlayer.vue`，不参与 Go 构建，本次未动。

## 执行步骤与结果

| # | 步骤 | 结果 |
| --- | --- | --- |
| 1 | `git fetch` 核对 `main`；确认 11-27 至今只有 `1f6aab8` 改动代码 | 通过 |
| 2 | 核对 Releases：最新为 `v2026.11.25-beta.1`，包内不含 `1f6aab8` | 通过（结论：打包路径尚未含修复） |
| 3 | 本地 `go vet` / `go test ./internal/devicertc/` | 通过 |
| 4 | 替换安装 beta.1 包，核对替换前后摘要 | 通过 |
| 5 | 交互会话启动桌面端：`/api/version/health` → `v2026.11.25-beta.1`、`/api/version/status` → `idle`、PoC 路由 404 | 通过 |
| 6 | Edge 打开 `127.0.0.1:1989`：设备列表渲染、点开小窗后 WS H.264 实时出画（设置页） | 通过 |
| 7 | 亮屏、解锁、`am start` 设置页、`/api/automation/observe` 返回界面树 | 通过 |
| 8 | 启动修复版服务（18080）：PoC 路由 200、`/api/devices` 在线、`adb connect` 已连接 | 通过 |
| 9 | `POST /api/webrtc/offer`：设备 SPS `profile=64000a` → SDP 声明 `profile-level-id=42e01f`，`sprop-parameter-sets` 仍为设备真实参数集 | 通过（修复生效） |
| 10 | Edge PoC 页点「连接」：`视频已到达`，answer `m=video 9 UDP/TLS/RTP/SAVPF 102 H264/90000`，`state=connected`，首帧到达 | 通过（beta.1 同机同浏览器时是 m-line 0 + 400） |
| 11 | 画面活动（连续滑动）时统计 13~16 fps、约 2 Mbps、抖动缓冲 10~12 ms、丢包 0 | 通过 |
| 12 | 静态画面时统计 0 fps / 0 kbps，服务端每约 4 s 收到 PLI 并 `RESET_VIDEO` | 观察项（见下） |
| 13 | 关闭 PoC、恢复常规启动：健康检查 200、PoC 回到 404、设备仍在列表 | 通过 |
| 14 | 设备设置还原并复核 `screen_off_timeout` / `stayon` | **未复核**：设备在还原阶段掉线，见下 |

## 结论

- [#5](https://github.com/liuzhuogood/Farm/issues/5) 的失败场景在 Windows 11 + Edge 158 上复测通过：同一台设备（High profile `64000a`）、同一浏览器，SDP 归一化后协商成功、服务端 `state=connected`、浏览器 `framesDecoded` 正常增长并出画，画面活动时 13~16 fps、丢包 0。Windows 侧的 H.264 解码路径本次已覆盖（此前未覆盖）。
- 打包路径未被覆盖：`v2026.11.25-beta.1` 资产内嵌的服务早于修复，需要发 `v2026.11.25-beta.2` 后再跑一次安装包路径才算 beta 验收闭环。

## 观察项与未完成

- **静态画面下的关键帧风暴**：PoC 会话开着且设备画面静止时（scrcpy 画面不变则不产生新帧），浏览器约每 4 s 发一次 PLI，服务端翻译成 `RESET_VIDEO`；期间统计为 0 fps / 0 kbps，一旦有画面变化立刻恢复（实测 6~16 fps）。本次未判定为缺陷：出画不受影响，是否需要“空闲时忽略 PLI”待确认。
- **设备在还原阶段掉线**：还原脚本中 `settings get` 报 `Failure calling service settings: Broken pipe (32)`，随后 adb 显示设备 `offline`、`10.0.0.30:5555` 拒绝连接（设备仍能 ping 通、未再广播 `_adb._tcp`），符合设备侧 `system_server` 重启/整机重启后无线 ADB 未恢复的特征。因此 `screen_off_timeout`（测试中临时改为 600000）与 `svc power stayon` 的还原结果**未能复核**，需设备重新连回后确认；可能诱因是静态画面期 42 次 `RESET_VIDEO` 或设备温度（复原前 cpu_temp 53℃），尚未定位。
- 未执行：应用内更新（虚机内更新地址不可达，与 11-27 一致）、beta 服务实例 8081 的通道隔离与切换、长时间稳定性、多设备群控。

## 测试机侧产物

- `C:\Users\test\t2_install.ps1`、`t2_launch.ps1`、`t2_api.ps1`、`t2_fixed_server.ps1`、`t2_offer.ps1`、`t2_activity.ps1`、`t2_restore.ps1`、`t2_check.ps1`；输出 `t2_*.txt`。
- `C:\Users\test\farm-fixed\{farm-fixed.exe,server.err.log,server.out.log,data\}`。
- 浏览器验证改用 Edge `--remote-debugging-port=9222` + 本机 SSH 隧道 + CDP 驱动（点击、取值、截图），不再用像素坐标点小窗。

## 文件

- 步骤：`windows11_steps.md`（当前文件）
- 日志：`windows11_logs.txt`
- 报告：`windows11_report.html`
- 截图：`windows11_01-gui-boot.png`（桌面端启动）、`windows11_02-ui-home.png`（设备列表）、`windows11_03-ui-ws-mirror.png`（WS 小窗投屏）、`windows11_04-webrtc-idle.png`（WebRTC 已出画，静态画面）、`windows11_05-webrtc-active.png`（WebRTC 画面活动 6 fps）、`windows11_06-restored.png`（恢复常规启动）。截图内设备 serial 已打码。
