# 测试工作流


可使用技能：
lz-dev 的 lz-ui-style 模块
lz-dev 的 lz-unslop 模块
lz-dev 的 lz-dbx 模块
lz-kb 知识库技能（如果有要求要阅、记时使用）
lz-auto 自动化技能

测试场景要覆盖：

1. 本机 macOS GUI：安装 beta 桌面包后走真实交互。
2. beta 服务实例：用 10.0.0.6 的 8081 实例做自动化验证与设备链路，正式 8080 只做「未受影响」的回归确认。
3. 通道隔离：正式通道客户端在 beta 存在时不提示更新。
4. 通道切换与回退：切到 beta 能装 beta；同号正式版发布后 beta 客户端能升级到正式版。
5. 应用内更新：部署后通过 `https://farm.s5.5slive.com` 访问做一次真实的检查更新与安装。
6. Windows 原生测试（见下节）。
7. （后面考虑）Linux 测试。

## Windows 原生测试

Windows 不能只靠 macOS 上的交叉编译结论。正式发布前必须在原生 Windows 上验证，仓库既有的做法是用 GitHub Actions 的 `windows-latest` runner 承载，测试项与发布流水线一致，失败会阻断 Release。

1. 触发与观测：
   - 推送 tag 后由 `Build and Release` 的 `desktop (windows-latest, windows-x86_64, nsis)` 作业执行；PR 与手动验证用 `Desktop build`（`macos-latest` / `windows-latest` / `ubuntu-24.04` 矩阵）触发。
   - 记录 Actions 运行链接与各步骤结论，不用退出码代替结果核对。
2. 必须通过的 Windows 步骤（与 `release.yml` 一致）：
   - `Build complete signed desktop packages`：NSIS 安装包与独立 GUI exe 构建成功，签名配置就绪。
   - `Verify updater signature and corruption rejection`：`node scripts/verification/update-signature.mjs`，验证 minisign 签名与篡改拒绝。
   - `Go regression`：`python -m pip install ./packages/python-sdk` 后 `go test ./...`，Windows 原生全量 Go 回归。
   - `Verify db package`：`go test -count=3 ./internal/db`。
   - `Verify embedded service cache`：`cargo test --locked --manifest-path desktop/Cargo.toml embedded_service::tests`。
   - `Verify standalone Windows GUI service`：`node scripts/verification/standalone-gui.mjs <独立 GUI exe>`，验证内嵌 Go 服务释放、启动、`/api/desktop/health` 与受控关闭。
   - `Verify desktop service lifecycle`：`node scripts/verification/desktop-service.mjs`，验证回环就绪、私有控制接口、依赖路径、升级守卫、SPA 路由与干净退出。
   - `Verify release manifest contracts`：`node scripts/verification/release-manifest.mjs`，验证发布资产清单与索引。
3. 产物核对：Windows 侧应产出 `farm-windows-cli-amd64.exe`、`farm-windows-gui-amd64.exe`、`farm-<版本>-windows-x86_64-gui-setup.exe` 及 `desktop-latest.json` 中的 `windows-x86_64` 条目；缺任一项视为未通过。
4. 已知 Windows 差异（不视为缺陷，但要确认覆盖）：
   - 控制台默认代码页为 cp1252，中文输出需 UTF-8 或显式设置编码。
   - 假可执行文件为 POSIX shell 脚本的用例按仓库约定在 Windows 跳过。
   - `fsnotify` 目录监听在 Windows 关闭时可能短暂占用句柄，删除需重试。
5. 记录未覆盖边界：Windows 安装包真实安装升级、真实设备投屏交互、系统托盘与原生菜单行为仍需在原生 Windows 手工确认，不能由 CI 绿灯推定。
