# v2026.11.25：设备投屏 WebRTC 通道

## 变更

- 新增设备投屏 WebRTC 通道：服务端生成 offer、H.264 直通、ICE-Lite 不开 STUN/TURN，浏览器硬解播放。
- SDP 的 `profile-level-id` 与 `sprop-parameter-sets` 取自设备真实 SPS，避免协商到错的解码配置导致黑屏。
- 观看端 PLI/FIR 翻译为 scrcpy `RESET_VIDEO`（协议新增 Type 17）；闸门以“等新关键帧”为准、2 秒兜底，避免反复重启编码。
- 新增 `POST /api/webrtc/offer`、`POST /api/webrtc/answer`、`DELETE /api/webrtc/session`；调试页仅在 `FARM_WEBRTC_POC` 设置时挂载。
- 原有 WebSocket 投屏通道保持原实现，未改动。

## 验证

- `go test ./...` 全绿；`go vet ./internal/ws ./internal/devicertc` 无告警；前端 `pnpm --dir frontend build` 成功。
- 真机：RTP 直通（pion 观看端，`-race` 无竞争）、RESET_VIDEO 后 0.8~1.9 秒出新 config+关键帧、浏览器出画 448×1024（抖动缓冲 93ms、解码 5ms）。
- 回归：真实 UI 播放器（`/player?serial=...`）WS 投屏正常。

## 发布

- beta 记录见 [tasks.md](./tasks.md)。
