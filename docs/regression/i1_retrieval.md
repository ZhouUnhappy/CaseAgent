# I1 检索回归样例

支撑 `IMPLEMENTATION_PLAN.md` 中 I1-T1 / I1-T2 / I1-T3 的回归证据。
每次 `bash scripts/i1_retrieval_smoke.sh` 通过后，在「最近一次实际结果摘要」追加最新一次 `run_token` 与命中详情。

## 前置环境

- 后端服务监听于 `CASEAGENT_BASE_URL`（默认 `http://localhost:8080/api/v1`），并已应用 `backend/migrations/001_init.sql`。
- PostgreSQL 已启用 `pgvector` 扩展，向量列维度与 `model.embedding.dimensions` 一致。
- `model.embedding` 配置可访问（Ark 或 OpenAI-compatible），后端启动后能成功调用 embedding 接口。
- 本地命令：`curl`、`jq`；启用 PSQL 校验时还需 `psql` 与 `CASEAGENT_PSQL_DSN`。

## 样例 1：文档检索（I1-T1）

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/requirement.md` |
| 查询词 | `probe-gate-7781 rollback completed` |
| 期望命中对象 | 本轮 smoke 上传的需求文档（rank 1） |
| 期望分块 | rank-1 条目至少有 1 条 `matched_chunks` |
| 执行命令 | `bash scripts/i1_retrieval_smoke.sh` |
| 启用数据库校验 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_smoke.sh` |

### 最近一次实际结果摘要

- run_token：TBD（待首次成功执行后回填）
- project_id / document_id：TBD
- rank-1 document_id：TBD
- matched_chunks 数量：TBD
- document_chunks 行数（启用 PSQL 时）：TBD
- 全部 chunk embedding 非空：TBD

## 样例 2：知识库检索（I1-T2）

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/product_knowledge.md`、`testdata/i1/module_knowledge.md` |
| 查询词 | `control-plane-probe 18080 probe-gate-7781` |
| 期望命中对象 | 本轮 smoke 创建的 module 类型知识条目（rank 1） |
| 期望 metadata | rank-1 条目 `metadata.run_token` 等于本轮 RUN_TOKEN |
| 执行命令 | `bash scripts/i1_retrieval_smoke.sh` |
| 启用数据库校验 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_smoke.sh` |

### 最近一次实际结果摘要

- run_token：TBD
- product_knowledge_id / module_knowledge_id：TBD
- rank-1 knowledge id：TBD
- rank-1 metadata.run_token：TBD
- 知识库 embedding 非空（启用 PSQL 时）：TBD

## 复现执行流程

1. 准备前置环境（见上）。
2. 执行 `bash scripts/i1_retrieval_smoke.sh`，连续运行 3 次。每次结束前末尾会输出 `run_token=...`。
3. 任一次失败，按错误信息处理：
   - `document retrieval expected document <id> at rank 1` → 检查文档分块/embedding 是否成功；可启用 `CASEAGENT_PSQL_DSN` 让脚本同时校验数据库行。
   - `knowledge retrieval expected module knowledge <id> at rank 1` → 多由历史 smoke 数据干扰：可清理 `knowledge_base` 中带 `metadata.aliases ? 'I1 smoke fixture'` 的旧行后重试。
4. 3 次都通过后，把最新一次的 `run_token`、`document_id`、`module_knowledge_id` 与 `assert_*` 输出回填到本文件「最近一次实际结果摘要」段落。

## 历史失败与处置（可选）

记录有助于诊断的历史失败 + 处置；空段无需保留。
