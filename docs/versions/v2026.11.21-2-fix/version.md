# v2026.11.21-2：虚拟导航键与更多菜单调整

## 变更

- 虚拟导航键顺序调整为切换任务、主页、返回；小窗虚拟键栏增加“更多”按钮。
- “更多”菜单抽成 `PlayerMoreMenu.vue`，工具栏与虚拟键栏共用同一份菜单。
- “窗口效果”“音量”改为图标在左、文字在右，与其他菜单项一致。
- 左侧菜单顶部去掉横向分割线，顶部高度与页面工具栏统一为 44px。
- 修复非 macOS 平台 `builder` 多余 `mut` 导致的 clippy 失败。
- `desktop.yml` 补 clippy 组件、Windows 限定 NSIS 并加入独立 GUI 服务验证。

## 验证

- `go test ./...`、`cargo test --locked`、`cargo clippy -- -D warnings`、前端生产构建、版本同步测试全部通过。
- Windows / macOS / Linux 原生 CI：GitHub Actions [45094006827](https://github.com/liuzhuogood/Farm/actions/runs/45094006827) 三个作业全部成功。
- 浏览器实测：虚拟键顺序为切换任务、主页、返回、更多；“更多”菜单可正常打开且样式统一。
- [v2026.11.21-beta.5](https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.21-beta.5) 与 [v2026.11.21-2](https://github.com/liuzhuogood/Farm/releases/tag/v2026.11.21-2) 已发布，各 14 个资产齐全，正式版已设为最新版本；Mac mini 下载正式版后 `/player`、`/tasks`、`/settings/about` 均返回 200。
- 正式版首轮发布时 macOS x64 的 `bundle_dmg.sh` 失败，重跑该作业后全部成功；同一代码在 beta.5 同一步已通过，判定为 runner 偶发。
