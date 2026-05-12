# I2 生成闭环回归

记录 I2 各任务的端到端样例与稳定性证据。每个任务一个样例段落。

## 样例 1：检索响应承载追溯字段（I2-T1）

### 字段约定

`POST /api/v1/retrieval/documents` 与 `POST /api/v1/retrieval/knowledge` 的 `items` 现在按以下结构返回（除既有字段外的新增）：

- 文档：`rank`（1-indexed 最终排名）、`best_score`（综合最高的 chunk 余弦相似度）、`hit_queries`（命中本文档的查询列表）；`matched_chunks[]` 由原 `[]string` 改为 `[]MatchedChunk{ text, score, query, rank }`，其中 `rank` 是该 chunk 在所属查询命中池中的位置，`score = 1 - cosine_distance`（pgvector `<=>` 算子）。
- 知识：`rank`、`score`、`hit_queries`。

`buildRequirementsContext`（`backend/internal/service/task/service.go`）在生成前把这些字段渲染成结构化 markdown 块（包含父文档 ID/名称、综合得分、每个命中片段的 score/query/chunk_rank），随后传给 `agent.GenerateCases`，确保 agent 上下文同时承载父文档身份、命中片段文本、检索 query 与片段排序+得分。

### 3 次连续执行稳定性

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/public_corpus/{long,short}/`（与 I1-T7 同语料） |
| 文档查询 | `Dubbo 双注册原理 服务提供者 注册中心` / `模块发布器 服务发布全过程 ServiceConfig` / `Service Weaver 微服务编排 Google 论文` |
| 知识查询 | `Dubbo 流量管理 路由规则` / `Dubbo SPI 扩展点 加载机制` / `Dubbo 回调参数 异步通知` |
| 期望 | 同一查询连续 3 次返回的 `(document_id/id, name, rank, hit_queries, matched_chunks[].rank/query)` 完全一致；`score`/`best_score` 因嵌入 API 服务端浮点抖动可能在第 4 位小数后微变，不计入比较 |
| 执行命令 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_determinism.sh` |

### 最近一次实际结果摘要

最近一轮：本地基于 I1 公开语料 project_id=14（KEP 替代 Dubbo 长文 6 篇 + Dubbo 短文 15 篇）执行通过：

文档查询：

- ✓ `Dubbo 双注册原理 服务提供者 注册中心` → 3/3 identical，rank-1 document_id=29（best_score=0.7484838006761214）
- ✓ `模块发布器 服务发布全过程 ServiceConfig` → 3/3 identical，rank-1 document_id=27（best_score=0.672531419384465）
- ✓ `Service Weaver 微服务编排 Google 论文` → 3/3 identical，rank-1 document_id=28（best_score=0.7321092496600046）

知识查询：

- ✓ `Dubbo 流量管理 路由规则` → 3/3 identical，rank-1 knowledge_id=52（score=0.8254885250331316）
- ✓ `Dubbo SPI 扩展点 加载机制` → 3/3 identical，rank-1 knowledge_id=64（score=0.7547588501439271）
- ✓ `Dubbo 回调参数 异步通知` → 3/3 identical，rank-1 knowledge_id=53（score=0.7562281425023546）

详细本地报告：`.dev/i1_retrieval_determinism.md`（gitignored，重跑覆盖）。

### 关于 score 抖动

实测发现，`Qwen3-Embedding-8B` 通过 Gitee OpenAI-compatible 端点返回的向量在多次相同输入下会有 1e-4 量级的微抖动（疑似服务端 batched FP 累加顺序差异），导致 cosine score 在第 4 位小数后变化。**命中对象与排序不受影响**：本轮 3×6 = 18 次调用中 hit set + ordering 全部一致。

`scripts/i1_retrieval_determinism.sh` 的指纹（fingerprint）函数因此**只比对 hit set + ordering**，把浮点 score 排除出确定性判定。

## 样例 2：DeepAgent / Agent Service 协调 + 端到端落库（I2-T2）

### 职责边界（已落档于代码）

`backend/internal/service/agent/service.go` 顶部 package doc 详述：

- **Agent Service**（应用层协调器）：拥有四个子 Agent 的实例（functional / ops / failure / boundary），按顺序调用，每个子 Agent 失败重试 1 次，失败后**记录日志并继续**（不阻塞其他子 Agent），最后把 dedupe 后的草稿交给 DeepAgent 做 refine。本层不直接调用 LLM。
- **DeepAgent**（agent 层协调器，`backend/internal/agent/deep`）：直接持有 chat model，承担两个角色 —— (a) **fallback**：当所有子 Agent 都失败/无可解析输出时，由 DeepAgent 重新生成全部 sections；(b) **refine**：把 dedupe 后的子 Agent 草稿合并精炼。

