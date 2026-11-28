# v2026.11.25 任务清单

## 开发

- [x] 新增设备投屏 WebRTC 视频通道（H.264 直通、ICE-Lite、服务端 offer）。
- [x] SDP 的 `sprop-parameter-sets` 使用设备真实 SPS。
- [x] SDP 的 `profile-level-id` 归一化到浏览器可接受集（baseline/main 保留，High 等声明 `42e01f`），修复 [#5](https://github.com/liuzhuogood/Farm/issues/5)。
- [x] PLI/FIR 翻译为 scrcpy `RESET_VIDEO`，以“等新关键帧”为闸门并加 2 秒兜底。
- [x] 保留 WebSocket 投屏通道，不改动现有实现。
- [x] 调试页仅在 `FARM_WEBRTC_POC` 设置时挂载，不进正式界面。

## 验证

- [x] `go test ./...`、`go vet`、前端生产构建。
- [x] 真机 RTP 直通测试与关键帧请求测试（`SCRCPY_BENCH_SERIAL` 守卫）。
- [x] profile 映射与 offer SDP 生成的单测（`internal/devicertc/bridge_test.go`）；Chrome 158 协商与解码对照实测。
- [x] 真实 UI 播放器 WS 投屏回归。

## beta 验收

- [ ] beta 版本号与 Release 链接：`v2026.11.25-beta.1` · https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.25-beta.1
  - Actions [45357895974](https://github.com/liuzhuogood/Farm/actions/runs/45357895974) 全部作业成功（docker 按 beta 跳过）。
  - 14 个资产齐全，预发布（`prerelease: true`），`releases/latest` 仍指向 `v2026.11.24`。
- [ ] beta 服务实例地址：**未提供**。`scripts/deploy/deploy-beta.sh`、`docs/environments/beta.md` 与 8081 实例均不存在（v2026.11.20 的 S4 未落地），10.0.0.6 上只有 8080 的 `mywebscrcpy.service`。
- [ ] 按 [test.md](../../workflows/test.md) 完成测试工作流：beta 包相关项（通道隔离、通道切换、应用内更新、8081 实例链路）因缺少 beta 实例阻塞；Windows 原生构建已在 Actions 中通过。
- [ ] Mac Intel 测试机（macOS 11.7.6）实测：包可安装但启动即崩溃，包内 Go 服务 `minos 12.0` 与 `Info.plist 10.13` 声明不一致，见 [#4](https://github.com/liuzhuogood/Farm/issues/4) 与 `docs/test/2026-11-27/s12_steps.md`。
- [ ] Mac mini（macOS 26.7.1 / arm64）实测：`v2026.11.25-beta.1` 桌面包替换安装、启动、设备发现、WS 投屏、UI 镜像点击与自动化 `check 56` 通过；用 `main` 本地构建的服务在 Chrome 158 上验证 profile 归一化（设备 `42c029` → 声明 `42c01f`）与出画（16 fps、丢包 0），见 `docs/test/2026-11-28/macmini_steps.md`。High profile 设备本次掉线（`No route to host`），Mac 端同场景待设备恢复后复测；包内仍含旧服务，待 `v2026.11.25-beta.2`。
- [ ] Windows 11 测试机（build 26100 虚机）实测：`v2026.11.25-beta.1` 桌面包替换安装、启动、ADB 发现、WS 投屏、镜像鼠标输入与自动化接口通过；新增 WebRTC 通道在 Edge 158 上 answer 被拒（视频 m-line 置 0），见 [#5](https://github.com/liuzhuogood/Farm/issues/5) 与 `docs/test/2026-11-27/windows11_steps.md`。已按 profile 归一化修复，待新包复测（Windows 的 Media Foundation 解码路径未覆盖）。
