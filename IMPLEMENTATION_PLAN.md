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

## 迭代总览（截至 2026-04-29）

| 迭代 | 主题 | 状态 |
| --- | --- | --- |
| Iteration 1 | 数据闭环（文档/知识库 -> 向量化 -> 检索） | In Progress |
| Iteration 2 | 生成闭环（需求 -> 检索增强 -> Agent -> 用例落库） | In Progress |
| Iteration 3 | 产品化闭环（前端工作台 + 审核体验） | Todo |

## Iteration 1：数据闭环（最高优先级）

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 验证证据 |
| --- | --- | --- | --- | --- |
| I1-T1 | 文档链路真实联调（上传 -> 分块 -> embedding -> 检索） | In Progress | 可重复执行一套联调步骤，且文档检索稳定命中。 | 已具备上传异步处理、分块、`document_chunks` 写入与检索 API。 |
| I1-T2 | 知识库链路真实联调（创建 -> embedding -> 检索） | In Progress | 可重复执行一套联调步骤，且知识库检索稳定命中。 | 知识库 CRUD、异步 embedding、检索 API 已可用。 |
| I1-T3 | 检索回归样例沉淀 | Todo | 至少 2 条可复现回归样例（文档/知识库各 1 条）。 | 待补充。 |

### 当前已完成能力（支撑 I1）

- 后端基础结构已落地：配置加载、Gin 路由、Bun 模型、迁移脚本、服务层骨架。
- pgvector 自定义向量编码/解码已完成。
- 启动时会按 `model.embedding.dimensions` 校验并对齐向量列维度；若已有非空向量且维度不一致则启动报错。
- 文档处理支持：Google Drive 导入、base64 图片清洗、Markdown 分块与二次切分、原文持久化、单文档重处理。
- 检索支持：`query` / `queries` 双形态调用，multi-query 合并去重。
- `2026-04-28` 已验证后端可连接本地 PostgreSQL，自动应用 `backend/migrations/001_init.sql` 并监听启动。

## Iteration 2：生成闭环

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 验证证据 |
| --- | --- | --- | --- | --- |
| I2-T1 | parent retriever 细化与上下文拼装增强 | In Progress | 生成前的上下文可追溯到父文档与命中片段，且结果一致性可验证。 | 已有需求文档检索上下文拼装基础。 |
| I2-T2 | DeepAgent 协调逻辑完善 | In Progress | 支持任务拆分、子 Agent 汇总、失败恢复，失败不阻塞整体流程。 | DeepAgent 与 4 个子 Agent 骨架已存在，基础汇总/去重已具备。 |
| I2-T3 | 生成质量控制（去重/结构化/追溯） | In Progress | 生成结果结构满足存储约束，重复率可控，且可追溯来源上下文。 | 已支持生成结果附带受影响产品/模块字段并落库。 |
| I2-T4 | 生成闭环联调样例沉淀 | Todo | 完成“选定需求 -> 生成 -> 入库 -> 查询/修改”可复现样例。 | 待补充。 |

### 当前已完成能力（支撑 I2）

- 任务服务已支持需求文档装载、受影响产品/模块初步识别、状态流转基础校验。
- Agent 服务已接入，支持生成测试用例落库。
- 生成结果可被审核接口读取与后续修改流程消费（基础能力已具备）。

## Iteration 3：产品化闭环

### 任务看板

| ID | 任务项 | 状态 | DoD（完成定义） | 验证证据 |
| --- | --- | --- | --- | --- |
| I3-T1 | 前端基础设施（`element-plus`、`pinia`、路由、布局） | Todo | 可承载业务页面开发，具备统一状态管理和路由框架。 | 当前仅有 Vue 3 + Vite 脚手架。 |
| I3-T2 | 业务页面（文档、知识库、任务、结果、审核） | Todo | 不依赖手工 API 调用即可走完主流程页面。 | 待开始。 |
| I3-T3 | 错误提示与运维可观测性 | Todo | 主要失败场景有用户可见提示，后端日志可定位问题。 | 待开始。 |

## 风险与约束

- 知识库当前仍采用“整篇文档一个 embedding”，召回粒度弱于文档分块。
- 多 Agent 协同当前更接近“骨架可跑”，距离稳定生产用例仍有差距。

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
