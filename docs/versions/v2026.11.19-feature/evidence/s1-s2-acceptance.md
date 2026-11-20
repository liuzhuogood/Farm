# v2026.11.19-feature S1/S2 验收记录

日期：2026-11-20 · 环境：10.0.0.6（Linux，adb 共享 `127.0.0.1:5037`）· 被测构建：工作区 S1+S2（基线 v2026.11.17-1）+ 前端生产构建

## 1. 测试方式

- 本地门禁：`go test ./... -count=1` 全绿；`GOOS=linux GOARCH=amd64 go build` 成功；`pnpm --dir frontend build` 成功。
- 真机：把 S2 构建以用户级临时服务跑在 `10.0.0.6:18090`（独立数据目录 `/home/liuzhuo/farm-s2-test`），验收结束后停止并删除；生产 `8080`（v2026.11.17-1，旧代码）全程未改动，只作为对照取样。

真机现场共 6 条 adb 记录，其中 3 台是 USB+无线双连接：

| 设备身份 | USB serial | 无线 serial |
| --- | --- | --- |
| `hw:31040209940025Z` | `31040209940025Z` | `10.0.0.104:5555` |
| `hw:3105357943002BH` | `3105357943002BH` | `10.0.0.244:5555` |
| `hw:9lmn897livkzprnf`（Redmi） | `9lmn897livkzprnf` | `10.0.0.30:5555` |

## 2. 用例结果

| 用例 | 步骤 | 期望 | 结果 | 证据 |
| --- | --- | --- | --- | --- |
| T1 双连接合并 | 打开 `GET /api/devices` | 每台机 1 个条目，`connections` 两条 | 通过：6 条 adb 记录归并为 3 个条目，每台 `connections=[usb,wifi]` | [s2-devices-merged.json](s2-devices-merged.json) |
| T2 默认 USB | 连续多次刷新 | `serial` 为硬件序列号、`active_kind=usb` | 通过：3 台均 `active_kind=usb`，`serial` 为硬件序列号，USB 连接 `active=true` | [s2-devices-merged.json](s2-devices-merged.json) |
| T4 无线关掉 | `adb disconnect 10.0.0.104:5555` 后刷新 | 条目保留、USB 仍为当前、无线标离线、不出现第二张卡 | 通过：条目数仍为 3，该机无线连接 `online=false`，USB `active=true`；恢复 `adb connect` 后无线回到在线 | [s2-devices-wifi-off.json](s2-devices-wifi-off.json)、[s2-devices-wifi-back.json](s2-devices-wifi-back.json) |
| T7 不同设备不误并 | 同时接三台 | 各自独立条目 | 通过：3 个条目身份互不相同 | [s2-devices-merged.json](s2-devices-merged.json) |
| 标识名按身份 | `PATCH /api/devices/10.0.0.104:5555`（无线 serial）设别名 | 解析到同一台机，别名挂在合并条目上 | 通过：返回 `ok`，`GET /api/devices` 中 USB 条目显示该别名 | 本文第 3 节 |
| T12 自动化口径（部分） | `GET /api/automation/devices` | 一行一设备 | 通过：3 行（S6 的 `/fram.skill` 契约文本仍未改） | [s2-automation-devices.txt](s2-automation-devices.txt) |
| T3 USB 拔掉 | 物理拔线 | 条目保留、切到无线 | **未执行**：远程无法插拔，逻辑由假 ADB 单测覆盖 | `internal/device/grouping_test.go` |
| T5/T8/T9/T10/T11 | 全离线、IP 复用、恢复出厂、顺序、会话解析 | — | 未执行：依赖 S3–S6 或现场人工操作 | — |

## 3. 对照：旧代码与新代码

同一台机、同一时刻：

- `8080`（v2026.11.17-1，S2 之前）：**6 个条目**，双连接设备被拆成 2 张卡 → [prod-8080-devices-before.json](prod-8080-devices-before.json)
- `18090`（S1+S2 构建）：**3 个条目**，双连接设备合并为 1 张卡、两条连接 → [s2-devices-merged.json](s2-devices-merged.json)

别名写入实测：

```text
PATCH /api/devices/10.0.0.104:5555 {"alias":"验收机"}
→ {"ok":true}
GET /api/devices
→ 31040209940025Z  alias=验收机  identifier=验收机
```

## 4. 结论

S1（身份采集与字段）与 S2（按身份归并）在真机上通过 T1、T2、T4、T7 与标识名解析；T3 及其余用例待具备条件后补测；S3–S6 尚未实施。本地 Go 全量单测、Linux 构建与前端生产构建全部通过。
