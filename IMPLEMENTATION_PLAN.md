# CaseAgent 实施计划（DoD 驱动）

## 文档用途

按迭代维护以下三件事：

- 要做什么：任务项（Task）。
- 什么算完成：完成定义（DoD, Definition of Done）。
- 当前做到哪：状态与已发生的进展。

不维护额外的进度快照文档。

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

迭代级 DoD = 本迭代所有任务 DoD 全部满足、进展列已落档为可复现证据。

## 执行约束

- 默认按迭代顺序执行：`Iteration 1 -> Iteration 2 -> Iteration 3`。
- 同一时间只允许一个迭代保持 `In Progress`，后续迭代保持 `Todo`，已有代码能力记录在对应迭代的“当前已完成能力”中。
- 进入下一迭代前，本迭代所有任务必须为 `Done` 且进展列已落档为可复现证据。

## Iteration 1：数据闭环（最高优先级）

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 进展 |
| --- | --- | --- | --- | --- |
| I1-T1 | 文档链路真实联调（上传 -> 分块 -> embedding -> 检索） | Done | 在具备后端、PostgreSQL、pgvector、embedding 配置的环境下，`bash scripts/i1_retrieval_smoke.sh` 连续 3 次执行均：（a）通过 run token 或本轮 `document_ids` 与历史数据隔离；（b）文档状态最终为 `completed`、分块与 embedding 数量 > 0；（c）`top_k=5` 检索结果中本轮上传文档排名第一。 | 本地连续 3 次通过：document_id ∈ {4, 5, 6} 各为本轮 rank-1，document_chunks 行数=4 且 embedding 全非空；详见 `docs/regression/i1_retrieval.md` 样例 1。 |
| I1-T2 | 知识库链路真实联调（创建 -> embedding -> 检索） | Done | 在具备后端、PostgreSQL、pgvector、embedding 配置的环境下，`bash scripts/i1_retrieval_smoke.sh` 连续 3 次执行均：（a）通过 run token 或本轮创建对象 ID/metadata 与历史数据隔离；（b）知识条目状态最终为 `completed`、embedding 已写入；（c）`top_k=5` 检索结果中本轮创建知识条目排名第一。 | 本地连续 3 次通过：module_knowledge_id ∈ {4, 6, 8} 各为本轮 rank-1，rank-1 `metadata.run_token` 与本轮 `RUN_TOKEN` 一致；`knowledge_base` embedding 全非空；详见 `docs/regression/i1_retrieval.md` 样例 2。 |
| I1-T3 | 检索回归样例沉淀 | Done | 在仓库内（建议 `docs/regression/i1_retrieval.md`）提交至少 2 条回归样例（文档/知识库各 1 条），每条包含：fixture 路径、查询词、期望命中对象、执行命令、前置环境、实际结果摘要；T1/T2 每次稳定通过后更新摘要。 | `docs/regression/i1_retrieval.md` 已包含 2 条样例 + 复现执行流程；已把连续 3 次通过的 run_token / 命中 ID / chunk 计数回填到摘要段。 |
| I1-T4 | 知识库分块检索评估与改造 | Done | 长知识库 fixture 包含至少 3 个相互独立主题，每个主题有唯一查询词。若每个查询在 `top_k=5` 内命中对应条目则视为整篇 embedding 稳定，记录证据后关闭；否则实现知识库分块向量化、按父知识条目聚合返回，并在 T3 回归样例中补充分块前后召回对比。 | 已补 `testdata/i1/long_knowledge.md`（3 个相互独立主题，唯一查询词列在 fixture 末尾的评估查询样例表）与 `scripts/i1_long_knowledge_eval.sh`；本地执行通过：run_token=`i1-long-20260507161825-10193`，long_knowledge_id=9，A/B/C 三个唯一查询均 rank-1 命中本条目，详见 `docs/regression/i1_retrieval.md` 样例 3。 |
| I1-T5 | 真实 Markdown base64 图片清洗 | Done | 文档与知识库处理链路在持久化、分块、embedding 前忽略 Markdown 中的 inline base64 图片与 reference-style base64 图片定义/引用；清洗逻辑有单元测试覆盖，且后端测试通过。 | 新增 `backend/internal/markdown` 清洗包并接入 `document` / `knowledge` 服务；覆盖 `![...](data:image...)`、`![][image1]` + `[image1]: <data:image...>`；执行 `GOCACHE=/private/tmp/caseagent-go-cache go test ./...` 通过。 |
| I1-T6 | 私有真实语料回归测试 | Done | 私有测试数据目录加入 `.gitignore`，不提交私有数据；脚本可通过环境变量读取真实架构知识目录与需求/设计输入目录，并记录文件数、字节数、清洗后字节数、chunk 数、embedding 成功数、典型查询 `top_k=5` 命中情况；在本地真实语料上至少跑通 1 次后回填证据。 | 新增 `scripts/i1_private_corpus_eval.sh` 与 gitignored `testdata/private/`；本地执行通过：run_token=`i1-private-20260507170014-5662`，架构知识 8 个 md / 18,780 bytes，需求输入 5 个 md / 4,204,164 bytes，清洗后文档内容 85,943 bytes，document_chunks=101 且 embeddings=101，knowledge_embeddings=8；3 条文档查询与 3 条知识查询均 rank-1 命中本轮对象，详见 `docs/regression/i1_retrieval.md` 私有语料样例。 |
| I1-T7 | 公开复杂文档语料回归测试 | Todo | 在仓库内提供至少 2 批公开复杂文档：1 批长篇设计提案（Kubernetes KEP / Rust RFC / Python PEP / Apache KIP 任选其一）+ 1 批短篇架构知识（ADR 集合，例如 `joelparkerhenderson/architecture-decision-record` 示例集），并在 fixture 目录的 README 记录来源 URL、抓取时间与许可证；脚本可通过环境变量读取该目录跑通完整链路（清洗 -> 分块 -> embedding -> 检索），记录文件数、清洗前/后字节数、chunk 数、embedding 成功数、典型查询 `top_k=5` 命中；在 `docs/regression/i1_retrieval.md` 至少补充长文 1 条 + 短文 1 条公开语料样例并回填证据。 | 待补充。 |

