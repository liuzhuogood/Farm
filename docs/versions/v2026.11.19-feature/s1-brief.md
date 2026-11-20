# S1 开发任务书 · 通道身份采集与接口字段

依据：[双连接设备归并设计](../../designs/2026-11-19-dual-link-device-merge.md) 第 1、2、6、8、9 节 · 仓库 `/Users/liuzhuo/code/Farm` · 2026-11-19

## 范围

只做 S1：给每个 adb 通道采集设备自身身份，并把身份与连接信息通过 `GET /api/devices` 暴露出来。**不做归并**（仍是每 serial 一条）、不改数据库、不改前端、不动 `/player` 与 `/ws` 的 serial 语义。

## 要做的

1. 通道在线且 `adb devices` 状态为 `device` 时采集身份，按顺序：
   - `adb -s <serial> shell getprop ro.serialno`
   - `adb -s <serial> shell getprop ro.boot.serialno`
   - `adb -s <serial> shell settings get secure android_id`
2. 校验后才采用：去空白；非空；不等于 `unknown` / `null` / `0` / `0123456789ABCDEF` / 全零；长度 4–64。加前缀 `hw:` 或 `aid:`。
3. 身份按通道 serial 缓存；取不到时 `identity_source=pending`，不影响现有列表；每条通道从离线变在线或重连成功时重采一次，身份变化打日志 `[Device] channel identity changed serial=... old=... new=...`。
4. `Device` 追加字段并在 `GET /api/devices` 返回：
   ```json
   {
     "identity": "hw:31040209940025Z",
     "identity_source": "ro.serialno",
     "active_kind": "usb",
     "connections": [
       { "kind": "usb", "serial": "31040209940025Z", "address": "", "state": "device", "online": true, "active": true, "last_seen_at": 1763550000000 }
     ]
   }
   ```
   `active` 规则：USB 在线取 usb，否则取 wifi；都离线时 `active_kind` 为空、`connections` 各自 `online=false`。同 kind 多条时只把 `last_seen_at` 最新的一条放进 `connections`，其余留在内存通道表。
5. 身份采集失败（未授权、命令超时）只打日志，不阻塞刷新循环，不影响其他设备。

## 验收（农场测试员照此验）

- 同一台手机插 USB + 开无线调试后，两条记录 `identity` 相同，`identity_source=ro.serialno`；`connections` 各一条，`active_kind=usb`。
- 拔掉 USB：`active_kind` 变为 wifi，`connections` 中 usb `online=false`。
- 新机插 USB 但不点“允许调试”：该条 `identity_source=pending`，`identity` 为空，列表条数不变、不报错。
- 无设备时 `GET /api/devices` 返回 `[]`，行为与今天一致。
- 单测覆盖：三个来源的优先级与校验（用假 adb 输出），身份缓存的失效与重采。

## 交付要求

- 单独分支；提交说明写清 S1 范围；不部署、不改生产配置。
- 提交前 `go test ./internal/device/...`、`go build ./...` 通过。
- 交付时说清：改了哪些文件、怎么验证、剩下哪些属于 S2（归并、缓存改 identity、门控）。
