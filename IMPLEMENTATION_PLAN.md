# CaseAgent 实施计划（DoD 驱动）

## 文档用途

这份文档用于指导 AI 与研发执行实现工作，按迭代维护：

- 要做什么：任务项（Task）。
- 什么算完成：完成定义（DoD, Definition of Done）。
- 当前做到哪：状态与验证证据。

不再额外维护独立的“进度快照”文档。

## 项目终态

CaseAgent 围绕“需求文档 + 架构知识”形成可审核测试用例闭环：

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
| Iteration 1 | 数据闭环（文档/知识库 -> 向量化 -> 检索） | In Progress |
| Iteration 2 | 生成闭环（需求 -> 检索增强 -> Agent -> 用例落库） | Todo |
| Iteration 3 | 产品化闭环（前端工作台 + 审核体验） | Todo |

## 执行约束

- 默认按迭代顺序执行：`Iteration 1 -> Iteration 2 -> Iteration 3`。
- 每个迭代必须先满足本迭代 DoD 并补充可复现验证证据，再进入下一迭代。
- 同一时间只允许一个迭代保持 `In Progress`；后续迭代保持 `Todo`，已有代码能力记录在对应迭代的“当前已完成能力”中。
- 从本计划版本起，按分阶段验收推进，避免最终集中返工。

## Iteration 1：数据闭环（最高优先级）

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 验证证据 |
| --- | --- | --- | --- | --- |
| I1-T1 | 文档链路真实联调（上传 -> 分块 -> embedding -> 检索） | In Progress | 可重复执行一套联调步骤，且文档检索稳定命中本轮上传文档；脚本重复执行时不受历史数据影响。 | 已补 `testdata/i1/requirement.md` 与 `scripts/i1_retrieval_smoke.sh`；后端启动会补齐老库 `documents.content`。下一步在具备后端、PostgreSQL、pgvector、embedding 配置的环境执行 `bash scripts/i1_retrieval_smoke.sh`，补充完整上传、分块、embedding、状态与检索命中结果。 |
| I1-T2 | 知识库链路真实联调（创建 -> embedding -> 检索） | In Progress | 可重复执行一套联调步骤，且知识库检索稳定命中本轮创建知识条目；脚本重复执行时不受历史数据影响。 | 已补 `testdata/i1/product_knowledge.md`、`testdata/i1/module_knowledge.md` 与 `scripts/i1_retrieval_smoke.sh`；已修复老库 `knowledge_base.status/metadata/created_at/updated_at` 补列。下一步执行 smoke 并补充创建、embedding、状态与检索命中结果。 |
| I1-T3 | 检索回归样例沉淀 | In Progress | 至少 2 条可复现回归样例（文档/知识库各 1 条），包含输入 fixture、查询词、期望命中对象、执行命令、前置环境与实际结果摘要。 | 已补固定 fixture 与 smoke 脚本作为回归样例载体；待执行后补充实际结果摘要。 |
| I1-T4 | 知识库分块检索评估与改造 | Todo | 使用长知识库 fixture 验证整篇 embedding 的召回效果：fixture 需包含至少 3 个相互独立主题，每个主题有唯一查询词；每个查询在 `top_k=5` 内命中对应知识条目则记录为稳定，否则实现知识库分块向量化、按父知识条目聚合返回，并补充分块前后召回对比。 | 待补：长知识库 fixture、查询词、期望命中条目、现有整篇 embedding 召回结果；如需改造，补充 `knowledge_chunks`/父条目聚合/API 返回兼容性说明与对比结果。 |
| I1-T5 | 联调脚本重复执行隔离 | Todo | smoke 每次运行使用唯一 run token 或清理策略，文档与知识库断言均校验本轮创建对象，避免旧数据、同名 fixture 或相似 embedding 影响结果。 | 当前文档检索已按 `document_ids` 限定；知识库检索仍需补充本轮 ID/metadata 校验或运行前清理策略。 |

### 当前已完成能力（支撑 I1）

