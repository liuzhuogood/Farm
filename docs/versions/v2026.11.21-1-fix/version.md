# v2026.11.21-1：虚拟导航键布局修正

## 修复

- 虚拟导航键从投屏画面覆盖层移到画面下方，占满投屏区域宽度，不再遮挡画面。
- 取消画面内悬浮球，改为手机详情页“设置”面板中的“显示虚拟导航键”开关，默认关闭。
- 原生设备小窗启用虚拟键时自动增加窗口高度，避免挤压画面。
- 紧凑工具栏仍固定显示返回、切换任务、主页三个按钮。

## 验证

- `pnpm --dir frontend build`
- `cargo test --locked --manifest-path desktop/Cargo.toml`
- 浏览器实测画面底部与虚拟键顶部不重叠；主窗口虚拟键宽度等于投屏区域宽度，内嵌小窗宽度等于窗口宽度。
- [v2026.11.21-beta.4](https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.21-beta.4) 已发布，14 个资产齐全；Mac mini 下载发布版后 `/tasks`、`/player`、`/settings/feedback` 均返回 200。
- [v2026.11.21-1](https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.21-1) 正式补丁版已发布并设为最新版本，14 个资产齐全；Mac mini 下载正式版完成版本、投屏页与反馈页检查。
