# CaseAgent 架构与回归索引

本文档面向新加入的开发者 / AI 协作者，解决两个问题：

1. **代码长在哪？** —— 按四大功能闭环列出关键入口。
2. **怎么验证还活着？** —— 每块功能配套的回归脚本和证据文档。

> 项目终态在 README 里有一句话概括；本文是按"功能 -> 入口 -> 验证"展开的导航。
> 历史实施过程见 git log；不在本文回头重述。

---

## 项目终态

围绕"需求文档 + 架构知识"形成可审核测试用例闭环：

1. 导入需求文档与架构知识。
2. 完成文档清洗、分块、向量化、检索。
3. 识别受影响产品和模块。
4. 基于需求与知识上下文生成测试用例。
5. 支持测试用例审核、修改、提交。
6. 必要时沉淀知识库更新建议。

## 固定技术决策

- 前端：Vue 3 + Vite + Element Plus + Pinia。
- 后端：Golang + Gin + Bun ORM + eino + eino-ext。
- 数据库：PostgreSQL + pgvector。
- 模型配置统一走配置文件，并支持环境变量覆盖。
- chat 当前支持 Ark / DeepSeek / OpenAI-compatible；embedding 当前支持 Ark / OpenAI-compatible。
- embedding 维度通过 `model.embedding.dimensions` 显式配置。
- 当前工程对 `Qwen3-Embedding-8B` 使用 `2000` 维输出；向量索引使用 `pgvector hnsw`（多租户改造后切换，对带 filter 的 ANN 更友好）。
- Google Drive 集成通过本地 `gws drive files export` 命令调用，无 Docker 依赖。
- 多租户隔离：PostgreSQL Row-Level Security + 事务内 `SET LOCAL app.tenant_id`。所有业务表强制 `tenant_id NOT NULL`，policy 统一 `tenant_id = current_setting('app.tenant_id')::int`；HTTP 入口通过 `X-Tenant-ID` header（tenant slug）解析。详见 [`docs/multitenancy.md`](multitenancy.md)。

---

## 一、数据闭环（文档/知识库 → 向量化 → 检索）

**做什么**：把需求文档和架构知识洗成纯文本、分块、生成 embedding 落 pgvector，提供向量检索接口。

**关键入口**：

- 文档处理服务：`backend/internal/service/document/`
- 知识库服务：`backend/internal/service/knowledge/`
- Markdown 清洗（含 base64 图片剥离）：`backend/internal/markdown/`
- 检索服务：`backend/internal/service/retrieval/`
- 向量存储：`backend/internal/db/vector/`、`backend/internal/db/pgvector/`
- 向量健康维护：`backend/internal/service/maintenance/`
  - `document_chunks.index_profile/index_version` 与 `knowledge_base.index_profile/index_version` 记录当前向量由哪套 embedding + chunk/profile 生成。
  - `GET /api/v1/maintenance/vector-health` 返回缺失 / 维度不匹配 / stale index 明细，`GET /api/v1/maintenance/stale-index` 返回可重建计划。
  - `POST /api/v1/maintenance/reindex` 只在当前 tenant 下把 stale/缺失对象排入 `background_jobs`，job payload 写入目标 profile/version，worker 会为重建 job 写 `workflow_runs` / `workflow_steps`。

**回归脚本**（详细用法见 [`scripts/README.md`](../scripts/README.md)）：

| 脚本 | 用途 |
| --- | --- |
| `scripts/i1_retrieval_smoke.sh` | 上传 → 处理完成 → 检索 top1 必中本轮对象，连跑 3 次 |
| `scripts/i1_long_knowledge_eval.sh` | 长知识库分块/聚合后多查询命中验证 |
| `scripts/i1_retrieval_determinism.sh` | 同 fixture/查询连跑 3 次，命中集合 + 排序一致 |
| `scripts/i1_private_corpus_eval.sh` | 通过环境变量喂私有真实语料，记录文件/字节/chunk/embedding/命中 |
| `scripts/i1_public_corpus_eval.sh` | 同上，但读 `testdata/i1/public_corpus/{long,short}/` 公开 fixture |
| `scripts/i1_retrieval_cleanup.sh` | 清理历史 smoke knowledge 行，避免排名假阳性 |