- 后端基础结构已落地：配置加载、Gin 路由、Bun 模型、迁移脚本、服务层骨架。
- pgvector 自定义向量编码/解码已完成。
- 启动时会按 `model.embedding.dimensions` 校验并对齐向量列维度；若已有非空向量且维度不一致则启动报错。
- 文档处理支持：Google Drive 导入、base64 图片清洗、Markdown 分块与二次切分、原文持久化、单文档重处理。
- 检索支持：`query` / `queries` 双形态调用，multi-query 合并去重。
- 已验证后端可连接本地 PostgreSQL，自动应用 `backend/migrations/001_init.sql` 并监听启动。

## Iteration 2：生成闭环

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 验证证据 |
| --- | --- | --- | --- | --- |
| I2-T1 | parent retriever 细化与上下文拼装增强 | Todo | 在现有需求检索上下文基础上，生成前上下文需包含父文档 ID/名称、命中片段、检索 query 与排序信息；同一 fixture 连续执行 3 次命中对象一致。 | 已有需求文档检索上下文拼装基础，但当前上下文主要是文本片段，追溯字段与稳定性证据不足。 |
| I2-T2 | DeepAgent 协调逻辑完善 | Todo | 在现有 4 个子 Agent 顺序调用基线上，明确 DeepAgent/Agent Service 的职责边界；支持任务拆分、子 Agent 汇总、失败恢复，且任一子 Agent 失败不会阻塞其他结果入库。 | 当前为 Agent Service 顺序调用 4 个子 Agent，子 Agent 失败会跳过，结果汇总去重后交给 DeepAgent refine；DeepAgent 原生子 Agent 协调仍待完善。 |
| I2-T3 | 生成质量控制（去重/结构化/追溯） | Todo | 生成结果结构满足 `docs/spec.md` 的 JSON 契约和数据库存储约束；重复用例可被去重；每个用例至少保留受影响产品/模块，并能追溯到生成使用的需求/知识上下文摘要。 | 已支持解析 section/flat JSON、默认字段归一、跨 section 去重、附带受影响产品/模块字段并落库；仍缺少来源上下文追溯证据。 |
| I2-T4 | 生成闭环联调样例沉淀 | Todo | 完成“选定需求 -> 分析影响范围 -> 审核影响范围 -> 生成 -> 入库 -> 查询 -> 修改/提交”的可复现样例，并记录请求样例、关键响应、数据库结果和失败重试方式。 | 待补充。 |

### 当前已完成能力（支撑 I2）

- 任务服务已支持需求文档装载、受影响产品/模块初步识别、状态流转基础校验。
- Agent 服务已接入，支持生成测试用例落库。
- 生成结果可被审核接口读取与后续修改流程消费（基础能力已具备）。

## Iteration 3：产品化闭环

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 验证证据 |
| --- | --- | --- | --- | --- |
| I3-T1 | 前端基础设施（`element-plus`、`pinia`、路由、布局） | Todo | 可承载业务页面开发，具备统一状态管理、路由框架、API client、统一错误处理入口；`npm run build` 通过。 | 当前仅有 Vue 3 + Vite 脚手架；`README.md` 描述了 Element Plus 目标，但 `frontend/package.json` 尚未接入。 |
| I3-T2 | 业务页面（文档、知识库、任务、结果、审核） | Todo | 不依赖手工 API 调用即可走完“项目创建/选择 -> 文档上传 -> 知识库维护 -> 任务创建 -> 影响范围审核 -> 用例生成 -> 用例修改/提交”主流程，页面状态与后端状态一致。 | 待开始。 |
| I3-T3 | 错误提示与运维可观测性 | Todo | 主要失败场景有用户可见提示，后端日志包含可定位的 task/document/knowledge ID；前端能展示 processing/completed/failed 状态并支持重试入口。 | 待开始。 |

## 参考文件

- 规格说明：`docs/spec.md`
- 配置示例：`backend/configs/config-example.yaml`
- 数据库迁移：`backend/migrations/001_init.sql`
- 路由定义：`backend/internal/api/router/router.go`
- 本地联调脚本：`dev.sh`

## 维护约定

- 所有迭代只维护“任务项 + 状态 + DoD + 验证证据”。
- 状态只用：`Todo`、`In Progress`、`Blocked`、`Done`。
- 任务完成后必须补充可复现证据（接口样例、脚本、日志要点）。
- 新任务先写 DoD，再进入 `In Progress`。
