# Farm 项目改名记录

1. 仓库统一使用 `liuzhuogood/Farm`，本机主目录为 `/Users/liuzhuo/code/Farm`，已修复关联 Git 工作树指针。
2. 产品显示名为 `Farm`，Go module、CLI、SDK import 使用 `farm`；环境变量使用 `FARM_` 前缀。桌面标识为 `com.farm.desktop`，移动端标识为 `com.farm.mobile`。
3. 本机数据库迁为 `data/farm.db`，迁移时通过 SQLite 备份、完整性和记录数检查。桌面数据目录、浏览器偏好、Python 环境及依赖绝对路径一并迁移；原程序和原始数据库已另行备份。
4. Python SDK 名称为 `farm-sdk`。服务内嵌同版本 SDK 源码，使用标准 pip 安装，项目 SDK 无需依赖尚未发布的 PyPI 包；第三方依赖仍由 pip 安装。
5. 已生成 `Farm.app`、`Farm_2026.11.8_aarch64.dmg`、`farm`、`farm-gui`、`farm_sdk-2026.11.8-py3-none-any.whl` 与 `farm_sdk-2026.11.8.tar.gz`。
6. Go 全量测试、前端构建和现有测试、原型检查、SDK 的 100 项测试、wheel/sdist 安装及 Rust 检查通过。已完成本机 macOS 构建；移动端仅检查配置，未执行 Flutter 构建。

本次未发布 PyPI 包、Docker 镜像或新的正式版本标签。
