# I2 生成闭环端到端样例（I2-T4）

支撑生成闭环端到端验证：从「选定需求 → 分析影响范围 → 审核影响范围 → 生成 → 入库 → 查询 → 修改/提交」全流程，落地一份可对照执行的样例文档。
本文与 `i2_retrieval_context.md` 互补：

- `i2_retrieval_context.md` 关注检索/上下文构造与生成质量的**断言式回归**（I2-T1/T2/T3）。
- 本文关注**端到端 API 路径**与**人工 / 自动化复跑可参考的请求/响应样例**（I2-T4）。

## 前置环境

- 后端服务监听于 `CASEAGENT_BASE_URL`（默认 `http://localhost:40003/api/v1`），已应用 `backend/migrations/001_init.sql`（含 `source_context JSONB` 列）。
- PostgreSQL 启用 `pgvector` 扩展；`model.chat` 与 `model.embedding` 在 `backend/configs/config.yaml` 中可用。
- 本地命令：`curl`、`jq`、`psql`（数据库验证用）；`CASEAGENT_PSQL_DSN` 指向同一库。
- 已通过 `scripts/i1_public_corpus_eval.sh` 或私有语料把至少 1 个 `documents.status = 'completed'` 与若干 `knowledge_base.status = 'completed'` 写入库。

以下 cURL 示例默认先设置租户上下文：

```bash
export BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003/api/v1}"
export TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-i1-smoke}"
TENANT_HEADER=(-H "X-Tenant-ID: $TENANT_SLUG")
```

## 主流程涉及的 API

| 步骤 | 方法 | 路径 | Handler |
| --- | --- | --- | --- |
| 创建项目 | `POST` | `/projects` | `handler.CreateProject` |
| 上传需求文档 | `POST` | `/projects/:id/documents`（multipart） | `handler.UploadDocument` |
| 查询文档状态 | `GET` | `/documents/:id` | `handler.GetDocument` |
| （可选）创建知识库 | `POST` | `/knowledge` | `handler.UploadKnowledge` |
| 创建生成任务（触发 analyze） | `POST` | `/projects/:id/tasks` | `handler.CreateGenerationTask` |
| 查询任务状态 | `GET` | `/tasks/:id` | `handler.GetTask` |
| 审核影响范围 | `PUT` | `/tasks/:id/review` | `handler.ReviewAffected` |
| 触发生成 | `PUT` | `/tasks/:id/generate` | `handler.GenerateCases` |
| 列出生成用例 | `GET` | `/tasks/:id/cases` | `handler.ListTestCases` |
| 修改用例 | `PUT` | `/tasks/:id/cases/:case_id` | `handler.UpdateTestCase` |
| 提交用例 | `PUT` | `/tasks/:id/cases/:case_id/submit` | `handler.SubmitTestCase` |

状态机参考：

- `documents.status`：`processing` → `completed` / `failed`。
- `knowledge_base.status`：`processing` → `completed` / `failed`。
- `case_generation_tasks.status`：`analyzing` → `awaiting_review` →（`PUT /review` 后）`ready_to_generate` →（`PUT /generate` 后）`generating` → `completed` / `failed`。
- `test_cases.status`：`draft` →（`PUT /submit` 后）`submitted` →（人工 / 维护逻辑）`approved`。

## 步骤 1：创建项目

```bash
curl -fsS -X POST "$BASE_URL/projects" \
    "${TENANT_HEADER[@]}" \
    -H 'Content-Type: application/json' \
    -d '{"name":"I2-T4 demo","description":"end-to-end walkthrough"}'
```

关键响应字段：

```json
{
  "id": 42,
  "name": "I2-T4 demo",
  "description": "end-to-end walkthrough",
  "created_at": "2026-05-12T09:00:00Z",
  "updated_at": "2026-05-12T09:00:00Z"
}
```

`PROJECT_ID` 取 `id`。

## 步骤 2：上传需求文档（markdown）

```bash
curl -fsS -X POST "$BASE_URL/projects/$PROJECT_ID/documents" \
    "${TENANT_HEADER[@]}" \
    -F 'name=需求-导出账单V2.md' \
    -F 'type=markdown' \
    -F 'source=upload' \
    -F 'file=@./fixtures/需求-导出账单V2.md'
```

关键响应：`id`（`DOCUMENT_ID`）、`status: "processing"`。处理为异步：

```bash
until [ "$(curl -fsS "${TENANT_HEADER[@]}" "$BASE_URL/documents/$DOCUMENT_ID" | jq -r .status)" = "completed" ]; do
    sleep 2
done
```

