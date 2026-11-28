# Mac mini 测试记录（2026-11-28）

## 范围

对照 `main` `a6bfaa4`（11-28 00:27），11-27 09:56 上次 Mac mini 测试至今只有 `1f6aab8` 改动代码（`internal/devicertc/bridge.go`、`bridge_test.go`），其余提交均为测试记录与文档。

- 变动功能：SDP 的 `profile-level-id` 归一化——设备 SPS 为浏览器不接受的 profile（High 等）时改声明 `42e01f`，`sprop-parameter-sets` 仍取设备真实参数集。目标问题：[#5](https://github.com/liuzhuogood/Farm/issues/5)。
- 最新 Release 仍是 `v2026.11.25-beta.1`（2026-11-24），**不含该修复**；本次沿用 Windows 11 测试的两条路径：
  1. 打包路径：替换安装 Release 包，复核安装、启动、设备发现、WS 投屏、UI 点击与自动化触控（回归）。
  2. 修复路径：用 `main` 源码本机构建 Go 服务（与桌面包内嵌服务同一条代码路径与接口），独立端口 `18080` 带 `FARM_WEBRTC_POC=1` 跑，用 Chrome PoC 页验证浏览器协议协商、SDP 取值与出画。
- 测试机：本机即 Mac mini `10.0.0.11`（macOS 26.7.1，arm64），本地图形会话（非 SSH）。Chrome `158.0.8127.88`。
- 设备：`emulator-5554`（Android 14 / sdk 34，1080x2400，H.264 `460x1024`）；Redmi `10.0.0.30`（`atom`）本次掉线，未参与。

## 资产

| 用途 | 文件 | 大小 | SHA-256 |
| --- | --- | --- | --- |
| 打包路径 | `farm-v2026.11.25-beta.1-macos-arm64-gui.dmg`（Release `v2026.11.25-beta.1`） | 21,132,033 | `a6a6871f2d9a29946afc88bf153fded37ad251cd32a8eb0272b9ed1e429c371a`，与 Release 及 11-27 记录一致 |
| 修复路径 | 本机 `go build -trimpath -ldflags="-s -w"`（go1.26.2 darwin/arm64） | 27,252,306 | `f2ec04942314d5050c7fa4220426c50f5a9b47ab4d16fe33ee69fb8c4305ae73` |

- 替换目标 `/Applications/Farm.app`（beta.1，Bundle 版本 `2026.11.25-beta.1`）。
- 修复版部署 `/tmp/farm-fixed/farm`，`--no-gui --listen 127.0.0.1:18080 --data-dir /tmp/farm-fixed/data`，`FARM_WEBRTC_POC=1`、`FARM_ADB=/Users/liuzhuo/Library/Android/sdk/platform-tools/adb`。
- 工作区另有未提交改动 `desktop/src/links.rs`、`frontend/src/components/player/FloatingPlayer.vue`，不在任何已发布包内，本次未改动、未覆盖。

## 执行步骤与结果

| # | 步骤 | 结果 |
| --- | --- | --- |
| 1 | `git fetch` 核对：本地 `main` 与 `origin/main` 一致（`a6bfaa4`），11-27 09:56 之后仅 `1f6aab8` 改代码 | 通过 |
| 2 | 核对 Releases：最新为 `v2026.11.25-beta.1`，包内不含 `1f6aab8` | 通过（打包路径尚未含修复） |
| 3 | 下载 DMG 核对摘要（与 Release 及 11-27 记录一致），退出旧版后替换 `/Applications/Farm.app` | 通过 |
| 4 | 启动桌面端：`/api/version/health` → `v2026.11.25-beta.1`；`/api/desktop/config` → `{"port":1989,"effective":1989,"is_overridden":false}` | 通过 |
| 5 | `/api/webrtc/poc` → `404`，PoC 调试页默认关闭 | 通过 |
| 6 | `/api/devices`：`emulator-5554` 在线（Calculator 前台，未锁屏）；Redmi `10.0.0.30:5555` `offline` | 通过（Redmi 见观察项） |
| 7 | WS 投屏 `/ws?serial=emulator-5554`：握手成功，元数据 `codec=h264 460x1024`、`audio_codec=aac`，6 秒内 200 个二进制帧 + `stream.heartbeat` | 通过 |
| 8 | `/api/devices/emulator-5554/latest-frame` → `200 image/png` 1080x2400 | 通过 |
| 9 | `POST /api/automation/act`：`tap @675,2198 / @135,1366 / @945,1366 / @405,1366 / @945,2198` + `check 56`，7/7 行完成，`check` 命中 `56`（`tree:uiautomator`） | 通过 |
| 10 | Chrome 打开本机服务界面：设备列表、卡片遥测渲染正常；点开设备进入镜像面板 | 通过 |
| 11 | 在镜像画面用 CDP 派发鼠标事件：`C,C,C,C` + `7 × 8 =`，设备端结果为 `7×8 = 56`，`observe` 读到 `text "56"` | 通过（UI 点击下传有效） |
| 12 | 启动修复版服务（18080）：PoC 路由 `200`，`/api/devices` 见 `emulator-5554` | 通过 |
| 13 | `POST /api/webrtc/offer`：设备 SPS `profile=42c029` → SDP 声明 `profile-level-id=42c01f`，`sprop-parameter-sets` 仍为设备真实参数集 | 通过（归一化生效） |
| 14 | Chrome PoC 页点「连接」：`视频已到达`、`460x1024`、`readyState=4`；服务端 `协商完成 视频编码=m=video 9 UDP/TLS/RTP/SAVPF 102 a=rtpmap:102 H264/90000`、`state=connected`、`首帧 pts=…us` | 通过 |
| 15 | 画面统计：活动时 4→16 fps、238~502 kbps、抖动缓冲 12~227 ms、丢包 0；静态画面 0 fps / 29 kbps | 通过（静态现象见观察项） |
| 16 | `go vet ./internal/devicertc/ ./internal/ws/` 与 `go test ./internal/devicertc/ -run 'Profile\|SDP\|Fmtp\|Format' -v`：`TestSDPProfileLevelID`、`TestFmtpLineKeepsDeviceParameterSets`、`TestFmtpLineWithoutParameterSets`、`TestOfferSDPUsesBrowserCompatibleProfile` 全 PASS | 通过 |
| 17 | 收尾：停掉 18080 修复版服务与两个 Chrome 调试实例，保留已安装的 beta.1 桌面端在 `1989` 运行 | 通过 |

## 结论

- 打包路径（`v2026.11.25-beta.1`）在 Mac mini arm64 上替换安装、启动、设备发现、WS H.264 投屏、UI 镜像与点击下传、自动化 `check 56` 全部通过。
- 修复路径在 Mac mini + Chrome 158 上通过：offer 的 `profile-level-id` 已归一化（设备 `42c029` → 声明 `42c01f`），浏览器接受 answer 的视频 m-line，`state=connected`、首帧到达、画面活动时 16 fps 且丢包 0。
- 打包路径仍未含修复：`v2026.11.25-beta.1` 资产内嵌的服务早于 `1f6aab8`，需要发 `v2026.11.25-beta.2` 后再跑一次安装包路径才算 beta 验收闭环。

## 观察项与未完成

- **High profile 场景未能在 Mac mini 复核**：目标设备 Redmi（`atom`，上次实测 SPS `64000a`）本次 `adb connect` 返回 `No route to host`（设备可 ping 通、adbd 未监听），全程未恢复，只能用模拟器（`42c029`，constrained baseline）验证。`profile-level-id` 归一化的 High 分支由单测 `TestOfferSDPUsesBrowserCompatibleProfile` 与 11-28 Windows 11 + Edge 实测覆盖，**Mac 端同场景待设备恢复后复测**。
- **静态画面的关键帧请求**：PoC 会话开着而设备画面静止（scrcpy 无新帧）时，浏览器按 RTCP PLI 请求，服务端翻译成 `RESET_VIDEO`（实测 09:05:52 前多次），统计 0 fps；画面一变化即恢复 16 fps。与 11-28 Windows 记录一致，未判定为缺陷。
- **未覆盖**：应用内更新、beta 服务实例 8081 的通道隔离与切换（`docs/versions/v2026.11.25-feature/tasks.md` 记录该实例仍未提供）、长时间稳定性、多设备群控；`desktop/src/links.rs`、`FloatingPlayer.vue` 的未提交改动不在包内，未验证。
- 本次为同日第二次 Mac mini 执行，未重复 11-27 的投屏异常自恢复（`StreamKeeper`）验证。

## 验证脚本与限制

- CDP 点击需先 `Input.dispatchMouseEvent(type=mouseMoved)` 再按下/抬起，否则合成点击可能按上一次的旧坐标下发到设备（实测表现为点击落点错行）。已回写避坑指南。
- 桌面端截图在本机图形会话可直接 `screencapture`；显示器休眠时会得到全黑图，先 `caffeinate -u -t 5` 再拍。

## 文件

- 步骤：`macmini_steps.md`（当前文件）
- 日志：`macmini_logs.txt`
- 报告：`macmini_report.html`
- 截图：`macmini_01-ui-home.png`（设备列表）、`macmini_02-ui-mirror-input.png`（镜像面板 7×8=56）、`macmini_03-webrtc-poc.png`（WebRTC 出画与统计）、`macmini_04-native-app.png`（Farm 原生窗口）、`macmini_05-device-7x8-56.png`（设备端结果）