子 Agent 暂未挂入 DeepAgent 的 `adk.Agent` 槽位（保持顺序协调而非图协调），后续可平滑迁移到 eino 原生 `adk.Agent` 协调。

### Retry-once + 非阻塞失败行为

`runSubAgentWithRetry` 单测覆盖三种状态：

- 第一次成功 → 仅 1 次调用
- 第一次失败、第二次成功 → 恰好 2 次调用
- 持续失败 → 恰好 2 次调用后向上抛错

实战验证（task 2，临时 401 场景）：日志连续显示 4 个子 Agent 各重试 1 次（共 8 次 401），随后输出 `all 4 sub-agents produced no usable output; falling back to DeepAgent.GenerateCases`，DeepAgent 也 401 后 task 进入 `failed` 状态。证明任一子 Agent 失败不阻塞其他子 Agent；失败汇总后正确触发 fallback。

`RefineCases` 输出加了 parse 校验：解析失败时退回未 refine 的 dedup 草稿（保证落库继续，不被 LLM 文本格式抖动卡死）。

### 端到端跑通

| 项 | 内容 |
| --- | --- |
| Fixture | I1-T7 公开语料 project_id=14 中的 document_id=30（`dubbo-zipkin-integration.md`） |
| 影响范围 | 由分析阶段自动推断，6 个短篇 Dubbo 知识（concepts/references 类） |
| 执行命令 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i2_generation_e2e.sh` |
| 期望 | task 终态 `completed`；`/tasks/:id/cases` 至少 1 条用例落库 |

### 最近一次实际结果摘要

最近一轮：本地执行 `scripts/i2_generation_e2e.sh` 通过。

- task_id=3，终态 `completed`，4 个 section（功能 / 运维 / 故障 / 边界）。
- 实际落库 case 数：37（功能 8 + 运维 8 + 故障 9 + 边界 12）。
- 前 5 条 case title：
  - `[链路生成] 正常Dubbo调用链Trace生成验证`
  - `[Span数据] 调用边界四个标注(cs/sr/ss/cr)正确性验证`
  - `[日志透传] SLF4J MDC中traceId、spanId正确性验证`
  - `[链路隔离] 多次并发调用TraceId互不干扰验证`
  - `[异常场景] 服务调用抛出异常时链路数据正常上报验证`
- 详细本地报告：`.dev/i2_generation_e2e.md`（gitignored）。

### 已知遗留（交由后续任务处理）

- ~~`case_generation_results.cases` 字段在数据库中为**双重 JSON 字符串**（外层 string 包内层 escaped JSON 数组），脚本目前需要 `fromjson | fromjson` 才能取到 case 对象。这是 persist 路径的一个序列化重叠 bug，归入 **I2-T3（生成质量控制）** 一并修复。~~ 已于 I2-T3 修复，见样例 3。

## 样例 3：生成结果结构 / 去重 / 追溯字段（I2-T3）

### 输出契约对齐 `docs/spec.md`

`backend/internal/service/task/service.go`：

- `parseGeneratedSections` / `groupFlatCases` / `normalizeSections` 把 LLM 返回归一化为 `[]generatedSection{ Section, Cases }`。
- `normalizeCase`（line 832 起）保证每条 case 至少包含 `title`、`priority_id`（默认 3）、`custom_preconds`、`custom_steps_separated`（每步含 `content` + `expected`）四个字段，与 `docs/spec.md` 的 JSON 契约一致。
- `attachCaseContext`（line 762 起）补齐每条 case 的 `affected_products`、`affected_modules`、`section`，保证落库后每条用例都能反查到所属影响范围与 section。

### 数据库存储约束

- `backend/internal/db/models/test_case.go`：`Cases` 字段已从 `string`（双重 JSON）改为 `[]map[string]any` + `type:jsonb`，新增 `SourceContext map[string]any` + `type:jsonb`。
- `backend/migrations/001_init.sql`：新增 `source_context JSONB` 列，并提供 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS source_context JSONB` 做幂等回填，兼容已存在的本地 schema。
- `persistGeneratedCases`（`service.go` line 260 起）直接以结构化值落库，不再 `json.Marshal(...)` 成 string，彻底消除「双重 JSON 字符串」bug。

### 去重

`dedupeGeneratedSections`（`service.go` line 719 起）按 `(title, custom_preconds, custom_steps_separated[].(content,expected))` 计算签名，跨 section 全局去重，空标题/空步骤的 case 直接丢弃。

### 追溯（source_context）

`buildSourceContext`（`service.go` line 418 起）为每个任务汇总以下字段并写入 `test_cases.source_context`：

