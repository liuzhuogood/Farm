# Farm MCP 与安全审核界面验收

## 无内卡片与应用图标

- 原生审核窗去掉内部卡片边框和通用“审核请求”大标题；有实际业务标题时以普通正文保留。
- 首页与审核窗复用 AppGlyph 矢量图标；审批创建时读取绑定设备的包名，并固定对应应用名称和图标标识。未知应用不冒用其他图标。
- 实测 emulator-5554 的系统设置应用：审核窗显示 Settings 图标，拒绝后 MCP 返回 deny，窗口关闭；首页图标显示正常。
- [实际截图](21-approval-app-icon-plain.png)。前端、原生构建与全量 Go 测试通过；相关后端 race 通过。未提交推送。

## Unslop 与独立多窗验收

- 每条审核独立置顶窗口，错位打开；窗口收紧为 420×300，弱化次要操作，只强调允许一次。
- 实测并发 A/B 请求：B 通过并关闭后 A 仍保留；随后拒绝 A，A 独立关闭。
- [优化后实拍](17-unslop-approval.png)、[处理 B 后 A 保留](18-independent-approval-remaining.png)。原图见 15-native-approval-popup.png。
- 前端与原生构建、Rust 5 项测试、全量 Go 测试、版本及 diff 检查通过。首次 Go 全测的既有设备状态测试失败，单测及全测重跑均通过，未改该模块。
- 本轮视觉实测为 macOS 浅色原生窗口；深色、Web 窄屏与跨桌面空间未实测。未提交推送。

本次补齐 AI 集成页 MCP 管理与安全审核操作界面，参考本机 Pinky 源码。未提交、未推送、未发布。

## 验证结果

- Cursor 真实界面安装、读回已安装状态、卸载、读回未安装状态通过。
- Codex、Claude Code、Cursor 配置预览齐全；三客户端配置安装卸载及其他配置保护由临时目录自动化测试覆盖。没有验证 AI 客户端重启后的工具发现。
- 真实 MCP phone_request_approval 经界面分别允许一次、拒绝、人工接管，调用方收到 allow_once、deny、take_over。测试请求不执行设备动作。
- 四档策略可选，保存并 GET 回读；测试后恢复支付审核。策略当前为进程内设置，重启恢复默认。
- 任务列表、详情和导出已真实交互验证；审计数据为明确标注的验收记录，不代表设备执行。
- Go 相关测试、race、go build ./...、前端构建、版本检查与 diff --check 通过。原始输出见 validation.log。

## 验收边界

本次验证 Web 界面，未验证原生桌面打包或真机。现有安全门禁主要覆盖 MCP 动作链，不代表全部 HTTP 自动化接口已受保护。未暴露尚未完整实现的本任务允许/始终允许决策。既有报告中全量通过的表述不作为本次验收依据。

## 截图目录

- [策略保存及回读](01-policy-saved.png)
- [真实 MCP 请求进入待审核](02-pending-approval.png)
- [允许一次后队列清空](03-approved.png)
- [拒绝后队列清空](04-denied.png)
- [MCP 配置预览](05-mcp-config.png)
- [Cursor 安装完成](06-mcp-installed.png)
- [Cursor 卸载完成](07-mcp-uninstalled.png)
- [任务审计详情](08-audit-detail.png)
- [导出报告实际页面](09-audit-export.png)
- [人工接管后队列清空](10-take-over.png)
- [Codex 配置预览](11-codex-config.png)
- [Claude Code 配置预览](12-claude-config.png)
- [自动化测试验证](13-tests.png)
- [构建及版本检查](14-build.png)

## 原生审核小窗补充验收

- 主窗口停留设备列表，实际 MCP 请求自动创建独立安全审核窗。
- 原生窗口使用 always_on_top(true)，同一标签复用窗口，轮询不重复抢焦点。
- 窗口正确展示请求与三项决策；实际允许后 MCP 返回 allow_once，队列清空后窗口自动关闭。
- 修复原生新路由 /approval 静态页面白名单和初始路由尚未就绪时审核窗未轮询的问题。
- 前端构建、cargo check、原生 App 构建、Go 静态页面路由回归测试、版本同步与 diff 检查通过。
- 截图：[原生审核小窗](15-native-approval-popup.png)、[审核完成后回到主窗口](16-native-approval-closed.png)。
- 未验证全屏独占应用或跨桌面空间置顶；Web 仅页面内浮层。尚未提交推送。
