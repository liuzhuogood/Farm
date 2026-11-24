# v2026.11.25 任务清单

## 开发

- [x] 新增设备投屏 WebRTC 视频通道（H.264 直通、ICE-Lite、服务端 offer）。
- [x] SDP 使用设备真实 SPS 生成 `profile-level-id` 与 `sprop-parameter-sets`。
- [x] PLI/FIR 翻译为 scrcpy `RESET_VIDEO`，以“等新关键帧”为闸门并加 2 秒兜底。
- [x] 保留 WebSocket 投屏通道，不改动现有实现。
- [x] 调试页仅在 `FARM_WEBRTC_POC` 设置时挂载，不进正式界面。

## 验证

- [x] `go test ./...`、`go vet`、前端生产构建。
- [x] 真机 RTP 直通测试与关键帧请求测试（`SCRCPY_BENCH_SERIAL` 守卫）。
- [x] 真实 UI 播放器 WS 投屏回归。

## beta 验收

- [ ] beta 版本号与 Release 链接：
- [ ] beta 服务实例地址：
- [ ] 按 [test.md](../../workflows/test.md) 完成测试工作流。
