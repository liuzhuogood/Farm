# 测试工作流（test）

用途：覆盖版本全部功能点，留下可追溯的证据与 HTML 报告。

1. 如果当前在版本分支（非main）分支时， 应该切回main分支，并合并版本分支代码。
2. 覆盖该版本的全部功能点，逐条对应 `tasks.md` 的验收口径；未通过退回开发，不跳过、不降级为「人工确认」。
3. 每步保留证据：截图、日志、请求与响应，归档到 `docs/versions/<版本>/evidence/`。
4. 在版本目录生成自包含 HTML 测试报告，可离线打开。
5. 报告中的图片用 （lz-act 的 OSS 模块）上传，目录固定 `test/`，正文引用 OSS 链接。

可使用技能：
lz-dev 的 lz-ui-style 模块
lz-dev 的 lz-unslop 模块
lz-dev 的 lz-dbx 模块
lz-kb 知识库技能（如果有要求要阅、记时使用）
lz-auto 自动化技能

测试场景要覆盖：
1. 本机MacOS进行GUI测试
2. 使用 10.0.0.6 上的服务进行自动更机更新，部署后在可以用 https://farm.s5.5slive.com 访问
3. （后面考虑）windows测试
3. （后面考虑）linux测试
