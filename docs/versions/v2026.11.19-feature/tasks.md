# v2026.11.19-feature 任务清单

设计：[docs/designs/2026-11-19-dual-link-device-merge.md](../../designs/2026-11-19-dual-link-device-merge.md) · 方向：方案一 并列图标（已定） · 拍板：A 改、D 不列，B/C 按建议

## 已完成

- [x] 双连接设备归并设计与影响面梳理，输出设计文档（现状/归并后流程、通道状态机、数据结构、影响面、S1–S6、T1–T12 用例、风险）。
- [x] 按 ui-prototype 技能做两个可运行原型并交付公网预览与截图；用户选定方案一（并列图标），方案二留档。
- [x] 拍板 A–D，写进设计文档第 7 节与本版本记录。
- [x] 建立 v2026.11.19-feature 版本记录，登记设计文档、分步与 S1 任务书。
- [x] **S1 通道身份采集与接口字段**：`b871fa1` 并入 main；10.0.0.6 真机验收通过（同一台机两条连接身份相同、`identity_source=ro.serialno`、未授权为 `pending`，字段齐全），单测覆盖来源优先级与校验。
- [x] **S2 后端按身份归并**：按 identity 分组、`serial`=当前通道、USB 优先；真机通过 T1、T2、T4、T7 与标识名解析，T3（物理拔 USB）与其余用例待补测。别名/遥测/硬件缓存改按 identity；`devicegate` 本就是全局单门，无需按 serial 改造。

## 待开发

- [ ] **S3 前端卡片与本地偏好迁移**（待 S2 剩余用例）
  - 依据设计文档第 4 节方案一；本地排序/隐藏/选择 key 改 identity 并迁移老偏好；验收 T10 与“切通道后编号、顺序不变”。
- [ ] **S4 详情页连接面板与 serial 解析**（待 S3 验收）
  - 只读「连接方式」列表（图标 + 该通道 serial/地址 + 在线状态 + 当前标记）；`/api/devices/{serial}` 接受任意通道 serial（`PATCH`/`DELETE` 已可按身份或任一通道 serial 解析，只读 GET 未提供）；验收 T11。
- [ ] **S5 数据库迁移与手动登记并入通道表**（待 S4 验收）
  - 新增两张表、一次性迁移、回滚可用；自动重连按设备去重；验收 T5、T8、T9。
- [ ] **S6 自动化与 MCP 口径改「一行一设备」**（待 S5 验收）
  - `/api/automation/devices` 与 MCP `phone_devices` 已随归并变为一行一设备；剩余同步 `/fram.skill` 契约与设备发现说明；验收 T12。

## 验收记录

| 步骤 | 结论 | 证据 |
| --- | --- | --- |
| S1 | 真机验收通过（10.0.0.6，3 台双连接设备身份一致且 `identity_source=ro.serialno`），已并入 main | [evidence/s1-s2-acceptance.md](evidence/s1-s2-acceptance.md)、[evidence/s1-s2-report.html](evidence/s1-s2-report.html)；`b871fa1` |
| S2 | 真机通过 T1、T2、T4、T7 与标识名解析；T3（物理拔 USB）与其余用例待补测 | 同上；`evidence/s2-devices-*.json`、`evidence/prod-8080-devices-before.json`（旧代码 6 条 vs 新代码 3 条）。本地 `go test ./...`、Linux 构建、前端构建通过 |
