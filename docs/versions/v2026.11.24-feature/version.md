# v2026.11.24：设备小窗交互与虚拟导航键

## 变更

- 首页点击设备默认打开小窗，补充群控使用说明。
- 虚拟导航键改为按设备单独开关，并保留贴底全宽布局。
- 小窗内的设备详情改在主窗口打开，避免小窗内再开窗口。
- 桌面诊断日志写入方式修正。
- 新增首次安装指引与设备分组。
- 修复 Windows 小窗创建死锁与跨平台 lint。
- 虚拟导航键顺序调整为切换任务、主页、返回，并增加“更多”。

## 验证

- `go test ./...`、`cargo test`、`cargo clippy -D warnings`、前端生产构建、版本同步测试。
- Windows / macOS / Linux 原生 `Desktop build` 三个作业成功（[45245093253](https://github.com/liuzhuogood/Farm/actions/runs/45245093253)）。

## 发布

- beta.1：https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.24-beta.1
- beta.2：[v2026.11.24-beta.2](https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.24-beta.2)，Actions [45301201638](https://github.com/liuzhuogood/Farm/actions/runs/45301201638) 全部成功。
- 正式版：[v2026.11.24](https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.24)，Actions [45303377366](https://github.com/liuzhuogood/Farm/actions/runs/45303377366) 全部成功，14 个资产齐全并设为最新版本。
- 生产部署：`10.0.0.6` `/opt/mywebscrcpy/farm` 已更新到 `v2026.11.24`，`mywebscrcpy.service` 为 `active`，首页、任务记录、API、反馈页均返回 200。
