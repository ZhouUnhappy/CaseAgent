# 后续优化方向（Future Work）

本文记录已识别但**尚未完成或尚未启动**的优化方向，按预期价值与成熟度排序。

当前团队假设：只有前端、后端工程师；没有算法工程师、数据工程师或专职测试工程师，也不计划补充这些角色。后续目标只纳入前后端工程师可以独立设计、实现、维护和验证的工作；需要模型训练、复杂数据平台、特征工程、专职测试平台或大规模离线实验的事项不进入当前计划。

## 团队能力边界

### 可以进入计划的工作

- 前端页面、交互、表格、筛选、批量操作、轻量审核流程。
- 后端 API、服务层、数据库模型、迁移、RLS / tenant 隔离、普通 SQL 聚合。
- 任务 trace、metadata、prompt/profile 版本记录、人工反馈样本沉淀。
- 本地脚本、固定样本报告、demo bootstrap/reset、README/脚本文档。
- 由前后端工程师维护的单元测试、集成测试、少量关键流程验证命令。

### 不进入当前计划的工作

- 需要算法工程师长期维护的模型能力：自动质量评分、语义重复检测模型、reranker、query rewrite 模型、embedding 训练、LLM-as-judge、大规模模型效果实验平台。
- 需要数据工程师长期维护的数据能力：数据仓库、BI 平台、特征工程 pipeline、复杂趋势预测、跨版本自动归因、大规模离线实验数据链路。
- 需要专职测试工程师长期维护的平台能力：完整自动化回归平台、大量端到端用例矩阵、跨浏览器/跨环境测试平台。

## 文档约定

每项使用三段式：

- **需要角色**：只写当前团队已有角色，例如前端工程师、后端工程师。
- **Trigger**：什么信号出现时启动这件事，避免过早优化。
- **DoD**：启动后怎么算完成，可机器复核；需要明确文件路径、API 路径、测试覆盖、验证命令。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护“做了哪些”。已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 后续目标（只含前后端可独立交付）

### 1. 用例质量反馈闭环

**需要角色**：前端工程师、后端工程师。

**Trigger**：多人试用开始真实审核生成用例，频繁出现“这条有用 / 重复 / 缺步骤 / 不符合需求 / 知识缺失”等人工判断，需要把这些反馈沉淀为可查询样本，而不是只停留在聊天或临时备注里。

**DoD**：后端新增用例级反馈模型、API 与服务逻辑，反馈至少关联 `task_id`、`test_case_id`、case index/title、反馈类型、备注、`source_context` 摘要、prompt id/version、model call id；前端 `frontend/src/views/TaskDetail.vue` 在每条用例旁提供轻量反馈入口，支持标记有用、重复、缺步骤、不符合需求、知识缺失；`GET /api/v1/tasks/:id/trace` 或 `/ops` 用普通 SQL/服务层逻辑汇总反馈计数与失败/低质原因；不引入自动质量评分模型；新增后端测试覆盖反馈写入、查询、tenant 隔离与 trace 关联；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

### 2. 运维成本与稳定性表格

**需要角色**：前端工程师、后端工程师。

**Trigger**：demo 进入多人或多 tenant 试用后，需要定期回答“哪个 tenant / provider / task 最耗 token、fallback 是否频繁、熔断/限流是否影响生成成功率、平均耗时是否变差”。

**DoD**：后端基于 `workflow_runs`、`agent_runs`、`model_calls`、`background_jobs` 增加只读聚合 API，支持按 tenant、时间范围、provider、model、workflow/task 过滤，返回 token/字符成本、调用次数、成功率、失败 stage、fallback 次数、限流/熔断/预算耗尽次数、平均耗时；前端 `frontend/src/views/OpsWorkbench.vue` 增加以表格、筛选器和摘要卡片为主的成本与稳定性视图；只做普通 SQL 聚合和页面展示，不引入 BI、数据仓库或复杂趋势分析；新增后端测试覆盖聚合口径、空数据、过滤条件与 tenant 隔离；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

### 3. 生成策略 Profile 与脚本报告

**需要角色**：后端工程师，可由前端工程师辅助展示 trace/profile 信息。

**Trigger**：prompt、检索参数、模型 provider 或预算策略开始频繁调整，需要用固定样本比较不同策略对生成数量、成本、失败率和人工反馈的影响，但团队只有前后端工程师，暂不做算法评测平台。