**回归证据**：`docs/regression/i1_retrieval.md` —— 文档/知识库样例、长知识库分块对比、私有 + 公开语料运行摘要。

---

## 二、生成闭环（需求 → 检索增强 → Agent → 用例落库）

**做什么**：选定需求 → 分析受影响产品/模块 → 用户审核 → 触发 4 个子 Agent 并行生成 → 去重/结构化 → 落库。

**关键入口**：

- Agent Service（编排）：`backend/internal/service/agent/service.go`
  - ADK/AgentGraph：`backend/internal/service/agent/graph.go` 把 functional / ops / failure / boundary 子 Agent 暴露成统一 `adk.Agent` 节点，Graph 负责节点输入、输出、错误、耗时与失败隔离；Service 根据 graph 输出决定 fallback / refine，并把 trigger reason 写入 workflow trace metadata。
  - DeepAgent fallback coordination：`backend/internal/agent/deep/deep.go` 能消费同一组 ADK sub-agents，先尝试汇总子 Agent 草稿并 refine；无可用草稿时再走 DeepAgent 自身直连生成。
  - 每次 LLM 调用使用 `model.chat.request_timeout_seconds` 做单次超时，并输出 agent start/end/failure 日志，避免真实 provider 慢调用让 task 长期停在 `generating`
- DeepAgent（协调）：`backend/internal/agent/deep/`
- 子 Agent：`backend/internal/agent/{functional,ops,failure,boundary}/`
- Prompt Registry：`backend/internal/agent/prompts/registry.go`
  - prompt 模板按 `ID + version` 注册，Agent 只渲染 registry 默认版本，不在业务代码中内联长模板。
  - `model_calls.metadata.prompt_id` / `prompt_version` 记录本次 LLM 调用实际使用的 prompt。
  - 新增 prompt 版本：在 registry 增加同 ID 新 version，标记 `Default: true` 并取消旧版本 default；回滚则把 default 标记切回旧版本，配套更新 `registry_test.go`。
- 任务服务（应用层生成 workflow）：`backend/internal/service/task/`
  - 顶层编排：`service.go`
  - 任务创建 / review / generate / retry 状态机：`lifecycle.go`
  - 需求与知识上下文：`context.go`
  - 影响范围推断与知识 fallback：`scope.go`
  - LLM 输出解析、归一化、去重、case context 注入：`parser.go`
  - 状态更新与 test_cases 持久化：`store.go`
  - 失败阶段归类与 context_gap 记录：`failure.go`
  - Agent / retrieval / suggestion 默认依赖注入点：`dependencies.go`
- 持久化任务运行器：`backend/internal/service/job/`
  - handler 只写入 `background_jobs`；worker 统一领取并执行 analyze / generate。
  - job 记录 job 类型、tenant、task_id、状态、重试次数、最后错误、领取/完成时间。
  - worker 按 `job_runner.max_concurrency` 限制并发，用 tenant-scoped tx 执行任务，启动时恢复超时 running job。
  - worker 领取 job 后创建 `workflow_runs` / `workflow_steps`，并把 `workflow_run_id` 注入任务执行 context。
  - 运维 API：`GET /api/v1/jobs` / `GET /api/v1/workflows` 支持按当前 tenant、resource、status、job_type/workflow_type 查询；`POST /api/v1/jobs/:id/{retry,cancel,replay}` 要求请求体带 `reason`，只允许安全状态转移，并把可信 operator header（`X-Operator-ID` / `X-Operator-Name`）与 reason 写入 `artifacts.artifact_type='intervention'`。
