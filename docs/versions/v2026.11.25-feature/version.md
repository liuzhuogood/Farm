# v2026.11.25：设备投屏 WebRTC 通道

## 变更

- 新增设备投屏 WebRTC 通道：服务端生成 offer、H.264 直通、ICE-Lite 不开 STUN/TURN，浏览器硬解播放。
- SDP 的 `profile-level-id` 按浏览器可接受集归一化：设备是 baseline/main 时保留其 profile，High 等其它 profile 统一声明 `42e01f`，level 统一 3.1；`sprop-parameter-sets` 仍用设备真实参数集，关键帧前也继续前置 SPS/PPS，解码按 in-band 参数进行。
  - 依据（issue #5）：Edge 158 对 High profile 的 offer 直接把视频 m-line 置 0，`answer` 应用失败返回 400；本机 Chrome 158 实测只接受 baseline/main，且声明 `42e01f`/`4d001f` 时能正常解出设备同分辨率的 High profile 码流。
- 观看端 PLI/FIR 翻译为 scrcpy `RESET_VIDEO`（协议新增 Type 17）；闸门以“等新关键帧”为准、2 秒兜底，避免反复重启编码。
- 新增 `POST /api/webrtc/offer`、`POST /api/webrtc/answer`、`DELETE /api/webrtc/session`；调试页仅在 `FARM_WEBRTC_POC` 设置时挂载。
- 原有 WebSocket 投屏通道保持原实现，未改动。

## 验证

- `go test ./...` 全绿；`go vet ./internal/ws ./internal/devicertc` 无告警；前端 `pnpm --dir frontend build` 成功。
- 单测覆盖 profile 映射与 offer SDP 生成（`internal/devicertc/bridge_test.go`）：设备 High → 声明 `42e01f`、`sprop` 保持设备真实参数集。
- 浏览器实测（本机 Chrome 158，无 GPU）：服务端 offer 声明 `42e01f` / `4d001f` 时 answer 接受且解出 448×1024 High profile 码流（149/150 帧、5 个关键帧）；声明 `64001f` / `64000a` 时 answer 视频 m-line 置 0，与 issue #5 一致。
- 真机：RTP 直通（pion 观看端，`-race` 无竞争）、RESET_VIDEO 后 0.8~1.9 秒出新 config+关键帧、浏览器出画 448×1024（抖动缓冲 93ms、解码 5ms）。
- 回归：真实 UI 播放器（`/player?serial=...`）WS 投屏正常。

## 发布

- beta 记录见 [tasks.md](./tasks.md)。