最终 `documents.status = "completed"` 表示分块、清洗、embedding、入库已完成。

数据库验证（chunk + embedding 都已落库）：

```sql
SELECT
    (SELECT count(*) FROM document_chunks WHERE document_id = :doc_id) AS chunk_count,
    (SELECT count(*) FROM document_chunks WHERE document_id = :doc_id AND embedding IS NOT NULL) AS chunk_with_embedding;
```

`chunk_count > 0` 且 `chunk_with_embedding = chunk_count`。

## 步骤 3（可选）：维护知识库

`product` / `module` 类知识用于让 analyze 阶段更精准地推断「受影响范围」。

```bash
curl -fsS -X POST "$BASE_URL/knowledge" \
    "${TENANT_HEADER[@]}" \
    -H 'Content-Type: application/json' \
    -d '{
        "type":"product",
        "name":"Billing-Core",
        "content":"# Billing-Core\n\n## 概述\n负责账单生成、汇总、导出。\n\n## 相关服务\n- billing-api ...",
        "metadata":{"aliases":["账单核心","billing"]}
    }'
```

响应 `id` 即 `KNOWLEDGE_ID`，初始 `status = "processing"`。轮询直到 `status = "completed"`。Metadata 中的 `aliases` 会参与 analyze 阶段的字符匹配（`inferAffectedKnowledge`），命中后将该条目纳入 `affected_products` / `affected_modules` 候选。

## 步骤 4：创建生成任务（触发 analyze）

```bash
curl -fsS -X POST "$BASE_URL/projects/$PROJECT_ID/tasks" \
    "${TENANT_HEADER[@]}" \
    -H 'Content-Type: application/json' \
    -d '{"document_ids":['"$DOCUMENT_ID"']}'
```

请求约束（见 `validateTaskDocuments`）：

- `document_ids` 必须属于当前项目且全部 `documents.status = "completed"`，否则 400。
- 同一 ID 重复传入会被 `dedupeInts` 去重。

响应即任务对象，初始 `status = "analyzing"`：

```json
{
  "id": 71,
  "project_id": 42,
  "document_ids": [...],
  "affected_products": null,
  "affected_modules": null,
  "status": "analyzing"
}
```

`TASK_ID` 取 `id`。Analyze 在 `taskservice.AnalyzeTask` 中异步执行：

1. 加载该任务的所有 document chunks 拼成全文 `requirements`。
2. 第一轮 `inferAffectedKnowledge`：基于已有知识库条目 `name + metadata.aliases/alias/keywords` 做规范化字符串匹配。
3. 若第一轮未命中，回退到 `inferAffectedKnowledgeWithRetrieval`：用 `buildKnowledgeQueries` 生成多 query，调 `retrievalservice.SearchKnowledgeMultiQuery` top-6，按 type 聚合到 products / modules。
4. 推断结果写回 `case_generation_tasks.affected_products / affected_modules`，状态转 `awaiting_review`。失败时状态置 `failed`，错误以 stdout 日志形式留痕（`Failed to analyze task <id>: ...`）。

轮询：

```bash
until [ "$(curl -fsS "${TENANT_HEADER[@]}" "$BASE_URL/tasks/$TASK_ID" | jq -r .status)" = "awaiting_review" ]; do
    sleep 2
done
curl -fsS "${TENANT_HEADER[@]}" "$BASE_URL/tasks/$TASK_ID" | jq '.affected_products, .affected_modules'
```

## 步骤 5：审核影响范围

```bash
curl -fsS -X PUT "$BASE_URL/tasks/$TASK_ID/review" \
    "${TENANT_HEADER[@]}" \
    -H 'Content-Type: application/json' \
    -d '{
        "affected_products":["Billing-Core"],
        "affected_modules":["Export-PDF"]
    }'
```

服务端约束（见 `canReviewAffected`）：仅 `awaiting_review` 与 `ready_to_generate` 可调用；其余返回 409。这意味着已经审核过的任务可以**重复修订**直到点击 `generate`。两个数组允许为空（即不限定影响范围，全量知识库参与），但通常仍由前端 / 调用方填入。响应：状态转 `ready_to_generate`。

## 步骤 6：触发生成

```bash
curl -fsS -X PUT "$BASE_URL/tasks/$TASK_ID/generate" \
    "${TENANT_HEADER[@]}" \
    -d '{}'
```

服务端用乐观锁：`UPDATE case_generation_tasks SET status = 'generating' WHERE id = ? AND status = 'ready_to_generate'`，`affected_rows = 0` 时返回 409 `task status has changed, please retry`，避免并发重复触发。

返回后异步执行 `taskservice.GenerateCases`：