- Workflow trace：`backend/internal/service/workflow/`
  - Workflow run / step 状态通过 `start/succeed/fail/cancel/replay` 事件做集中转移，非法 transition 在 service 层拒绝。
  - `GenerateCases` 记录 document / knowledge retrieval 摘要、case generation agent 摘要和 generated cases artifact。
  - 后台 job 通过 tenant-scoped 独立事务写 trace，避免生成业务事务回滚时丢失失败现场。
  - `backend/internal/service/agent/` 为每次子 Agent / DeepAgent 调用创建 `agent_runs`，并通过 chat model decorator 记录每次 LLM `Generate` 的 provider/model、prompt/response 字符数、agent/attempt metadata、`agent_run_id` 和错误摘要。
  - API：`GET /api/v1/tasks/:id/trace` 返回 workflow runs、steps、agent runs、model calls、retrieval runs、artifacts。
  - 诊断包：`GET /api/v1/tasks/:id/diagnostics` 返回 task、test_cases、workflow/agent/model/retrieval trace、feedback、错误摘要和 source_context 摘要；默认脱敏 artifact content、敏感 metadata key、长 prompt/metadata 和原始 source_context 查询正文。
  - 生命周期治理：`retention.trace_retention_days` 配置默认保留天数；`GET /api/v1/ops/retention/cleanup` 返回 tenant-scoped dry-run 行数 / 字节估算，`POST /api/v1/ops/retention/cleanup` 清理过期终态诊断明细、脱敏 `test_cases.source_context`，保留 `case_generation_tasks` / `test_cases` 最终状态和 `intervention` 审计 artifact，并新增 retention cleanup intervention 摘要。
  - 前端：`frontend/src/views/TaskDetail.vue` 展示 job timeline 与 Workflow Trace 面板，用于 demo 排查生成链路。
- 数据库表：
  - `backend/migrations/001_init.sql`（`test_cases.source_context JSONB`）
  - `backend/migrations/003_background_jobs.sql`（`background_jobs` + RLS）
  - `backend/migrations/005_v2_workflow_trace.sql`（workflow / agent / model / retrieval / artifact trace + RLS）

**单元测试**：`backend/internal/service/task/service_test.go`
- `TestParseGeneratedSectionsSectionedJSON` / `TestParseGeneratedSectionsFlatJSON`
- `TestDedupeGeneratedSections`
- `TestAttachCaseContext` / `TestBuildSourceContext`
- `backend/internal/service/task/lifecycle_test.go` 覆盖 review / generate 状态允许条件
- `backend/internal/service/job/runner_test.go` 覆盖并发上限、失败重试、启动恢复和 tenant-scoped 执行上下文

**回归脚本**：

| 脚本 | 用途 |
| --- | --- |
| `scripts/demo_bootstrap.sh` | 使用公开 fixture + fake provider 预期配置创建可演示的 tenant/project/document/knowledge/task，并输出前端任务 URL |
| `scripts/i2_generation_e2e.sh` | 选最近一个 public-corpus 项目 → analyze → review → generate → 校验落库；含三项硬断言（`duplicate_title_count==0` / `cases_missing_affected_fields==0` / `sections_with_source_context==section_count`）|
| `scripts/i2_generation_quality_eval.sh` | 包装 e2e 并输出 Markdown + JSON + 静态 HTML 质量报告：case/section、字段完整率、source_context、失败阶段、model_call、字符/token、prompt version 指标 |
| `scripts/trace_retention_cleanup.sh` | 默认 dry-run 诊断数据保留策略；执行时调用 tenant-scoped cleanup API 并写入 operator / reason 审计摘要 |

**回归证据**：

- `docs/regression/i2_retrieval_context.md` —— parent retriever 上下文样例、DeepAgent 失败重试样例、生成质量样例
- `docs/regression/i2_e2e_generation.md` —— 端到端流程（选需求 → 分析 → 审核 → 生成 → 入库 → 查询 → 修改/提交），含 cURL 与 SQL
- `docs/regression/i2_generation_quality_eval.html` —— `scripts/i2_generation_quality_eval.sh` 生成的静态可视化入口（同名 `.md` / `.json` 保留原始指标）

