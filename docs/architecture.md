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
- DeepAgent（协调）：`backend/internal/agent/deep/`
- 子 Agent：`backend/internal/agent/{functional,ops,failure,boundary}/`
- 任务服务（解析、去重、source_context 拼装）：`backend/internal/service/task/service.go`
  - 解析：`parseGeneratedSections*`
  - 去重：`dedupeGeneratedSections`
  - 上下文追溯：`attachCaseContext` / `buildSourceContext`
- 数据库表：`backend/migrations/001_init.sql`（`test_cases.source_context JSONB`）

**单元测试**：`backend/internal/service/task/service_test.go`
- `TestParseGeneratedSectionsSectionedJSON` / `TestParseGeneratedSectionsFlatJSON`
- `TestDedupeGeneratedSections`
- `TestAttachCaseContext` / `TestBuildSourceContext`

**回归脚本**：

| 脚本 | 用途 |
| --- | --- |
| `scripts/i2_generation_e2e.sh` | 选最近一个 public-corpus 项目 → analyze → review → generate → 校验落库；含三项硬断言（`duplicate_title_count==0` / `cases_missing_affected_fields==0` / `sections_with_source_context==section_count`）|

**回归证据**：

- `docs/regression/i2_retrieval_context.md` —— parent retriever 上下文样例、DeepAgent 失败重试样例、生成质量样例
- `docs/regression/i2_e2e_generation.md` —— 端到端流程（选需求 → 分析 → 审核 → 生成 → 入库 → 查询 → 修改/提交），含 cURL 与 SQL

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
  - `ProjectList.vue` / `ProjectDetail.vue` / `KnowledgeBase.vue` / `TaskDetail.vue` / `KnowledgeSuggestions.vue`

**可观测性**：

- 前端：`api/client.js` 把 5xx/408/429/网络错误标记 `retryable=true`，`utils/error.js` 据此分别用 `warning` / `error` 弹给用户；处理失败行有「重新处理」按钮。
- 后端：`backend/internal/api/handler/{document,knowledge,task,testcase}.go` 在主要请求上输出 `[handler]` 前缀日志，含 `document_id` / `knowledge_id` / `task_id` / `case_id`。

**验证方式**：手工跑全流程；`cd frontend && npm run build` 通过。

---

## 四、知识库建议沉淀

**做什么**：分析阶段自动从需求里挖未覆盖的"产品/模块"候选，写入 `knowledge_update_suggestion_groups`，每次出现记录在 `knowledge_update_suggestion_occurrences`；用户也可以在用例审核页手动反馈知识缺失。前端列表按跨任务覆盖数与总频次排序，支持采纳（生成知识条目草稿并跳到知识库页，保存后回填 `resolved_knowledge_id`）/ 忽略；长期未处理的 pending 建议会自动过期为 `dismissed_reason='auto_expired'`。

**关键入口**：

- 表定义：`backend/migrations/002_suggestion_groups.sql`（表 `knowledge_update_suggestion_groups` / `knowledge_update_suggestion_occurrences`，含 `source_case_id` / `resolved_knowledge_id` / `dismissed_reason`）
- 模型：`backend/internal/db/models/knowledge_update_suggestion{,_group}.go`
- 候选提取与聚合：`backend/internal/service/suggestion/{extractor,service}.go`
- 知识草稿生成：`backend/internal/agent/knowledge/`、`backend/internal/service/suggestion/draft.go`
- 生命周期清理：`backend/internal/service/suggestion/cleanup.go`（启动时一次 + 每天一次；阈值见 `backend/configs/config-example.yaml` 的 `suggestion.auto_dismiss_pending_days`）
- API handler：`backend/internal/api/handler/knowledge_suggestion.go`（`GET/POST/PUT /knowledge-suggestions`、`POST /knowledge-suggestions/:id/draft`）
- 异步触发：`backend/internal/service/task/service.go` 的 `AnalyzeTask` 末尾 goroutine
- 前端：`frontend/src/views/TaskDetail.vue`（手动反馈）、`frontend/src/views/KnowledgeSuggestions.vue`、`frontend/src/views/KnowledgeBase.vue`（接收 `?type=&name=&from_suggestion_id=` 或兼容 `?create_type=&create_name=` 预填）

**当前能力边界**（写在这里防止误以为是 bug）：

- 候选识别覆盖英文标识符（kebab-case / snake_case 复合 token、2–6 字符全大写缩写、CamelCase 至少两段）和常见中文后缀实体（如 `X模块` / `Y服务` / `Z组件`）。
- 触发只在 analyze 阶段；生成阶段失败信号目前不入 suggestion，见 [`docs/future_work.md`](future_work.md) 的 `P3：失败信号驱动的 suggestion`。
- 采纳时的草稿不会自动落库，仍需人工校对后保存。

**单元测试 / 集成测试**：`backend/internal/agent/knowledge/draft_test.go`、`backend/internal/service/suggestion/{extractor,draft}_test.go`、`backend/internal/service/suggestion/service_integration_test.go`

---

## 路由速查

完整 API 列表见 README；路由定义在 `backend/internal/api/router/router.go`。

## 常用复跑命令

```bash
# 后端 + 数据库 + pgvector 起好后
bash scripts/i1_retrieval_smoke.sh           # 数据闭环冒烟
bash scripts/i1_retrieval_determinism.sh     # 检索稳定性
bash scripts/i2_generation_e2e.sh            # 生成闭环 e2e

# 后端单测
cd backend && go test ./...

# 前端构建
cd frontend && npm run build
```

## 参考文件

- 规格说明：`docs/spec.md`
- 配置示例：`backend/configs/config-example.yaml`
- 数据库迁移：`backend/migrations/001_init.sql`
- 路由定义：`backend/internal/api/router/router.go`
- 本地联调脚本：`dev.sh`
- 回归脚本说明：`scripts/README.md`
- 回归证据：`docs/regression/`
- 后续优化方向：`docs/future_work.md`
