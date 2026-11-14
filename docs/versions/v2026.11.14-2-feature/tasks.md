# v2026.11.14-2-feature 任务清单

## 已完成任务
- [x] 在前端实现 `SkillContractPanel.vue`，集成 `/fram.skill` 契约文档与安装动作合一。
- [x] 更新 `frontend/src/api/aiIntegrations.ts`，支持 `/fram.skill` 权威契约内容拉取与解析。
- [x] 更新 `frontend/src/views/AiIntegrationView.vue`，将技能文档契约面板嵌入 AI 集成页。
- [x] 更新 `internal/skillsmarket/catalog.go`，完善通用基础设备技能的契约描述。
- [x] 更新 `internal/mcp/server.go`，在输入安全评估中注入 `ElementLabel` 意图，防止向凭据字段输入。
- [x] 更新 `docs/glossary.md`，对齐「自动化技能文档面板」术语。
- [x] 编写 `scripts/run_comprehensive_e2e_tests.py`，完成 17 项全链路端到端自动化测试与证据收集。
- [x] 归档测试证据与 HTML 测试报告至 `docs/verification/`。
- [x] 同步版本至 `v2026.11.14-2` 并完成版本一致性校验。

## 待开发任务
- [ ] 针对 iOS / 鸿蒙等更多设备底层的通用契约与感知支持。
- [ ] 自动化技能市场在线动态拉取与第三方社区源订阅机制。
- [ ] 技能执行的自适应降级（纯视觉 OCR 坐标推断）。
- [ ] 多模态视觉匹配在 MCP 工具集中的动态流水线组合。