---

## 三、产品化闭环（前端工作台）

**做什么**：不依赖手工 cURL 即可走完"项目 → 文档 → 知识库 → 任务 → 影响范围审核 → 用例生成 → 修改/提交"全流程。

**关键入口**：

- 布局壳：`frontend/src/layout/AppLayout.vue`
- 路由：`frontend/src/router/index.js`
- API client（含 retryable 标记拦截器）：`frontend/src/api/client.js`
- 错误归一化：`frontend/src/utils/error.js`
- 状态展示（直读后端 status，前端不二次推断）：`frontend/src/components/StatusTag.vue`
- 业务页面：`frontend/src/views/`
  - `CaseGenerationWorkspace.vue`：默认首页 `/generate`，承载项目选择、文档上传/选择、任务创建、影响范围确认、生成触发、测试用例查看/导出/提交
  - `ProjectList.vue` / `ProjectDetail.vue`：项目与文档管理，并提供跳转到生成工作台的入口
  - `TaskDetail.vue`：任务深度排查、Workflow Trace、用例 JSON / 单条编辑、提交前结构校验、dirty state、按反馈 / 优先级 / 来源筛选和批量操作
  - `KnowledgeBase.vue` / `KnowledgeSuggestions.vue`：知识库与知识建议沉淀
  - `OpsWorkbench.vue`：tenant-scoped jobs / workflows 运维视图，支持筛选、重试、取消、重放；Preflight tab 展示 DB role、pgvector、RLS、tenant context、模型配置、向量健康和 worker risk，并可复制完整诊断 JSON；Quality tab 用现有 feedback / trace / artifact 数据展示 prompt/profile 对比、反馈趋势和报告历史。

**可观测性**：

- 前端：`api/client.js` 把 5xx/408/429/网络错误标记 `retryable=true`，`utils/error.js` 据此分别用 `warning` / `error` 弹给用户；处理失败行有「重新处理」按钮。API client 还会从 localStorage 注入 `X-Tenant-ID` 与可信 operator header，运维页执行 retry / cancel / replay 前要求填写操作者和原因。
- 后端：`backend/internal/api/handler/{document,knowledge,task,testcase}.go` 在主要请求上输出 `[handler]` 前缀日志，含 `document_id` / `knowledge_id` / `task_id` / `case_id`；`workflow_runs` 及相关 trace 表持久化生成链路的可查询状态；`GET /api/v1/ops/preflight` 覆盖 DB role、pgvector、RLS、tenant context、模型配置、向量健康和 worker risk；`GET /api/v1/ops/quality` 基于人工 feedback、test case source_context 和 generated_cases/output artifact 做质量复盘聚合，不引入自动评分；过期诊断明细通过 retention cleanup API / 脚本按 tenant dry-run 后清理。

**验证方式**：手工跑全流程；`cd frontend && npm run build` 通过。

**运维权限边界**：当前 retry / cancel / replay / reindex / retention cleanup 适合 demo、本地调试和单租户可信操作员场景。前端会提示操作者和原因，后端把可信 operator header 与 reason 写入 intervention artifact；这仍不是登录或 RBAC。生产环境应把 header 换成登录态 / JWT claims，并在这些 API 前增加角色权限与更细的资源级授权；跨 tenant 批量处理仍应走独立 admin API，而不是复用 tenant-scoped 业务入口。

---

## 四、知识库建议沉淀

**做什么**：分析阶段自动从需求里挖未覆盖的"产品/模块"候选，生成阶段失败时沉淀 `context_gap` 候选；两类信号都会写入 `knowledge_update_suggestion_groups`，每次出现记录在 `knowledge_update_suggestion_occurrences`。用户也可以在用例审核页手动反馈知识缺失。前端列表按跨任务覆盖数与总频次排序，支持普通 `product` / `module` 建议采纳（生成知识条目草稿并跳到知识库页，保存后回填 `resolved_knowledge_id`）/ 忽略；`context_gap` 仅展示失败阶段、影响范围和相关知识上下文，暂不自动生成知识草稿。长期未处理的 pending 建议会自动过期为 `dismissed_reason='auto_expired'`。