1. 重新加载需求全文 + 命中知识 + 检索 hits（`retrieveKnowledgeFallback`）。
2. `buildRequirementsContext` 把命中的需求文档片段以「父文档 ID / 命中 query / 片段 score / 片段 rank」结构化方式写进 prompt 上下文。
3. `agentservice.GenerateCases` 顺序执行功能 / 运维 / 故障 / 边界四个子 Agent，每个失败重试 1 次（`runSubAgentWithRetry`）；任一子 Agent 最终失败不阻塞其余结果。
4. 子 Agent 草稿被 `agentservice` 去重后交给 DeepAgent `RefineCases`；若 refine 输出无法 parse，回退到未 refine 的 dedup 草稿（保证落库继续）。
5. 全部子 Agent 都失败/空输出时，由 `agentservice` 触发 DeepAgent fallback `GenerateCases` 重新生成。
6. `parseGeneratedSections` → `dedupeGeneratedSections` → `attachCaseContext`：归一化为 `[]generatedSection`，跨 section 去重，并为每条 case 注入 `affected_products / affected_modules / section`。
7. `buildSourceContext` 把检索 queries / hits 收集成 JSON 写入 `test_cases.source_context`。
8. `persistGeneratedCases` 在事务中清空旧 `test_cases`（同 task_id）后批量插入新行。最终任务转 `completed`；任一阶段失败转 `failed`。

轮询：

```bash
until [[ "$(curl -fsS "${TENANT_HEADER[@]}" "$BASE_URL/tasks/$TASK_ID" | jq -r .status)" =~ ^(completed|failed)$ ]]; do
    sleep 2
done
```

## 步骤 7：查询生成结果

```bash
curl -fsS "${TENANT_HEADER[@]}" "$BASE_URL/tasks/$TASK_ID/cases" \
    | jq '[.[] | {id, section, status, case_count: (.cases|length),
                  source_doc_ids: (.source_context.document_hits | map(.document_id)),
                  source_kb_ids: .source_context.knowledge_shipped_ids,
                  first_case_title: .cases[0].title}]'
```

每行对应一个 section。关键字段：

- `id`：`test_cases.id`，后续修改/提交用。
- `section`：section 名（功能 / 运维 / 故障 / 边界，或 LLM 自定义）。
- `cases`：JSONB 数组（**已不再是双重 JSON string**），每条至少含 `title` / `priority_id` / `custom_preconds` / `custom_steps_separated`，并由 `attachCaseContext` 注入 `affected_products` / `affected_modules` / `section`。
- `source_context.document_hits[]`：含 `document_id` / `parent_doc_id` / `name` / `rank` / `best_score` / `top_chunks[<=3]`（每片段含 `text` / `score` / `query` / `rank`），来源片段可追溯。
- `source_context.knowledge_shipped_ids`：实际拼到 prompt 的知识库 ID 列表（检索命中 ∪ 影响范围匹配）。

数据库直查（确认 cases 是真 JSONB 数组）：

```sql
SELECT id, section, status,
    jsonb_typeof(cases) AS cases_kind,
    jsonb_array_length(cases) AS case_count,
    source_context ? 'document_queries' AS has_doc_queries
FROM test_cases
WHERE task_id = :task_id
ORDER BY id;
```

`cases_kind = 'array'` 且 `has_doc_queries = true` 即满足 I2-T3 契约。

## 步骤 8：修改用例

`UpdateTestCase` 接受 `section` 与 `cases`（数组，按 section 整体替换）：

```bash
curl -fsS -X PUT "$BASE_URL/tasks/$TASK_ID/cases/$CASE_ID" \
    "${TENANT_HEADER[@]}" \
    -H 'Content-Type: application/json' \
    -d '{
        "section":"功能测试",
        "cases":[
            {
                "title":"[导出] 校验生成账单 PDF 文件不为空",
                "priority_id":3,
                "custom_preconds":"账单已生成",
                "custom_steps_separated":[
                    {"content":"调用导出接口","expected":"返回 200 且 file_size > 0"}
                ],
                "affected_products":["Billing-Core"],
                "affected_modules":["Export-PDF"],
                "section":"功能测试"
            }
        ]
    }'
```

注意：

- `cases` 是数组类型（与之前「单条 JSON string」写法不同），保留与生成结果同形。
- 若仅修改某一 case，仍需把整段 section 的所有 case 一并回传，否则会被整体替换。
- 当前 handler 不会再次校验 case 结构，但建议保留 `title / priority_id / custom_preconds / custom_steps_separated` 四个字段，便于消费方稳定渲染。

## 步骤 9：提交用例

