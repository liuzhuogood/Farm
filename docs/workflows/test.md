# 测试工作流（test）

用途：在 beta 发布之后对 beta 产物做全量验收，通过才进入正式发布。测试对象是 main 打出的 beta 包与 beta 服务实例，不是本地开发环境。

前提：已按 [release-beta.md](release-beta.md) 发出 beta，并拿到 beta 版本号、Release 链接与 8081 地址。

1. 核对测试对象：beta tag 指向的提交与 main 最新提交一致；本地开发环境的临时改动不计入验收结论。
2. 覆盖该版本全部功能点，逐条对应 `tasks.md` 的验收口径；未通过退回开发，不跳过、不降级为「人工确认」。
3. 不通过时退回开发：说明差在哪，开发修复后重发 beta.N+1 再重测，不复用旧 beta 的结论。
4. 每步保留证据：截图、日志、请求与响应，归档到 `docs/versions/<版本>/evidence/`。
5. 在版本目录生成自包含 HTML 测试报告，可离线打开。
6. 报告中的图片用（lz-act 的 OSS 模块）上传，目录固定 `test/`，正文引用 OSS 链接。
7. 全部通过后更新 `tasks.md` 的验收记录，进入 [release.md](release.md)。

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
6. （后面考虑）Windows 测试。
7. （后面考虑）Linux 测试。
