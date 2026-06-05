# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 后续目标

### 1. 模型成本与稳定性控制

**Trigger** —— demo 开始持续使用真实模型 provider 或多人试用时，出现 token / 请求成本不可控、provider 限流/超时导致任务失败或长时间卡住，或需要按 tenant / task 设置模型调用预算与降级策略。

**DoD** —— 后端在 `backend/internal/ai/`、`backend/internal/service/agent/`、`backend/internal/service/job/` 增加模型调用 guardrail：记录每个 task / tenant 的 prompt、completion、总 token 或可替代字符估算成本；支持配置单任务预算、tenant 并发上限、provider 超时、失败熔断和可选 fallback provider；预算耗尽或熔断时任务进入可解释失败状态并写入 workflow trace / model_calls metadata；前端 `/tasks/:id` 或 `/ops` 能看到成本、限流、熔断、fallback 结果；新增后端测试覆盖预算耗尽、provider 超时、fallback 成功/失败、熔断短路；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

### 2. 检索与生成可调试性

**Trigger** —— demo 用户开始追问“为什么生成了这条用例 / 为什么没有生成那条用例”，或 prompt、检索参数、知识库内容调整后，需要从单条用例反查 query、命中 chunk、agent、prompt version、model call 和 source_context 证据链。

**DoD** —— 后端把生成结果与检索/Agent/模型调用 trace 建立可查询关联：`test_cases.source_context` 或 workflow artifacts 能定位到相关 document/knowledge chunk、retrieval query、rank/score、agent run、prompt id/version、model call id；`GET /api/v1/tasks/:id/trace` 或新增只读 endpoint 返回 case-level provenance；前端 `frontend/src/views/TaskDetail.vue` 在每条用例旁提供“生成依据/调试”入口，展示命中的知识/文档片段、query、score、agent 与 model call 摘要；新增后端测试覆盖 provenance 写入与 trace API 输出，前端构建通过；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
- 质量评估产品化：暂不做 prompt 全文对比、prompt/model A/B 看板或质量趋势产品页；现阶段保留脚本生成的质量报告即可。
- 权限、操作者与审计：当前仍按可信本地 demo / 小范围试用处理，不引入登录、RBAC、操作者审计、危险操作二次确认。
- 知识库治理扩展：暂不做重复知识检测、冲突内容提示、过期知识提醒、知识覆盖率、chunk 来源高亮、知识变更影响任务分析。
- 用例审核体验升级：暂不做批量编辑、结构化 diff、版本历史、复杂筛选或外部测试管理系统导出。
- API 契约与自动化回归：暂不引入 OpenAPI、生成式 API client 或 Playwright 主流程 e2e。
- Demo 初始化与重置：现阶段继续使用 `scripts/demo_bootstrap.sh` 等脚本，不新增演示数据控制台或一键 reset 页面。