```bash
curl -fsS -X PUT "$BASE_URL/tasks/$TASK_ID/cases/$CASE_ID/submit" \
    "${TENANT_HEADER[@]}" \
    -d '{}'
```

服务端把 `test_cases.status` 从 `draft` 改成 `submitted`，`updated_at` 刷新；响应整行。`approved` 状态保留给后续维护/审核策略使用，本流程不强制推进。

## 失败重试方式

| 失败点 | 表现 | 处置 |
| --- | --- | --- |
| 文档上传后处理失败 | `documents.status = failed` | `POST /documents/:id/reprocess` 重新清洗 / 分块 / embedding |
| 知识库处理失败 | `knowledge_base.status = failed` | `POST /knowledge/:id/reprocess` 重新 embedding |
| Analyze 阶段失败 | `case_generation_tasks.status = failed`，无 `affected_products / affected_modules` | 当前无独立"重 analyze"接口；通常重新建一个任务（同 `document_ids`），新 task_id 走整个流程；或先修复底层原因（如缺失知识库 / 模型 key），再重建 |
| 子 Agent 单次失败 | 不写库，`runSubAgentWithRetry` 内部自动**重试 1 次** | 不需要外部介入 |
| 单个子 Agent 重试后仍失败 | 该子 Agent 无产出，但其它子 Agent 结果仍会进入后续 dedup / refine 流程 | 不阻塞，继续执行；日志关键字 `[agent-service]` |
| 全部子 Agent 都失败 | `agentservice` 触发 DeepAgent fallback `GenerateCases` 重新生成全部 sections | 自动 |
| DeepAgent.RefineCases 解析失败 | 回退到未 refine 的 dedup 草稿继续落库（不让格式抖动卡死任务） | 自动 |
| Generate 阶段总体失败（如 chat 模型 401、DeepAgent fallback 也失败） | `case_generation_tasks.status = failed`，事务回滚，原 `test_cases` 行（若存在）不动 | 修复模型 key / 限流后，把任务状态回退到 `ready_to_generate` 再调 `PUT /tasks/:id/generate`；或新建任务重跑 |
| 并发重复触发 generate | 第二次请求收到 409 `task status has changed, please retry` | 不需要外部介入 |
| `PUT /review` 在错误状态调用 | 409 `task status %q does not allow affected-scope review` | 重读 `GET /tasks/:id`，确认状态后再调 |

把已 `failed` 的任务恢复到 `ready_to_generate` 的最小 SQL（仅在排查 generate 阶段时使用，确保根因已修复）：

```sql
UPDATE case_generation_tasks
SET status = 'ready_to_generate', updated_at = now()
WHERE id = :task_id AND status = 'failed';
```

随后再次调用 `PUT /tasks/$TASK_ID/generate` 即可走一遍 retry-once + DeepAgent fallback 完整链路。

## 自动化脚本与本文档的关系

- `scripts/i2_generation_e2e.sh` 已串起步骤 4–7 并对 I2-T3 三项断言做硬校验。
- 本文档补全步骤 1–3、步骤 8–9 与失败恢复路径，作为前端 / 调用方对照实现 + 人工冒烟的参考。

## 最近一次实际结果摘要

最近一轮：

- 步骤 4–7：由 `CASEAGENT_PSQL_DSN='postgres://...' CASEAGENT_I2_E2E_PROJECT_ID=54 bash scripts/i2_generation_e2e.sh` 承担，task_id=46，终态 `completed`，4 个 section / 32 cases；脚本内含 I2-T3 三项断言（无重复标题 / 每条 case 都有 `affected_products`+`affected_modules` / 每行 `test_cases` 都有 `source_context`）。
- 本轮基于 I1 public corpus run_token=`i1-public-20260531134530-8970`，project_id=54，document_id=13 (`dubbo-zipkin-integration.md`)。
- 断言结果：`duplicate_title_count=0`，`cases_missing_affected_fields=0`，`sections_with_source_context=4/4`，`source_context[0]` 摘要为 `document_queries=9 knowledge_queries=9 document_hits=1 knowledge_hits=6 knowledge_shipped_ids=8`。
- 步骤 1–3 与 步骤 8–9：本文 cURL 样例可直接对照执行；首次走完整流程后，把请求/响应/`UPDATE` 后的 `jsonb_typeof(cases)` 校验结果回填到 `.dev/i2_e2e_generation.md`（gitignored，重跑覆盖）。
- DB 直查校验由 `scripts/i2_generation_e2e.sh` 退出码隐含覆盖；task_id=46 的所有 4 行均满足 JSONB 数组形态与 source_context 字段要求。
