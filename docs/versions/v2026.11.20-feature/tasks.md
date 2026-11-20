# v2026.11.20-feature 任务清单

设计：[docs/designs/2026-11-20-update-channels.md](../../designs/2026-11-20-update-channels.md) · 决定：beta 全平台、沿用签名、PyPI 按需发布、桌面端不出 `-N`（方案 A）

## 已完成

- [x] **S1 版本比较与通道后端**
  - `internal/updater/checker.go`：自定义优先级比较（`beta.K` < 无后缀 < `-N`），beta 通道走 `/releases?per_page=30` 过滤 draft 与 `-beta.*` 预发布并取较高者；协议请求抽成 `getJSON`。
  - `internal/updater/channel.go`：通道常量与校验、`ConfigStore`、桌面更新索引解析（beta 只认 `-beta.*`，无 beta 时回退正式索引并报告实际生效通道）。
  - `internal/updater/handlers.go`：`GET/PUT /api/version/channel`、`GET /api/version/desktop-endpoint`、`/api/version/check?channel=`（临时覆盖不保存）；升级复用已保存通道；服务启动时从系统配置载入通道。
  - `main.go`：启动时绑定数据库为通道配置存储。
  - `web/openapi.json`：新增两个通道接口与 `channel` 字段，`VersionCheckResult` 补 `channel`。
  - 验证：`go vet ./internal/updater/` 干净，`go test ./...` 35 包全绿，macmini 真机隔离实例通过（见验收记录）。

- [x] **S2 桌面与前端通道接入**
  - `desktop/src/updater.rs`：`builder()` 改为异步，先向本机后端取 `GET /api/version/desktop-endpoint`；后端不可用或地址非 https 时回退正式通道并记日志；检查与安装共用同一地址；新增 3 项单测。
  - `frontend/src/api/updater.ts`、`useUpdateSettings.ts`：通道读写走后端配置，读取失败按正式通道处理。
  - `frontend/src/components/system/SystemConfigDialog.vue`：「检查更新」行内加入通道下拉与「立即检查更新」图标按钮，切换通道后立即重新检查，失败弹提示。
  - 验证：`pnpm --dir frontend build`（含 `vue-tsc`）、`cargo test` 13 项、macmini 浏览器真机切换通过。

## 待开发

- [ ] **S3 beta 发布流水线**（待 S1/S2）
  - `scripts/sync-version.mjs` 支持 `--version <beta号>`，覆盖 `VERSION` 与全部产物版本文件且不提交。
  - `.github/workflows/release.yml`：beta tag 或手动触发时校验 `tag == v$(VERSION)-beta.N`、构建前覆盖版本、`prerelease: true`、跳过 docker job；桌面端不产 `-N` 号。
  - 校验 beta 产出的 9 个程序与 `desktop-latest.json` 齐全，签名与正式包同密钥。

- [ ] **S4 服务器 beta 实例**（待 S3）
  - `scripts/deploy/deploy-beta.sh`：端口、路径、服务名参数化，健康检查指向 beta 端口。
  - `docs/environments/beta.md`：登记 10.0.0.6 的 `farm-beta.service`、8081、`/opt/farm-beta`、独立 SQLite。
  - 验证双实例共存：8080 与 8081 同时可访问；同机 adb 与投屏是否互抢单独出结论，未通过则 beta 不接生产设备。

- [ ] **S5 SDK 独立版本与按需发布**（可与 S3/S4 并行）
  - 新增 `packages/python-sdk/VERSION`（起始 `2026.11.18`）与 `scripts/sdk-version.mjs`（同步 + `--check`）。
  - `scripts/sync-version.mjs` 移除 SDK 三项；`python-sdk.yml` 触发路径与版本校验改为 SDK 自身版本。
  - CI 检查：改动 SDK 源码却未改 `packages/python-sdk/VERSION` 时失败。

- [ ] **S6 工作流与验收**（待 S1–S5）
  - `docs/workflows/release-beta.md` 已建；推进到实际 beta 发布时补 `test.md` 与 `release.md` 的实测步骤。
  - `docs/updating.md`、`docs/environments/beta.md` 补通道、beta 发布与部署；`docs/glossary.md` 登记「更新通道」。
  - 执行 T1–T7；beta 自更新（下载、验签、安装、回退）需 S3 发布真实 beta 后验证。

## 验收记录

| 步骤 | 结论 | 证据 |
| --- | --- | --- |
| S1 | macmini 真机隔离实例通过：通道读写、非法值 400、beta 取正式、重启持久化 | [evidence/s1-s2-macmini-selftest.md](evidence/s1-s2-macmini-selftest.md) |
| S2 | macmini 真机验证下拉切换与后端一致、立即检查弹窗可用；自测发现并修复「无 beta 时把正式 tag 拼成下载地址」 | [evidence/s1-s2-macmini-selftest.md](evidence/s1-s2-macmini-selftest.md)、`evidence/farm-channel-ui-beta.png`、`evidence/farm-channel-ui-check.png` |
| S3–S7 | 待开工 | — |