### 当前已完成能力（支撑 I1）

- 后端基础结构已落地：配置加载、Gin 路由、Bun 模型、迁移脚本、服务层骨架。
- pgvector 自定义向量编码/解码已完成。
- 启动时会按 `model.embedding.dimensions` 校验并对齐向量列维度；若已有非空向量且维度不一致则启动报错。
- 文档处理支持：Google Drive 导入、base64 图片清洗、Markdown 分块与二次切分、原文持久化、单文档重处理。
- Markdown 清洗支持 inline / reference-style base64 图片移除，并会把清洗后的文档/知识库内容写回数据库，避免图片数据进入后续检索与生成上下文。
- 检索支持：`query` / `queries` 双形态调用，multi-query 合并去重。
- 已验证后端可连接本地 PostgreSQL，自动应用 `backend/migrations/001_init.sql` 并监听启动。
- 私有真实语料回归支持通过环境变量读取本地私有目录，并把详细报告写入 gitignored `testdata/private/runs/`。

## Iteration 2：生成闭环

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 进展 |
| --- | --- | --- | --- | --- |
| I2-T1 | parent retriever 细化与上下文拼装增强 | Todo | 生成前的检索上下文包含：父文档 ID、父文档名称、命中片段文本、检索 query、片段排序与得分；同一 fixture 连续执行 3 次返回的命中对象与排序一致。 | 已有需求文档检索上下文拼装基础，但当前以文本片段为主，追溯字段与稳定性证据不足。 |
| I2-T2 | DeepAgent 协调逻辑完善 | Todo | DeepAgent 与 Agent Service 的职责边界以代码注释或 `docs/` 内说明落档；任一子 Agent 失败可重试至少 1 次，重试后仍失败的子 Agent 不阻塞其他结果入库；选定需求 fixture 端到端跑通并落库 ≥1 条用例。 | 当前为 Agent Service 顺序调用 4 个子 Agent，子 Agent 失败会跳过，结果汇总去重后交给 DeepAgent refine；DeepAgent 原生子 Agent 协调仍待完善。 |
| I2-T3 | 生成质量控制（去重/结构化/追溯） | Todo | 生成结果结构满足 `docs/spec.md` 的 JSON 契约和数据库存储约束；同 fixture 重复用例可被去重；每条用例至少保留受影响产品/模块字段，并能追溯到生成使用的需求/知识上下文（来源 ID 列表 + 关键片段）。 | 已支持解析 section/flat JSON、默认字段归一、跨 section 去重、附带受影响产品/模块字段并落库；仍缺少来源上下文追溯字段。 |
| I2-T4 | 生成闭环联调样例沉淀 | Todo | 在仓库内提交端到端样例文档，覆盖“选定需求 -> 分析影响范围 -> 审核影响范围 -> 生成 -> 入库 -> 查询 -> 修改/提交”全流程，记录请求样例、关键响应、数据库结果、失败重试方式。 | 待补充。 |

