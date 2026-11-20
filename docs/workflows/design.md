# 设计工作流（design）

用途：用 lz-dev 的 lz-vlog 模块 与原型技能把版本的设计、任务和验收口径定下来。

1. 版本记录：用 LZ-Vlog 技能在 `docs/versions/<版本>/` 建立并维护 `design.md`、`tasks.md`、`version.md` 三件套，设计先行、任务可验收。
2. 需要界面时用 lz-prototype 技能做可运行原型，交付源码与截图；多方案由用户选定，只把选定方案写进版本 `design.md`。
3. 设计文档只写已确认的方案、数据与接口口径；未定项标「待确认」，按项目事实核对，不用猜测补全。
4. 新增或改动页面、面板、核心概念时，同步登记 `docs/glossary.md`。

可使用技能：
lz-dev 的 lz-ui-style 模块
lz-dev 的 lz-unslop 模块
lz-dev 的 lz-dbx 模块
lz-kb 知识库技能（如果有要求要阅、记时使用）
lz-auto 自动化技能