**关键入口**：

- 表定义：`backend/migrations/002_suggestion_groups.sql`（表 `knowledge_update_suggestion_groups` / `knowledge_update_suggestion_occurrences`，含 `source_case_id` / `resolved_knowledge_id` / `dismissed_reason`）
- 模型：`backend/internal/db/models/knowledge_update_suggestion{,_group}.go`
- 候选提取、失败信号记录与聚合：`backend/internal/service/suggestion/{extractor,service}.go`
- 知识草稿生成：`backend/internal/agent/knowledge/`、`backend/internal/service/suggestion/draft.go`
- 生命周期清理：`backend/internal/service/suggestion/cleanup.go`（启动时一次 + 每天一次；阈值见 `backend/configs/config-example.yaml` 的 `suggestion.auto_dismiss_pending_days`）
- API handler：`backend/internal/api/handler/knowledge_suggestion.go`（`GET/POST/PUT /knowledge-suggestions`、`POST /knowledge-suggestions/:id/draft`）
- 异步触发：`backend/internal/api/handler/task.go` 只提交 `background_jobs`；`backend/internal/service/job/` worker 领取后执行 `AnalyzeTask` / `GenerateCases`。`AnalyzeTask` 末尾同步记录普通 suggestion（best-effort，不阻断 analyze），`GenerateCases` 失败且 job 重试耗尽后另开 tenant tx 标记 task failed 并记录 `context_gap`
- 前端：`frontend/src/views/TaskDetail.vue`（手动反馈）、`frontend/src/views/KnowledgeSuggestions.vue`、`frontend/src/views/KnowledgeBase.vue`（接收 `?type=&name=&from_suggestion_id=` 或兼容 `?create_type=&create_name=` 预填）

**当前能力边界**（写在这里防止误以为是 bug）：

- 候选识别覆盖英文标识符（kebab-case / snake_case 复合 token、2–6 字符全大写缩写、CamelCase 至少两段）和常见中文后缀实体（如 `X模块` / `Y服务` / `Z组件`）。
- `context_gap` 只记录事实型失败现场（失败阶段、错误摘要、影响范围、相关知识 ID/名称），不做自动诊断；API key / 限流等非知识原因也可能作为失败上下文出现，需要人工判断。
- 采纳时的草稿不会自动落库，仍需人工校对后保存。

**单元测试 / 集成测试**：`backend/internal/agent/knowledge/draft_test.go`、`backend/internal/service/suggestion/{extractor,draft}_test.go`、`backend/internal/service/suggestion/service_integration_test.go`

---

## 路由速查

完整 API 列表见 README；路由定义在 `backend/internal/api/router/router.go`。

## 常用复跑命令

```bash
# 后端 + 数据库 + pgvector 起好后
bash scripts/demo_bootstrap.sh              # 稳定演示数据
bash scripts/i1_retrieval_smoke.sh           # 数据闭环冒烟
bash scripts/i1_retrieval_determinism.sh     # 检索稳定性
bash scripts/i2_generation_e2e.sh            # 生成闭环 e2e
bash scripts/i2_generation_quality_eval.sh   # 生成质量门禁报告

# 后端单测
cd backend && go test ./...

# 前端构建
cd frontend && npm run build
```

## 参考文件

- 规格说明：`docs/spec.md`
- 配置示例：`backend/configs/config-example.yaml`
- 数据库迁移：`backend/migrations/*.sql`
- 路由定义：`backend/internal/api/router/router.go`
- 本地联调脚本：`dev.sh`
- 回归脚本说明：`scripts/README.md`
- 回归证据：`docs/regression/`
- 后续优化方向：`docs/future_work.md`