### 当前已完成能力（支撑 I2）

- 任务服务已支持需求文档装载、受影响产品/模块初步识别、状态流转基础校验。
- Agent 服务已接入，支持生成测试用例落库。
- 生成结果可被审核接口读取与后续修改流程消费（基础能力已具备）。

## Iteration 3：产品化闭环

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 进展 |
| --- | --- | --- | --- | --- |
| I3-T1 | 前端基础设施（`element-plus`、`pinia`、路由、布局） | Todo | `frontend/package.json` 接入 `element-plus`、`pinia`、`vue-router`；提供布局壳（顶栏/侧栏/内容区）、统一 API client、统一错误处理入口；至少 1 个示例业务页面跑通；`npm run build` 通过。 | 当前仅有 Vue 3 + Vite 脚手架；`README.md` 描述了 Element Plus 目标，但 `frontend/package.json` 尚未接入。 |
| I3-T2 | 业务页面（文档、知识库、任务、结果、审核） | Todo | 不依赖手工 API 调用即可走完“项目创建/选择 -> 文档上传 -> 知识库维护 -> 任务创建 -> 影响范围审核 -> 用例生成 -> 用例修改/提交”主流程；页面展示的状态字段（`pending`/`processing`/`completed`/`failed` 等）直接来自后端响应，前端不本地推断。 | 待开始。 |
| I3-T3 | 错误提示与运维可观测性 | Todo | 主要失败场景（上传失败、embedding 失败、生成失败）有用户可见提示并区分可重试 / 不可重试；后端日志在每条主要请求中包含 task / document / knowledge ID；前端能展示 `processing`/`completed`/`failed` 状态并提供重试入口。 | 待开始。 |

## 参考文件

- 规格说明：`docs/spec.md`
- 配置示例：`backend/configs/config-example.yaml`
- 数据库迁移：`backend/migrations/001_init.sql`
- 路由定义：`backend/internal/api/router/router.go`
- 本地联调脚本：`dev.sh`

## 维护约定

- 所有迭代只维护“任务项 + 状态 + DoD + 进展”。
- 状态只用：`Todo`、`In Progress`、`Blocked`、`Done`。
- 进展列只填已发生的事实（已提交的文件、已跑通的脚本、已落库的数据），不写“下一步”；下一步由 DoD 与当前进展之差隐含。
- 任务完成后必须补充可复现证据（接口样例、脚本、日志要点）。
- 新任务先写 DoD，再进入 `In Progress`。
