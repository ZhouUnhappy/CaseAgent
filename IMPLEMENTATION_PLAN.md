# CaseAgent 实施计划（DoD 驱动）

## 文档用途

本文件供 AI 协作执行使用，按迭代维护两件事：

- 要做什么：任务项（Task）。
- 什么算完成：完成定义（DoD, Definition of Done）。

不再维护"进展叙述"或"已完成能力清单"——已发生的事实由代码与 `docs/regression/` 等证据目录承载。AI 拿到本计划后，对每个 `Done` 任务可按 DoD 中点明的脚本/路径复跑验证，对每个 `Todo` 任务以 DoD 为合同推进。

## 项目终态

CaseAgent 围绕"需求文档 + 架构知识"形成可审核测试用例闭环：

1. 导入需求文档与架构知识。
2. 完成文档清洗、分块、向量化、检索。
3. 识别受影响产品和模块。
4. 基于需求与知识上下文生成测试用例。
5. 支持测试用例审核、修改、提交。
6. 必要时沉淀知识库更新建议。

## 固定技术决策

- 前端：Vue 3 + Vite。
- 后端：Golang + Gin + Bun ORM + eino + eino-ext。
- 数据库：PostgreSQL + pgvector。
- 模型配置统一走配置文件，并支持环境变量覆盖。
- chat 当前支持 Ark；embedding 当前支持 Ark / OpenAI-compatible。
- embedding 维度通过 `model.embedding.dimensions` 显式配置。
- 当前工程对 `Qwen3-Embedding-8B` 使用 `2000` 维输出，以兼容 `pgvector + ivfflat` 索引约束。
- Google Drive 集成通过本地 `gws drive files export` 命令调用，无 Docker 依赖。

## 迭代总览

| 迭代 | 主题 | 状态 |
| --- | --- | --- |
| Iteration 1 | 数据闭环（文档/知识库 -> 向量化 -> 检索） | Done |
| Iteration 2 | 生成闭环（需求 -> 检索增强 -> Agent -> 用例落库） | Done |
| Iteration 3 | 产品化闭环（前端工作台 + 审核体验） | Todo |

迭代级 DoD = 本迭代所有任务 DoD 全部满足。

## 执行约束

- 默认按迭代顺序执行：`Iteration 1 -> Iteration 2 -> Iteration 3`。
- 同一时间只允许一个迭代保持 `In Progress`，后续迭代保持 `Todo`。
- 进入下一迭代前，本迭代所有任务必须为 `Done`。

## Iteration 1：数据闭环（最高优先级）

| ID | 任务项 | 状态 | DoD（完成定义） |
| --- | --- | --- | --- |
| I1-T1 | 文档链路真实联调（上传 -> 分块 -> embedding -> 检索） | Done | 在具备后端、PostgreSQL、pgvector、embedding 配置的环境下，`bash scripts/i1_retrieval_smoke.sh` 连续 3 次执行均：（a）通过 run token 或本轮 `document_ids` 与历史数据隔离；（b）文档状态最终为 `completed`、分块与 embedding 数量 > 0；（c）`top_k=5` 检索结果中本轮上传文档排名第一。证据：`docs/regression/i1_retrieval.md` 样例 1。 |
| I1-T2 | 知识库链路真实联调（创建 -> embedding -> 检索） | Done | 在具备后端、PostgreSQL、pgvector、embedding 配置的环境下，`bash scripts/i1_retrieval_smoke.sh` 连续 3 次执行均：（a）通过 run token 或本轮创建对象 ID/metadata 与历史数据隔离；（b）知识条目状态最终为 `completed`、embedding 已写入；（c）`top_k=5` 检索结果中本轮创建知识条目排名第一。证据：`docs/regression/i1_retrieval.md` 样例 2。 |
| I1-T3 | 检索回归样例沉淀 | Done | 在 `docs/regression/i1_retrieval.md` 提交至少 2 条回归样例（文档/知识库各 1 条），每条包含：fixture 路径、查询词、期望命中对象、执行命令、前置环境、实际结果摘要；T1/T2 每次稳定通过后更新摘要。 |
| I1-T4 | 知识库分块检索评估与改造 | Done | 长知识库 fixture（`testdata/i1/long_knowledge.md`）包含至少 3 个相互独立主题，每个主题有唯一查询词。若每个查询在 `top_k=5` 内命中对应条目则视为整篇 embedding 稳定，记录证据后关闭；否则实现知识库分块向量化、按父知识条目聚合返回，并在 T3 回归样例中补充分块前后召回对比。证据：`docs/regression/i1_retrieval.md` 样例 3。 |
| I1-T5 | 真实 Markdown base64 图片清洗 | Done | 文档与知识库处理链路在持久化、分块、embedding 前忽略 Markdown 中的 inline base64 图片与 reference-style base64 图片定义/引用；清洗逻辑有单元测试覆盖（`backend/internal/markdown`），且 `go test ./...` 通过。 |
| I1-T6 | 私有真实语料回归测试 | Done | 私有测试数据目录加入 `.gitignore`，不提交私有数据；`scripts/i1_private_corpus_eval.sh` 可通过环境变量读取真实架构知识目录与需求/设计输入目录，并记录文件数、字节数、清洗后字节数、chunk 数、embedding 成功数、典型查询 `top_k=5` 命中情况；在本地真实语料上至少跑通 1 次。证据：`docs/regression/i1_retrieval.md` 私有语料样例。 |
| I1-T7 | 公开复杂文档语料回归测试 | Done | 在 `testdata/i1/public_corpus/` 提交至少 2 批公开复杂文档（fixture 直接 commit 进仓库，不在运行时 fetch）：1 批长篇设计/分析文档（建议 `testdata/i1/public_corpus/long/`）+ 1 批短篇单话题架构知识（建议 `testdata/i1/public_corpus/short/`）；每个子集目录内包含上游 `LICENSE` 与 `SOURCES.md`（逐条记录上游 URL、抓取的 commit hash 与日期）；新增脚本 `scripts/i1_public_corpus_eval.sh` 通过环境变量读取该目录跑通完整链路（清洗 -> 分块 -> embedding -> 检索），记录文件数、清洗前/后字节数、chunk 数、embedding 成功数、典型查询 `top_k=5` 命中；在 `docs/regression/i1_retrieval.md` 至少补充长文 1 条 + 短文 1 条公开语料样例。证据：`docs/regression/i1_retrieval.md` 样例 5。 |