**DoD**：后端引入 generation profile 配置与持久化记录，至少覆盖 provider/model、prompt registry version、document/knowledge topK、多 query 数量、chunk 展示上限、预算/timeout/fallback 策略；每次任务在 workflow metadata / trace 中记录 profile id/version；新增脚本和测试数据目录维护一批固定需求与知识，脚本运行后输出 JSON/Markdown 报告，包含生成数量、去重后数量、成本、失败 stage、命中 source_context 和人工反馈统计；不实现自动覆盖率评分、rerank 训练或 LLM-as-judge；新增测试覆盖 profile 解析、默认值、trace 写入；`cd backend && go test ./...`、相关脚本、`cd frontend && npm run build` 通过。

### 4. 用例审核体验升级

**需要角色**：前端工程师、后端工程师。

**Trigger**：试用用户开始一次性审核几十条以上用例，单条 JSON 编辑、逐段提交和缺少筛选导致审核成本明显高于生成成本。

**DoD**：前端 `frontend/src/views/TaskDetail.vue` 支持按 section、优先级、影响产品/模块、反馈状态、生成依据筛选用例；支持批量提交、批量修改优先级/影响范围、手工标记重复用例并隐藏；编辑体验从整段 JSON 扩展为结构化行内编辑或侧边编辑器，并保留 JSON 高级编辑入口；后端补齐批量更新/提交 API 与测试，确保 tenant 隔离和状态流转正确；不实现自动语义重复检测；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

### 5. Demo 初始化与重置脚本增强

**需要角色**：后端工程师。

**Trigger**：演示或试用频率提高后，手动准备 tenant、项目、文档、知识库、任务状态变得容易出错，或每次演示前需要快速恢复到稳定样例。

**DoD**：在 `scripts/` 中增强现有 demo bootstrap/reset 脚本，支持创建/复用指定 tenant、导入固定项目/文档/知识库、触发分析或生成任务、清理 demo 任务与测试数据；脚本输出明确的 API URL、task id、tenant slug 和失败原因；新增 README 或脚本文档说明使用方式；不新增复杂控制台页面；相关脚本能在本地 demo 配置下运行，`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

## 前后端能做但当前不启动

这些事项不需要算法工程师或数据工程师，但不是当前最短路径。除非出现明确 Trigger，否则只保留为备选，不主动开工。

- **权限、操作者与审计**：前后端可做。当前仍按可信本地 demo / 小范围试用处理；只有出现外部用户、多人并发编辑、危险操作追责或客户侧安全要求时，再考虑登录、RBAC、操作者审计和危险操作二次确认。
- **API 契约与轻量 e2e**：前后端可做。OpenAPI、生成式 API client、少量 Playwright 主流程验证都可以由现有团队维护；但在 API 调整频率高、外部调用方少的阶段，先用后端测试和前端 build 控制风险。
- **Demo 控制台**：前后端可做。现阶段脚本更便宜、可复现性更强；只有非工程同学频繁准备演示数据，或脚本使用成本明显阻碍试用时，再做可视化一键 reset 页面。
- **知识库治理的规则化功能**：前后端可做。重复知识的人工标记、来源筛选、过期时间字段、chunk 来源高亮、知识变更影响任务的普通 SQL 查询都可以做；但当前优先级低于用例反馈、审核体验和 demo 稳定性。
- **质量评估产品化页面**：前后端可做。prompt/profile 对比页、质量报告历史页、人工反馈汇总页可以从脚本报告演进而来；在固定样本报告尚未稳定前，不先做产品页。

## 暂不纳入计划

- 自动 OCR / 图片识别：README 已声明“输入文档中的图片内容已在正文以文字描述覆盖”。除非这个约定被破坏，否则不引入。
- 自动质量评分与算法优化：没有算法工程师前，不做自动覆盖率评分、LLM-as-judge、reranker、语义重复检测模型、query rewrite 模型、embedding 训练或模型效果实验平台。
- 数据平台与复杂分析：没有数据工程师前，不做数据仓库、BI 平台、特征工程 pipeline、复杂趋势预测、跨版本自动归因或大规模离线实验平台。
- 专职测试平台：没有专职测试工程师前，不做完整自动化回归平台、跨浏览器矩阵或大规模端到端用例平台；前后端工程师只维护与当前功能直接相关的测试。
