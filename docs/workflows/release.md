# 发布工作流（release）

用途：提交推送、打 tag，并跟到 GitHub Actions 发布成功。

1. 如果当前在版本分支（非main）分支时， 应该切回main分支，并合并版本分支代码。
2. 提交：按当前任务精确暂存并提交，再推送主分支。
3. 打版本 tag：版本号与 `VERSION` 一致，推送 tag 触发发布流水线。
4. 跟踪 GitHub Actions：`release.yml`、`macos-dmg.yml` 等发布流程跑到全部成功。
4. 核验 Release 产物：版本号、五平台 CLI、macOS 包与清单齐全；失败则修复重跑，不以命令退出码代替结果核对。