## Iteration 2：生成闭环

| ID | 任务项 | 状态 | DoD（完成定义） |
| --- | --- | --- | --- |
| I2-T1 | parent retriever 细化与上下文拼装增强 | Done | 生成前的检索上下文包含：父文档 ID、父文档名称、命中片段文本、检索 query、片段排序与得分；同一 fixture 连续执行 3 次返回的命中对象与排序一致。证据：`docs/regression/i2_retrieval_context.md` 样例 1。 |
| I2-T2 | DeepAgent 协调逻辑完善 | Done | DeepAgent 与 Agent Service 的职责边界以代码注释或 `docs/` 内说明落档；任一子 Agent 失败可重试至少 1 次，重试后仍失败的子 Agent 不阻塞其他结果入库；选定需求 fixture 端到端跑通并落库 ≥1 条用例。证据：`backend/internal/service/agent/service.go` 顶部 package doc + `docs/regression/i2_retrieval_context.md` 样例 2（`scripts/i2_generation_e2e.sh` 一次跑通，37 cases / 4 sections 落库）。 |
| I2-T3 | 生成质量控制（去重/结构化/追溯） | Done | 生成结果结构满足 `docs/spec.md` 的 JSON 契约和数据库存储约束；同 fixture 重复用例可被去重；每条用例至少保留受影响产品/模块字段，并能追溯到生成使用的需求/知识上下文（来源 ID 列表 + 关键片段）。证据：`backend/internal/service/task/service_test.go`（`TestParseGeneratedSectionsSectionedJSON` / `TestParseGeneratedSectionsFlatJSON` / `TestDedupeGeneratedSections` / `TestAttachCaseContext` / `TestBuildSourceContext`）+ `backend/migrations/001_init.sql`（`source_context JSONB` + `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`）+ `docs/regression/i2_retrieval_context.md` 样例 3；`scripts/i2_generation_e2e.sh` 已加入 `duplicate_title_count==0` / `cases_missing_affected_fields==0` / `sections_with_source_context==section_count` 三项硬断言。 |
| I2-T4 | 生成闭环联调样例沉淀 | Done | 在 `docs/regression/` 下提交端到端样例文档，覆盖"选定需求 -> 分析影响范围 -> 审核影响范围 -> 生成 -> 入库 -> 查询 -> 修改/提交"全流程，记录请求样例、关键响应、数据库结果、失败重试方式。证据：`docs/regression/i2_e2e_generation.md`（API 路径表 + 步骤 1–9 cURL 样例 + DB 直查 SQL + 失败重试表），与 `scripts/i2_generation_e2e.sh` 配合复跑步骤 4–7。 |

## Iteration 3：产品化闭环

| ID | 任务项 | 状态 | DoD（完成定义） |
| --- | --- | --- | --- |
| I3-T1 | 前端基础设施（`element-plus`、`pinia`、路由、布局） | Todo | `frontend/package.json` 接入 `element-plus`、`pinia`、`vue-router`；提供布局壳（顶栏/侧栏/内容区）、统一 API client、统一错误处理入口；至少 1 个示例业务页面跑通；`npm run build` 通过。 |
| I3-T2 | 业务页面（文档、知识库、任务、结果、审核） | Todo | 不依赖手工 API 调用即可走完"项目创建/选择 -> 文档上传 -> 知识库维护 -> 任务创建 -> 影响范围审核 -> 用例生成 -> 用例修改/提交"主流程；页面展示的状态字段（`pending`/`processing`/`completed`/`failed` 等）直接来自后端响应，前端不本地推断。 |
| I3-T3 | 错误提示与运维可观测性 | Todo | 主要失败场景（上传失败、embedding 失败、生成失败）有用户可见提示并区分可重试 / 不可重试；后端日志在每条主要请求中包含 task / document / knowledge ID；前端能展示 `processing`/`completed`/`failed` 状态并提供重试入口。 |

## 参考文件

- 规格说明：`docs/spec.md`
- 配置示例：`backend/configs/config-example.yaml`
- 数据库迁移：`backend/migrations/001_init.sql`
- 路由定义：`backend/internal/api/router/router.go`
- 本地联调脚本：`dev.sh`
- 回归证据：`docs/regression/`

## 维护约定

- 每个任务只维护"任务项 + 状态 + DoD"三件事。
- 状态只用：`Todo`、`In Progress`、`Blocked`、`Done`。
- DoD 必须可机器复核：写明验证命令、脚本路径或证据落档位置（如 `docs/regression/...` 段落、单元测试包路径），AI 凭此即可判定完成与否。
- 任务标 `Done` 之前，DoD 中点明的证据必须在仓库内真实存在。
- 新任务先写 DoD，再置为 `In Progress`。
- 任务范围若发生变化，直接修改 DoD；不在表外另起说明。
