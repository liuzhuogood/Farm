# S1/S2 macmini 自测记录

日期：2026-11-20
环境：macmini `10.0.0.11` · darwin/arm64 · macOS 26.7.1 · Go 1.26.2
被测对象：本次改动的源码树（rsync 到 `~/farm-selftest/src` 后本机构建），隔离实例 `--no-gui --listen 0.0.0.0:18089 --data-dir ~/farm-selftest/data-run`

## 结果

| # | 用例 | 期望 | 实际 | 结论 |
| --- | --- | --- | --- | --- |
| 1 | `GET /api/version/health` | 返回版本 | `{"version":"v2026.11.17-1"}` | 通过 |
| 2 | `GET /api/version/channel`（首次） | `stable` | `{"channel":"stable"}` | 通过 |
| 3 | `PUT /api/version/channel` → beta | 回显 beta | `{"channel":"beta"}`，回读一致 | 通过 |
| 4 | `PUT` 非法通道 `nightly` | 400 | `400` | 通过 |
| 5 | `GET /api/version/check`（通道 beta） | 返回 `channel:"beta"`，无 beta 发布时取正式 | `channel:"beta"`、`latest_version:"v2026.11.17-1"`、`has_update:false` | 通过 |
| 6 | `GET /api/version/check?channel=stable` | 临时覆盖不落库 | `channel:"stable"`；随后 GET channel 仍为 beta | 通过 |
| 7 | `GET /api/version/desktop-endpoint`（beta，无 beta 发布） | 回退正式索引并报告实际通道 | `channel:"stable"`、`endpoint:.../releases/latest/download/desktop-latest.json` | 通过（修复后，见下） |
| 8 | `GET /api/version/desktop-endpoint?channel=stable` | 正式索引 | 同上 | 通过 |
| 9 | 重启进程后读取通道 | 保持上次保存值 | beta → 重启仍 beta；改 stable → 重启仍 stable | 通过 |
| 10 | 浏览器打开 `http://10.0.0.11:18089` → 系统设置 → 通用 | 出现「更新通道：正式版 / beta 测试版」 | 见 `evidence/farm-channel-beta.png` | 通过 |
| 11 | 点击「beta 测试版」/「正式版」 | 选中态与后端一致 | `aria-pressed` 正式版 true→false→true；后端 `stable→beta→stable` | 通过 |

服务端日志对应链路：

```
[Updater] 更新通道已切换为 beta
[Updater] beta 通道选中发布: v2026.11.17-1
[Updater] 更新通道已切换为 stable
```

## 自测发现并修复的问题

1. **beta 通道在没有 beta 发布时，用正式 tag 拼出了下载地址**：修复前 `desktop-endpoint` 返回 `.../releases/download/v2026.11.17-1/desktop-latest.json` 且自称 beta。原因是解析复用了 `pickRelease`（含正式发布）。已改为 beta 只认 `-beta.*` 预发布，无 beta 时回退正式索引并返回实际生效通道；补 `TestDesktopEndpointFallsBackWithoutBeta`。
2. **通道状态与生效通道可能不一致**：`desktop-endpoint` 原先回显请求通道而非实际通道，已改为返回生效值，Rust 侧日志因此可信。

## 代码验证

- macmini：`go test ./internal/updater/ -count=1` 通过（arm64）。
- 本机（同为 darwin/arm64）：`go test ./...` 35 包通过；`pnpm --dir frontend build`（含 `vue-tsc`）通过；`cargo test` 13 项通过，其中新增 3 项覆盖后端地址解析与非法地址拒绝。

## 未覆盖

- Tauri 桌面 App 未在 macmini 上构建与实跑：`desktop_check_update` 的完整链路（Rust 取地址 → Tauri 检查）需在真实 App 中验证，目前只验证了后端契约与 Rust 地址解析。
- beta 自更新（下载、验签、安装、回退）需要 S3 先发布真实 beta 包后才能验。