| 字段 | 含义 |
| --- | --- |
| `document_queries` | 文档检索使用的 query 列表（去重后） |
| `knowledge_queries` | 知识库检索使用的 query 列表（去重后） |
| `document_hits[]` | 命中的需求文档，含 `document_id` / `parent_doc_id` / `name` / `rank` / `best_score` / `hit_queries` / `top_chunks[<=3]`（每个 chunk 含 `text` / `score` / `query` / `rank`） |
| `knowledge_hits[]` | 命中的知识条目，含 `id` / `name` / `type` / `rank` / `score` / `hit_queries` |
| `knowledge_shipped_ids` | 实际拼装进 prompt 的知识条目 ID（检索命中 ∪ 影响范围匹配） |
| `knowledge_shipped_names` | 同上，按 name |

来源 ID 列表来自 `knowledge_shipped_ids` + `document_hits[].document_id`，关键片段来自 `document_hits[].top_chunks[].text`，满足 DoD「来源 ID 列表 + 关键片段」要求。

### 单测证据

| 单测 | 覆盖点 |
| --- | --- |
| `TestParseGeneratedSectionsSectionedJSON` | LLM 输出 `[{section, cases:[...]}]` 时解析正确 |
| `TestParseGeneratedSectionsFlatJSON` | LLM 输出扁平 `[{type, title, steps, expected_result}]` 时按 type 重新分组并归一化 |
| `TestDedupeGeneratedSections` | 跨 section 重复的 case 被去重，空 section 被丢弃 |
| `TestAttachCaseContext` | 每条 case 自动获得 `affected_products` / `affected_modules` / `section` 字段 |
| `TestBuildSourceContext` | source_context 携带 queries / document_hits / knowledge_hits / shipped_ids，文档命中最多保留前 3 个 chunk，shipped 跳过 id=0 |

执行命令：`cd backend && go test ./internal/service/task/...`

### 端到端校验

`scripts/i2_generation_e2e.sh`（I2-T2 共用脚本）已扩展三个 I2-T3 断言：

- `duplicate_title_count == 0`：跨 section 不存在标题完全相同的 case。
- `cases_missing_affected_fields == 0`：每条 case 都带 `affected_products` 与 `affected_modules`（数组类型，可为空）。
- `sections_with_source_context == section_count`：每行 `test_cases` 都有非空 `source_context`。

任一不满足时脚本以非零退出。本地报告默认落在 `.dev/i2_generation_e2e.md`（gitignored），含 `source_context[0] summary` 行用于人工抽查。

### 最近一次实际结果摘要

| 项 | 内容 |
| --- | --- |
| Fixture | I1-T7 公开语料 project_id=14，document_id=30（`dubbo-zipkin-integration.md`），与样例 2 同 |
| 执行命令 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i2_generation_e2e.sh` |
| go 单测 | `cd backend && go test ./internal/service/task/...` ✓ 全通过（2026-05-12 本地） |
| e2e 脚本 | 详细本地报告：`.dev/i2_generation_e2e.md`（gitignored，重跑覆盖），脚本现已包含 I2-T3 三项断言；下一轮真实模型可用时回填实际 duplicate / missing / source_context summary 数值 |

## 复现执行流程

1. 准备前置环境（同 `i1_retrieval.md`）。
2. 执行 `bash scripts/i1_public_corpus_eval.sh` 把公开语料加载到 project（持久化在 DB 中）。
3. 执行 `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_determinism.sh` 验证 I2-T1 的 3 次稳定性，把 "3/3 identical" 行回填到本文件「样例 1 最近一次实际结果摘要」。
4. 执行 `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i2_generation_e2e.sh` 触发 analyze → review → generate → 落库，把 task_id / section_count / case_count 回填到「样例 2 最近一次实际结果摘要」；脚本同时硬断言 I2-T3 三项（duplicate_title=0、无缺失 affected 字段、每行 source_context 非空），任一不通过都以非零退出，把对应数字回填到「样例 3 最近一次实际结果摘要」。
5. （可选）`cd backend && go test ./internal/service/task/...` 单独跑 I2-T3 的结构化/去重/source_context 单测。

## 历史失败与处置（可选）

- 2026-05-09：task 1 / task 2 因 Ark chat API key 失效返回 401，子 Agent 与 DeepAgent fallback 全部失败，task 进入 `failed`。处置：替换 `backend/configs/config.yaml` 中的 `model.chat.api_key`，重启后端后 task 3 通过。日志中 "[agent-service]" 前缀的行可清晰看到 retry-once + fallback 链路的全部决策点。
- 2026-05-09：task 2 子 Agent 全部成功，但 DeepAgent.RefineCases 返回的内容无法被 `parseGeneratedSections` 解析，task 进入 `failed`。处置：在 agent service 给 RefineCases 输出加 parse 校验，失败时回落到未 refine 的 dedup payload；task 3 验证通过。
