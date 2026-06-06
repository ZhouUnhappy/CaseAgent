# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 后续目标

### 1. 用例质量反馈闭环（无需算法工程师）

**Trigger** —— 多人试用开始真实审核生成用例，频繁出现“这条有用 / 重复 / 缺步骤 / 不符合需求 / 知识缺失”等人工判断，需要把这些反馈沉淀为可查询样本，而不是只停留在聊天或临时备注里。

**DoD** —— 后端新增用例级反馈模型、API 与服务逻辑，反馈至少关联 `task_id`、`test_case_id`、case index/title、反馈类型、备注、`source_context` 摘要、prompt id/version、model call id；前端 `frontend/src/views/TaskDetail.vue` 在每条用例旁提供轻量反馈入口，支持标记有用、重复、缺步骤、不符合需求、知识缺失；`GET /api/v1/tasks/:id/trace` 或 `/ops` 能汇总反馈计数与失败/低质原因；新增后端测试覆盖反馈写入、查询、tenant 隔离与 trace 关联；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

### 2. 运维成本与稳定性看板

**Trigger** —— demo 进入多人或多 tenant 试用后，需要定期回答“哪个 tenant / provider / task 最耗 token、fallback 是否频繁、熔断/限流是否影响生成成功率、平均耗时是否变差”。

**DoD** —— 后端基于 `workflow_runs`、`agent_runs`、`model_calls`、`background_jobs` 增加只读聚合 API，支持按 tenant、时间范围、provider、model、workflow/task 过滤，返回 token/字符成本、调用次数、成功率、失败 stage、fallback 次数、限流/熔断/预算耗尽次数、平均/分位耗时；前端 `frontend/src/views/OpsWorkbench.vue` 增加成本与稳定性视图；新增后端测试覆盖聚合口径、空数据、过滤条件与 tenant 隔离；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

### 3. 生成策略 Profile 与轻量评测

**Trigger** —— prompt、检索参数、模型 provider 或预算策略开始频繁调整，需要在没有算法工程师的情况下，也能用固定样本比较不同策略对召回、成本、失败率和人工反馈的影响。

**DoD** —— 后端引入 generation profile 配置与持久化记录，至少覆盖 provider/model、prompt registry version、document/knowledge topK、多 query 数量、chunk 展示上限、预算/timeout/fallback 策略；每次任务在 workflow metadata / trace 中记录 profile id/version；新增脚本或测试数据目录维护一批固定需求、知识与期望覆盖点，能运行离线轻量评测并输出 JSON/Markdown 报告，包含生成数量、去重后数量、成本、失败 stage、命中 source_context 和人工反馈统计；新增测试覆盖 profile 解析、默认值、trace 写入；`cd backend && go test ./...`、相关评测脚本、`cd frontend && npm run build` 通过。

### 4. 用例审核体验升级

**Trigger** —— 试用用户开始一次性审核几十条以上用例，单条 JSON 编辑、逐段提交和缺少筛选导致审核成本明显高于生成成本。

**DoD** —— 前端 `frontend/src/views/TaskDetail.vue` 支持按 section、优先级、影响产品/模块、反馈状态、生成依据筛选用例；支持批量提交、批量修改优先级/影响范围、重复用例合并或隐藏；编辑体验从整段 JSON 扩展为结构化行内编辑或侧边编辑器，并保留 JSON 高级编辑入口；后端补齐批量更新/提交 API 与测试，确保 tenant 隔离和状态流转正确；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
- 质量评估产品化：暂不做 prompt 全文对比、prompt/model A/B 看板或质量趋势产品页；现阶段保留脚本生成的质量报告即可。
- 权限、操作者与审计：当前仍按可信本地 demo / 小范围试用处理，不引入登录、RBAC、操作者审计、危险操作二次确认。
- 知识库治理扩展：暂不做重复知识检测、冲突内容提示、过期知识提醒、知识覆盖率、chunk 来源高亮、知识变更影响任务分析。
- API 契约与自动化回归：暂不引入 OpenAPI、生成式 API client 或 Playwright 主流程 e2e。
- Demo 初始化与重置：现阶段继续使用 `scripts/demo_bootstrap.sh` 等脚本，不新增演示数据控制台或一键 reset 页面。
