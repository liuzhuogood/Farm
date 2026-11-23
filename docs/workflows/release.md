# 正式发布工作流（release）

用途：测试通过后，把 beta 验收过的同一份代码打成正式版，部署生产并向正式通道推送。

前提：[test.md](test.md) 已通过，beta tag 的提交与 main 最新提交一致，beta 之后没有未经 beta 验收的改动。

1. 确认状态：在 main 分支、工作区干净，`VERSION` 等于目标正式号（与本次 beta 的日期段一致）；若 beta 之后还有改动，先重发一个 beta 并重测。
2. 提交推送：按当前任务精确暂存并提交，再推送 main。
3. 打正式 tag：与 `VERSION` 完全一致，推送 tag 触发发布流水线。
4. 跟踪 GitHub Actions：`release.yml`、`macos-dmg.yml` 等发布流程跑到全部成功；beta 跳过的 Docker 与 PyPI 在正式发布里恢复正常，PyPI 只在 SDK 版本有变更时才发。
5. 核验 Release 产物：版本号、五平台 CLI、Windows GUI 与安装包、macOS 两种 DMG 与更新包、`desktop-latest.json` 齐全；Release 为正式标记，`releases/latest` 指向新版本；失败则修复重跑，不以命令退出码代替结果核对。
   - Windows 侧必须包含原生 runner 的桌面作业成功结论，具体步骤与验收口径见 [test.md](test.md#windows-原生测试)。
6. 部署生产：`bash scripts/deploy/deploy-prod.sh` 覆盖 `/opt/farm` 并重启 `farm.service`，健康检查通过。
7. 更新核验：正式通道客户端在应用内能发现并安装新版本；beta 通道客户端收到同号正式版；beta 服务实例按需停用，留作下个版本的 beta。
8. 收尾：版本目录 `version.md` 的状态与 `tasks.md` 验收记录更新到实际发布结果，区分已实现、已提交、已推送与已部署。
