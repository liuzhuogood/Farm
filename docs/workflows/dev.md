# 开发工作流（dev）

用途：按版本设计实现功能，自测通过后提交。

1. 读取 `docs/versions/<版本>/` 下的 `design.md`、`tasks.md`、`version.md`，用 LZVlog 技能按任务清单逐项推进，完成后据实勾选。
2. 缺陷先验红再修绿，新逻辑先写断言；关键流程、异常与外部调用补必要日志。
3. 改动后跑对应模块单测与 `go test ./...`，前端改动再跑 `pnpm --dir frontend build`，红则不进提交。
4. 提交按当前任务精确暂存，中文 Conventional Commit，不带入无关改动。
