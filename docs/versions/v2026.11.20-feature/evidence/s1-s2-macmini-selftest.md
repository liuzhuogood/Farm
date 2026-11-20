# S1/S2 macmini 自测记录

日期：2026-11-20
环境：macmini `10.0.0.11` · darwin/arm64 · macOS 26.7.1 · Go 1.26.2 · pnpm
被测对象：已推送的 main（`d8a85bc`），在 macmini 的 `~/code/Farm` 真实仓库里 `git pull --ff-only` 后本机构建，不再用临时拷贝。

## 流程

```bash
# macmini
cd ~/code/Farm && git pull --ff-only
pnpm --dir frontend build          # web/dist 不入库，需本地重建
go build -o /tmp/farm-channel-selftest .
go test ./internal/updater/ -count=1
/tmp/farm-channel-selftest --no-gui --listen 0.0.0.0:18089 --data-dir /tmp/farm-channel-data
```

## 结果

| # | 用例 | 期望 | 实际 | 结论 |
| --- | --- | --- | --- | --- |
| 1 | `GET /api/version/health` | 返回版本 | `v2026.11.17-1` | 通过 |
| 2 | `GET /api/version/channel`（首次） | `stable` | `stable` | 通过 |
| 3 | `PUT /api/version/channel` → beta | 回显 beta | `beta`，回读一致 | 通过 |
| 4 | `PUT` 非法通道 `nightly` | 400 | `400` | 通过 |
| 5 | `GET /api/version/check`（beta 通道） | `channel:"beta"`；无 beta 发布时取正式 | `channel:beta`、`latest_version:v2026.11.17-1`、`has_update:false` | 通过 |
| 6 | `GET /api/version/check?channel=stable` | 临时覆盖不落库 | `channel:stable`，随后 GET 仍为 beta | 通过 |
| 7 | `GET /api/version/desktop-endpoint`（beta，无 beta 发布） | 回退正式索引并报告实际通道 | `channel:stable`、`endpoint:.../releases/latest/download/desktop-latest.json` | 通过 |
| 8 | 重启进程后读取通道 | 保持上次保存值 | 重启后仍为 beta | 通过 |
| 9 | 系统设置 → 通用 → 「检查更新」行 | 通道下拉、立即检查图标、开关同排 | 见 `evidence/farm-channel-ui-beta.png` | 通过 |
| 10 | 下拉切换到 beta 测试版 | 后端同步为 beta | 触发器显示「beta 测试版」，`GET /api/version/channel` 为 beta | 通过 |
| 11 | 点击「立即检查更新」图标 | 打开更新弹窗并给出结果 | 弹窗标题「软件更新」，正文「当前已是最新版本」 | 通过（见 `evidence/farm-channel-ui-check.png`） |

服务端日志：

```
[Updater] 更新通道已切换为 beta
[Updater] beta 通道选中发布: v2026.11.17-1
```

## 自测发现并修复的问题

1. **beta 通道在没有 beta 发布时，用正式 tag 拼出下载地址**：修复前返回 `.../releases/download/v2026.11.17-1/desktop-latest.json` 且自称 beta。已改为 beta 只认 `-beta.*` 预发布，无 beta 时回退正式索引并返回实际生效通道；补 `TestDesktopEndpointFallsBackWithoutBeta`。
2. **通道回显与实际生效不一致**：`desktop-endpoint` 原先回显请求通道，已改为返回实际生效值，Rust 侧日志因此可信。

## 代码验证

- macmini：`go test ./internal/updater/ -count=1` 通过（arm64）。
- 本机（同为 darwin/arm64）：`go test ./...` 35 包通过；`pnpm --dir frontend build`（含 `vue-tsc`）通过；`cargo test` 13 项通过，其中 3 项覆盖后端地址解析与非法地址拒绝。

## 未覆盖

- Tauri 桌面 App 未在 macmini 上构建与实跑：`desktop_check_update` 的完整链路（Rust 取地址 → Tauri 检查）需在真实 App 中验证，目前只验了后端契约与 Rust 地址解析。
- beta 自更新（下载、验签、安装、回退）需要 S3 先发布真实 beta 包后才能验。
